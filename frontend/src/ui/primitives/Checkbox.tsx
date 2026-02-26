import { type JSX, Show, splitProps } from "solid-js";

export interface CheckboxProps extends Omit<
  JSX.InputHTMLAttributes<HTMLInputElement>,
  "type" | "onChange"
> {
  checked?: boolean;
  onChange?: (checked: boolean) => void;
  label?: string;
}

export function Checkbox(props: CheckboxProps): JSX.Element {
  const [local, rest] = splitProps(props, ["checked", "onChange", "label", "class"]);

  const input = (
    <input
      {...rest}
      type="checkbox"
      checked={local.checked}
      onChange={(e) => local.onChange?.(e.currentTarget.checked)}
      class={
        "h-4 w-4 rounded-cf-sm border-cf-border-input text-cf-accent " +
        "focus:ring-2 focus:ring-cf-focus-ring focus:ring-offset-2 cursor-pointer" +
        (local.class ? " " + local.class : "")
      }
    />
  );

  return (
    <Show when={local.label} fallback={input}>
      <label class="inline-flex cursor-pointer items-center gap-1.5 text-sm">
        {input}
        {local.label}
      </label>
    </Show>
  );
}
