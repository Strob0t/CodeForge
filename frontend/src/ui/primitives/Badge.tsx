import { type JSX, splitProps } from "solid-js";

export type BadgeVariant =
  | "default"
  | "primary"
  | "success"
  | "warning"
  | "danger"
  | "info"
  | "error"
  | "neutral";

export interface BadgeProps {
  variant?: BadgeVariant;
  pill?: boolean;
  class?: string;
  style?: JSX.CSSProperties;
  children: JSX.Element;
}

const variantClasses: Record<BadgeVariant, string> = {
  default: "bg-cf-bg-surface-alt text-cf-text-secondary border-cf-border",
  primary: "bg-cf-accent/10 text-cf-accent border-cf-accent/20",
  success: "bg-cf-success-bg text-cf-success-fg border-cf-success-border",
  warning: "bg-cf-warning-bg text-cf-warning-fg border-cf-warning-border",
  danger: "bg-cf-danger-bg text-cf-danger-fg border-cf-danger-border",
  error: "bg-cf-danger-bg text-cf-danger-fg border-cf-danger-border",
  info: "bg-cf-info-bg text-cf-info-fg border-cf-info-border",
  neutral: "bg-cf-bg-surface-alt text-cf-text-secondary border-cf-border",
};

export function Badge(props: BadgeProps): JSX.Element {
  const [local, rest] = splitProps(props, ["variant", "pill", "class", "style", "children"]);

  const variant = (): BadgeVariant => local.variant ?? "default";

  return (
    <span
      {...rest}
      class={
        "inline-flex items-center border px-2 py-0.5 text-xs font-medium " +
        variantClasses[variant()] +
        (local.pill ? " rounded-full" : " rounded-cf-sm") +
        (local.class ? " " + local.class : "")
      }
      style={local.style}
    >
      {local.children}
    </span>
  );
}
