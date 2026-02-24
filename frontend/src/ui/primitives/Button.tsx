import { type JSX, Show, splitProps } from "solid-js";

import { Spinner } from "./Spinner";

export type ButtonVariant = "primary" | "secondary" | "danger" | "ghost";
export type ButtonSize = "sm" | "md" | "lg";

export interface ButtonProps extends JSX.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
  size?: ButtonSize;
  loading?: boolean;
  fullWidth?: boolean;
}

const variantClasses: Record<ButtonVariant, string> = {
  primary:
    "bg-cf-accent text-cf-accent-fg hover:bg-cf-accent-hover focus-visible:ring-2 focus-visible:ring-cf-focus-ring focus-visible:ring-offset-2",
  secondary:
    "border border-cf-border bg-cf-bg-surface text-cf-text-secondary hover:bg-cf-bg-surface-alt focus-visible:ring-2 focus-visible:ring-cf-focus-ring focus-visible:ring-offset-2",
  danger:
    "bg-cf-danger text-white hover:opacity-90 focus-visible:ring-2 focus-visible:ring-cf-danger focus-visible:ring-offset-2",
  ghost:
    "text-cf-text-secondary hover:bg-cf-bg-surface-alt focus-visible:ring-2 focus-visible:ring-cf-focus-ring focus-visible:ring-offset-2",
};

const sizeClasses: Record<ButtonSize, string> = {
  sm: "px-2.5 py-1 text-xs rounded-cf-sm",
  md: "px-4 py-2 text-sm rounded-cf-md",
  lg: "px-6 py-3 text-base rounded-cf-lg",
};

export function Button(props: ButtonProps): JSX.Element {
  const [local, rest] = splitProps(props, [
    "variant",
    "size",
    "loading",
    "fullWidth",
    "disabled",
    "class",
    "children",
  ]);

  const variant = (): ButtonVariant => local.variant ?? "primary";
  const size = (): ButtonSize => local.size ?? "md";
  const isDisabled = (): boolean => !!local.disabled || !!local.loading;

  return (
    <button
      {...rest}
      type={rest.type ?? "button"}
      disabled={isDisabled()}
      class={
        "inline-flex items-center justify-center font-medium transition-colors " +
        variantClasses[variant()] +
        " " +
        sizeClasses[size()] +
        (local.fullWidth ? " w-full" : "") +
        (isDisabled()
          ? " opacity-[var(--cf-disabled-opacity)] cursor-not-allowed"
          : " cursor-pointer") +
        (local.class ? " " + local.class : "")
      }
    >
      <Show when={local.loading}>
        <Spinner size="sm" class="mr-2" />
      </Show>
      {local.children}
    </button>
  );
}
