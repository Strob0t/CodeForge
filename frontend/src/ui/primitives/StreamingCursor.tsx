import { type JSX, Show, splitProps } from "solid-js";

export interface StreamingCursorProps {
  active?: boolean;
  class?: string;
}

export function StreamingCursor(props: StreamingCursorProps): JSX.Element {
  const [local] = splitProps(props, ["active", "class"]);

  return (
    <Show when={local.active}>
      <span
        aria-hidden="true"
        class={"inline-block" + (local.class ? " " + local.class : "")}
        style={{
          animation: "cf-blink 600ms step-end infinite",
          color: "var(--cf-accent)",
        }}
      >
        &#x2588;
      </span>
    </Show>
  );
}
