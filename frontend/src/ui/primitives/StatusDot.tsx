import { type JSX, splitProps } from "solid-js";

export interface StatusDotProps {
  color: string;
  pulse?: boolean;
  class?: string;
  label?: string;
}

export function StatusDot(props: StatusDotProps): JSX.Element {
  const [local] = splitProps(props, ["color", "pulse", "class", "label"]);

  return (
    <span
      class={
        "inline-block h-2 w-2 rounded-full" +
        (local.pulse ? " animate-pulse" : "") +
        (local.class ? " " + local.class : "")
      }
      style={{ "background-color": local.color }}
      role={local.label ? "img" : "presentation"}
      aria-label={local.label}
      aria-hidden={local.label ? undefined : "true"}
    />
  );
}
