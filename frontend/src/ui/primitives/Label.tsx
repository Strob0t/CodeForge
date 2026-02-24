import { type JSX, Show, splitProps } from "solid-js";

export interface LabelProps extends JSX.LabelHTMLAttributes<HTMLLabelElement> {
  required?: boolean;
}

export function Label(props: LabelProps): JSX.Element {
  const [local, rest] = splitProps(props, ["required", "class", "children"]);

  return (
    <label
      {...rest}
      class={
        "block text-sm font-medium text-cf-text-secondary" + (local.class ? " " + local.class : "")
      }
    >
      {local.children}
      <Show when={local.required}>
        <span class="ml-0.5 text-cf-danger" aria-hidden="true">
          *
        </span>
        <span class="sr-only"> (required)</span>
      </Show>
    </label>
  );
}
