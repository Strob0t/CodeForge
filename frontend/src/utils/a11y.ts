import type { JSX } from "solid-js";

/**
 * Returns props that make a non-interactive element behave as an
 * accessible button: focusable, keyboard-operable, announced as interactive.
 * Prefer <button> whenever possible. Use this only when a <button>
 * cannot wrap the content (e.g., complex card layouts).
 */
export function clickable(
  handler: (e: MouseEvent | KeyboardEvent) => void,
  ariaLabel?: string,
): JSX.HTMLAttributes<HTMLDivElement> {
  return {
    role: "button" as const,
    tabIndex: 0,
    onClick: handler as JSX.EventHandlerUnion<HTMLDivElement, MouseEvent>,
    onKeyDown: (e: KeyboardEvent) => {
      if (e.key === "Enter" || e.key === " ") {
        e.preventDefault();
        handler(e);
      }
    },
    ...(ariaLabel ? { "aria-label": ariaLabel } : {}),
  };
}
