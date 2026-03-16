import { type JSX, splitProps } from "solid-js";

export interface TypingIndicatorProps {
  class?: string;
}

export function TypingIndicator(props: TypingIndicatorProps): JSX.Element {
  const [local] = splitProps(props, ["class"]);

  return (
    <span
      role="status"
      aria-label="Agent is typing"
      class={"inline-flex items-center gap-1" + (local.class ? " " + local.class : "")}
    >
      <span class="sr-only">Agent is typing</span>
      <span
        class="inline-block h-2 w-2 rounded-full bg-cf-text-muted"
        style={{ animation: "cf-bounce-dot 1.4s ease-in-out infinite", "animation-delay": "0s" }}
      />
      <span
        class="inline-block h-2 w-2 rounded-full bg-cf-text-muted"
        style={{ animation: "cf-bounce-dot 1.4s ease-in-out infinite", "animation-delay": "0.2s" }}
      />
      <span
        class="inline-block h-2 w-2 rounded-full bg-cf-text-muted"
        style={{ animation: "cf-bounce-dot 1.4s ease-in-out infinite", "animation-delay": "0.4s" }}
      />
    </span>
  );
}
