import { createEffect, onCleanup } from "solid-js";

const FOCUSABLE_SELECTOR =
  'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])';

/**
 * Trap keyboard focus inside a container while `active` is true.
 *
 * Handles:
 * - Tab / Shift+Tab cycling within focusable elements
 * - Auto-focusing the first focusable element on activation
 * - Restoring the previously-focused element on deactivation
 * - Locking body scroll while active
 */
export function useFocusTrap(
  containerRef: () => HTMLElement | undefined,
  active: () => boolean,
): { onKeyDown: (e: KeyboardEvent) => void } {
  let previousFocus: HTMLElement | null = null;

  createEffect(() => {
    if (active()) {
      previousFocus = document.activeElement as HTMLElement | null;
      document.body.style.overflow = "hidden";
      requestAnimationFrame(() => {
        const first = containerRef()?.querySelector<HTMLElement>(FOCUSABLE_SELECTOR);
        first?.focus();
      });
    } else {
      document.body.style.overflow = "";
      previousFocus?.focus();
    }
  });

  onCleanup(() => {
    document.body.style.overflow = "";
  });

  function onKeyDown(e: KeyboardEvent) {
    if (e.key !== "Tab") return;
    const container = containerRef();
    if (!container) return;

    const focusable = container.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR);
    if (focusable.length === 0) return;

    const first = focusable[0];
    const last = focusable[focusable.length - 1];

    if (e.shiftKey) {
      if (document.activeElement === first) {
        e.preventDefault();
        last.focus();
      }
    } else {
      if (document.activeElement === last) {
        e.preventDefault();
        first.focus();
      }
    }
  }

  return { onKeyDown };
}
