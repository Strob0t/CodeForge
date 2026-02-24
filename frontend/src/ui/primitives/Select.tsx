import { type JSX, splitProps } from "solid-js";

export interface SelectProps extends JSX.SelectHTMLAttributes<HTMLSelectElement> {
  error?: boolean;
}

export function Select(props: SelectProps): JSX.Element {
  const [local, rest] = splitProps(props, ["error", "class", "children"]);

  return (
    <select
      {...rest}
      class={
        "block w-full rounded-cf-md border bg-cf-bg-surface px-3 py-2 text-sm text-cf-text-primary transition-colors " +
        "focus:outline-none focus:ring-2 focus:ring-cf-focus-ring focus:border-cf-accent " +
        (local.error ? "border-cf-danger focus:ring-cf-danger" : "border-cf-border-input") +
        (local.class ? " " + local.class : "")
      }
    >
      {local.children}
    </select>
  );
}
