import { createResource, createUniqueId, For, type JSX } from "solid-js";

import { api } from "~/api/client";

export interface ModelComboboxProps {
  /** Current model value. */
  value: string;
  /** Called when the user types or selects a model. */
  onInput: (value: string) => void;
  /** HTML id for the input element. */
  id?: string;
  /** Placeholder text. */
  placeholder?: string;
  /** Additional CSS classes for the input. */
  class?: string;
  /** Whether the input is required. */
  required?: boolean;
}

/**
 * Text input with a datalist populated from discovered LLM models.
 * Users can type freely or pick from the dropdown.
 */
export function ModelCombobox(props: ModelComboboxProps): JSX.Element {
  const listId = createUniqueId();

  const [models] = createResource(() =>
    api.llm.discover().then((r) => r.models.map((m) => m.model_name)),
  );

  return (
    <>
      <input
        id={props.id}
        type="text"
        list={listId}
        value={props.value}
        onInput={(e) => props.onInput(e.currentTarget.value)}
        placeholder={props.placeholder ?? "e.g. openai/gpt-4o"}
        required={props.required}
        class={
          "rounded-cf border border-cf-border bg-cf-bg-surface px-3 py-1.5 text-sm text-cf-text-primary placeholder:text-cf-text-muted focus:border-cf-accent focus:outline-none focus:ring-1 focus:ring-cf-accent" +
          (props.class ? " " + props.class : "")
        }
        autocomplete="off"
      />
      <datalist id={listId}>
        <For each={models() ?? []}>{(name) => <option value={name} />}</For>
      </datalist>
    </>
  );
}
