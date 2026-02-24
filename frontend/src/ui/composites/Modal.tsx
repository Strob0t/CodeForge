import { createEffect, type JSX, onCleanup, Show, splitProps } from "solid-js";
import { Portal } from "solid-js/web";

export interface ModalProps {
  open: boolean;
  onClose: () => void;
  title?: string;
  class?: string;
  children: JSX.Element;
}

export function Modal(props: ModalProps): JSX.Element {
  const [local] = splitProps(props, ["open", "onClose", "title", "class", "children"]);
  let dialogRef: HTMLDivElement | undefined;
  let previousFocus: HTMLElement | null = null;

  function handleKeyDown(e: KeyboardEvent) {
    if (e.key === "Escape") {
      e.stopPropagation();
      local.onClose();
      return;
    }
    if (e.key === "Tab" && dialogRef) {
      trapFocus(e, dialogRef);
    }
  }

  function trapFocus(e: KeyboardEvent, container: HTMLElement) {
    const focusable = container.querySelectorAll<HTMLElement>(
      'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])',
    );
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

  function handleBackdropClick(e: MouseEvent) {
    if (e.target === e.currentTarget) {
      local.onClose();
    }
  }

  // Lock body scroll and manage focus
  createEffect(() => {
    if (local.open) {
      previousFocus = document.activeElement as HTMLElement | null;
      document.body.style.overflow = "hidden";
      // Focus the dialog content after mount
      requestAnimationFrame(() => {
        const firstFocusable = dialogRef?.querySelector<HTMLElement>(
          'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])',
        );
        firstFocusable?.focus();
      });
    } else {
      document.body.style.overflow = "";
      previousFocus?.focus();
    }
  });

  onCleanup(() => {
    document.body.style.overflow = "";
  });

  return (
    <Show when={local.open}>
      <Portal>
        <div
          class="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
          role="dialog"
          aria-modal="true"
          aria-label={local.title}
          onKeyDown={handleKeyDown}
          onClick={handleBackdropClick}
        >
          <div
            ref={dialogRef}
            class={
              "relative mx-4 max-h-[85vh] w-full max-w-lg overflow-auto rounded-cf-lg border border-cf-border bg-cf-bg-surface shadow-cf-lg" +
              (local.class ? " " + local.class : "")
            }
          >
            <Show when={local.title}>
              <div class="flex items-center justify-between border-b border-cf-border px-4 py-3">
                <h2 class="text-lg font-semibold text-cf-text-primary">{local.title}</h2>
                <button
                  type="button"
                  onClick={() => local.onClose()}
                  class="text-cf-text-muted hover:text-cf-text-primary transition-colors"
                  aria-label="Close"
                >
                  {"\u2715"}
                </button>
              </div>
            </Show>
            <div class="p-4">{local.children}</div>
          </div>
        </div>
      </Portal>
    </Show>
  );
}
