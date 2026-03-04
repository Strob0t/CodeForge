import { type JSX, Show, splitProps } from "solid-js";

import { Button } from "./Button";

export type AlertVariant = "error" | "warning" | "success" | "info";

export interface AlertProps {
  variant?: AlertVariant;
  onDismiss?: () => void;
  class?: string;
  children: JSX.Element;
}

const variantClasses: Record<AlertVariant, string> = {
  error: "bg-cf-danger-bg text-cf-danger-fg border-cf-danger-border",
  warning: "bg-cf-warning-bg text-cf-warning-fg border-cf-warning-border",
  success: "bg-cf-success-bg text-cf-success-fg border-cf-success-border",
  info: "bg-cf-info-bg text-cf-info-fg border-cf-info-border",
};

const iconMap: Record<AlertVariant, string> = {
  error: "\u2718", // cross mark
  warning: "\u26A0", // warning sign
  success: "\u2714", // check mark
  info: "\u2139", // info
};

export function Alert(props: AlertProps): JSX.Element {
  const [local, rest] = splitProps(props, ["variant", "onDismiss", "class", "children"]);

  const variant = (): AlertVariant => local.variant ?? "info";

  return (
    <div
      {...rest}
      role="alert"
      class={
        "flex items-start gap-3 rounded-cf-md border p-3 text-sm " +
        variantClasses[variant()] +
        (local.class ? " " + local.class : "")
      }
    >
      <span class="mt-0.5 shrink-0" aria-hidden="true">
        {iconMap[variant()]}
      </span>
      <div class="flex-1">{local.children}</div>
      <Show when={local.onDismiss}>
        <Button
          variant="icon"
          size="xs"
          onClick={() => local.onDismiss?.()}
          class="shrink-0 opacity-60 hover:opacity-100"
          aria-label="Dismiss"
        >
          {"\u2715"}
        </Button>
      </Show>
    </div>
  );
}
