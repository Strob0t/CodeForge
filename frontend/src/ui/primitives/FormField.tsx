import { type JSX, Show, splitProps } from "solid-js";

import { Label } from "./Label";

export interface FormFieldProps {
  label: string;
  id: string;
  required?: boolean;
  help?: string;
  error?: string;
  class?: string;
  children: JSX.Element;
}

export function FormField(props: FormFieldProps): JSX.Element {
  const [local] = splitProps(props, [
    "label",
    "id",
    "required",
    "help",
    "error",
    "class",
    "children",
  ]);

  const helpId = (): string => local.id + "-help";
  const errorId = (): string => local.id + "-error";

  return (
    <div class={"space-y-1" + (local.class ? " " + local.class : "")}>
      <Label for={local.id} required={local.required}>
        {local.label}
      </Label>
      <div aria-describedby={local.error ? errorId() : local.help ? helpId() : undefined}>
        {local.children}
      </div>
      <Show when={local.error}>
        <p id={errorId()} class="text-xs text-cf-danger-fg" role="alert">
          {local.error}
        </p>
      </Show>
      <Show when={local.help && !local.error}>
        <p id={helpId()} class="text-xs text-cf-text-muted">
          {local.help}
        </p>
      </Show>
    </div>
  );
}
