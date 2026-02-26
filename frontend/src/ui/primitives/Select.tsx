import { type JSX, onMount, splitProps } from "solid-js";

export interface SelectProps extends JSX.SelectHTMLAttributes<HTMLSelectElement> {
  error?: boolean;
  /** Callback ref for the underlying <select> element. Use this instead of ref for forwarding. */
  selectRef?: (el: HTMLSelectElement) => void;
}

export function Select(props: SelectProps): JSX.Element {
  const [local, rest] = splitProps(props, ["error", "class", "children", "selectRef", "value"]);
  let selectEl!: HTMLSelectElement;

  // onMount fires after the component tree (including <For> children) is in the DOM.
  // This ensures the initial value syncs correctly even when <option>s are dynamic.
  onMount(() => {
    if (local.value != null && local.value !== "") {
      selectEl.value = local.value as string;
    }
  });

  return (
    <select
      ref={(el) => {
        selectEl = el;
        local.selectRef?.(el);
      }}
      {...rest}
      value={(local.value as string) ?? ""}
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
