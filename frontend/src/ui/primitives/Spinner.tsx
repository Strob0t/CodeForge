import { type JSX, splitProps } from "solid-js";

export type SpinnerSize = "sm" | "md" | "lg";

export interface SpinnerProps {
  size?: SpinnerSize;
  class?: string;
}

const sizeClasses: Record<SpinnerSize, string> = {
  sm: "h-4 w-4",
  md: "h-6 w-6",
  lg: "h-8 w-8",
};

export function Spinner(props: SpinnerProps): JSX.Element {
  const [local] = splitProps(props, ["size", "class"]);

  const size = (): SpinnerSize => local.size ?? "md";

  return (
    <span
      role="status"
      aria-label="Loading"
      class={
        "cf-spinner inline-block " + sizeClasses[size()] + (local.class ? " " + local.class : "")
      }
    >
      <span class="sr-only">Loading</span>
    </span>
  );
}
