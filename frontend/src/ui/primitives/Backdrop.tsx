import type { JSX } from "solid-js";

interface BackdropProps {
  onClick: () => void;
  class?: string;
}

/**
 * Accessible backdrop overlay for modals and slide-over panels.
 * Supports click-to-close and Escape key dismissal (WCAG 2.1.1).
 */
export function Backdrop(props: BackdropProps): JSX.Element {
  const handleKeyDown = (e: KeyboardEvent): void => {
    if (e.key === "Escape") {
      props.onClick();
    }
  };

  return (
    <div
      class={`fixed inset-0 z-40 ${props.class ?? "bg-black/30"}`}
      onClick={() => props.onClick()}
      onKeyDown={handleKeyDown}
      role="button"
      tabIndex={0}
      aria-label="Close"
    />
  );
}
