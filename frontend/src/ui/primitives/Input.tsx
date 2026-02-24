import { type JSX, splitProps } from "solid-js";

export interface InputProps extends JSX.InputHTMLAttributes<HTMLInputElement> {
  error?: boolean;
  mono?: boolean;
}

export function Input(props: InputProps): JSX.Element {
  const [local, rest] = splitProps(props, ["error", "mono", "class"]);

  return (
    <input
      {...rest}
      class={
        "block w-full rounded-cf-md border bg-cf-bg-surface px-3 py-2 text-sm text-cf-text-primary placeholder:text-cf-text-muted transition-colors " +
        "focus:outline-none focus:ring-2 focus:ring-cf-focus-ring focus:border-cf-accent " +
        (local.error ? "border-cf-danger focus:ring-cf-danger" : "border-cf-border-input") +
        (local.mono ? " font-mono" : "") +
        (local.class ? " " + local.class : "")
      }
    />
  );
}
