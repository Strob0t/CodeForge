import { type JSX, splitProps } from "solid-js";

export interface CheckboxProps extends Omit<JSX.InputHTMLAttributes<HTMLInputElement>, "type"> {
  checked?: boolean;
  onChange?: (checked: boolean) => void;
}

export function Checkbox(props: CheckboxProps): JSX.Element {
  const [local, rest] = splitProps(props, ["checked", "onChange", "class"]);

  return (
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
}
