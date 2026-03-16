import { type JSX, Show, splitProps } from "solid-js";

export type ProgressBarSize = "sm" | "md" | "lg";
export type ProgressBarVariant = "accent" | "success" | "warning" | "danger";

export interface ProgressBarProps {
  value?: number;
  size?: ProgressBarSize;
  variant?: ProgressBarVariant;
  label?: string;
  class?: string;
}

const sizeClasses: Record<ProgressBarSize, string> = {
  sm: "h-1",
  md: "h-2",
  lg: "h-3",
};

const variantClasses: Record<ProgressBarVariant, string> = {
  accent: "bg-cf-accent",
  success: "bg-cf-success",
  warning: "bg-cf-warning",
  danger: "bg-cf-danger",
};

export function ProgressBar(props: ProgressBarProps): JSX.Element {
  const [local] = splitProps(props, ["value", "size", "variant", "label", "class"]);

  const size = (): ProgressBarSize => local.size ?? "md";
  const variant = (): ProgressBarVariant => local.variant ?? "accent";
  const determinate = (): boolean => local.value !== undefined;

  return (
    <div
      role="progressbar"
      aria-valuenow={determinate() ? local.value : undefined}
      aria-valuemin={0}
      aria-valuemax={100}
      aria-label={local.label}
      class={
        "bg-cf-bg-inset rounded-full overflow-hidden " +
        sizeClasses[size()] +
        (local.class ? " " + local.class : "")
      }
    >
      <Show
        when={determinate()}
        fallback={
          <div
            class={"h-full w-1/3 rounded-full " + variantClasses[variant()]}
            style={{ animation: "cf-progress-slide 2s ease-in-out infinite" }}
          />
        }
      >
        <div
          class={"h-full rounded-full transition-all duration-300 " + variantClasses[variant()]}
          style={{ width: `${Math.min(100, Math.max(0, local.value ?? 0))}%` }}
        />
      </Show>
    </div>
  );
}
