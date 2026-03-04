import { createSignal, type JSX, onCleanup, Show } from "solid-js";
import { Portal } from "solid-js/web";

export interface TooltipProps {
  /** Text to display in the tooltip. */
  text: string;
  /** Preferred placement relative to the trigger element. Default: "right". */
  placement?: "top" | "right" | "bottom" | "left";
  /** The trigger element(s) to wrap. */
  children: JSX.Element;
}

/**
 * Portal-based tooltip that renders at document.body level,
 * escaping any overflow-hidden containers. Automatically flips
 * placement to stay within the viewport.
 */
export function Tooltip(props: TooltipProps): JSX.Element {
  const [visible, setVisible] = createSignal(false);
  const [pos, setPos] = createSignal({ top: 0, left: 0 });
  let timer: ReturnType<typeof setTimeout> | undefined;
  let triggerRef: HTMLSpanElement | undefined;

  const DELAY_MS = 400;
  const GAP = 6;

  function calcPosition(rect: DOMRect, placement: string): { top: number; left: number } {
    // Estimate tooltip dimensions (will be measured after render, but use heuristic first)
    const estWidth = Math.max(props.text.length * 7 + 16, 40);
    const estHeight = 28;

    let top = 0;
    let left = 0;

    switch (placement) {
      case "right":
        top = rect.top + rect.height / 2 - estHeight / 2;
        left = rect.right + GAP;
        break;
      case "left":
        top = rect.top + rect.height / 2 - estHeight / 2;
        left = rect.left - GAP - estWidth;
        break;
      case "top":
        top = rect.top - GAP - estHeight;
        left = rect.left + rect.width / 2 - estWidth / 2;
        break;
      case "bottom":
        top = rect.bottom + GAP;
        left = rect.left + rect.width / 2 - estWidth / 2;
        break;
    }

    // Viewport clamping
    const vw = window.innerWidth;
    const vh = window.innerHeight;
    const pad = 8;

    if (left + estWidth > vw - pad) left = vw - pad - estWidth;
    if (left < pad) left = pad;
    if (top + estHeight > vh - pad) top = vh - pad - estHeight;
    if (top < pad) top = pad;

    return { top, left };
  }

  function show() {
    timer = setTimeout(() => {
      if (!triggerRef) return;
      const rect = triggerRef.getBoundingClientRect();
      const placement = props.placement ?? "right";

      // Try preferred placement, flip if it would overflow
      let finalPlacement = placement;
      const vw = window.innerWidth;
      const vh = window.innerHeight;

      if (placement === "right" && rect.right + GAP + 100 > vw) finalPlacement = "left";
      else if (placement === "left" && rect.left - GAP - 100 < 0) finalPlacement = "right";
      else if (placement === "bottom" && rect.bottom + GAP + 28 > vh) finalPlacement = "top";
      else if (placement === "top" && rect.top - GAP - 28 < 0) finalPlacement = "bottom";

      setPos(calcPosition(rect, finalPlacement));
      setVisible(true);
    }, DELAY_MS);
  }

  function hide() {
    clearTimeout(timer);
    setVisible(false);
  }

  onCleanup(() => clearTimeout(timer));

  return (
    <span
      ref={triggerRef}
      onMouseEnter={show}
      onMouseLeave={hide}
      onFocusIn={show}
      onFocusOut={hide}
      style={{ display: "inline-flex" }}
    >
      {props.children}
      <Show when={visible()}>
        <Portal>
          <div
            class="pointer-events-none fixed z-[70] whitespace-nowrap rounded-cf-md border border-cf-border bg-cf-bg-surface px-2 py-1 text-xs text-cf-text-primary shadow-cf-md"
            style={{ top: `${pos().top}px`, left: `${pos().left}px` }}
            role="tooltip"
          >
            {props.text}
          </div>
        </Portal>
      </Show>
    </span>
  );
}
