import { createSignal, createUniqueId, For, type JSX } from "solid-js";

import { Button } from "../primitives/Button";

export interface TagInputProps {
  /** Currently selected values. */
  values: string[];
  /** Called when the tag set changes. */
  onChange: (values: string[]) => void;
  /** Suggested options shown in the datalist dropdown. */
  suggestions?: string[];
  /** Placeholder text for the input. */
  placeholder?: string;
  /** HTML id for the input element. */
  id?: string;
  /** Additional CSS classes for the wrapper. */
  class?: string;
}

/**
 * Multi-value input that displays selected values as removable tags
 * and offers a datalist dropdown for suggestions.
 */
export function TagInput(props: TagInputProps): JSX.Element {
  const listId = createUniqueId();
  const [input, setInput] = createSignal("");

  const addTag = (raw: string) => {
    const value = raw.trim();
    if (value === "" || props.values.includes(value)) return;
    props.onChange([...props.values, value]);
    setInput("");
  };

  const removeTag = (value: string) => {
    props.onChange(props.values.filter((v) => v !== value));
  };

  const handleKeyDown: JSX.EventHandlerUnion<HTMLInputElement, KeyboardEvent> = (e) => {
    if (e.key === "Enter" || e.key === ",") {
      e.preventDefault();
      addTag(input());
    }
    if (e.key === "Backspace" && input() === "" && props.values.length > 0) {
      removeTag(props.values[props.values.length - 1]);
    }
  };

  const handleInput: JSX.EventHandlerUnion<HTMLInputElement, InputEvent> = (e) => {
    const val = e.currentTarget.value;
    // If user picks from datalist (value contains comma or matches a suggestion exactly), add it
    if (val.includes(",")) {
      for (const part of val.split(",")) {
        addTag(part);
      }
      return;
    }
    setInput(val);
  };

  // Filter suggestions to exclude already-selected values.
  const filteredSuggestions = () =>
    (props.suggestions ?? []).filter((s) => !props.values.includes(s));

  return (
    <div
      class={
        "flex flex-wrap items-center gap-1.5 rounded-cf border border-cf-border bg-cf-bg-surface px-2 py-1.5 text-sm focus-within:border-cf-accent focus-within:ring-1 focus-within:ring-cf-accent" +
        (props.class ? " " + props.class : "")
      }
    >
      <For each={props.values}>
        {(tag) => (
          <span class="inline-flex items-center gap-1 rounded bg-cf-accent/15 px-2 py-0.5 text-xs text-cf-accent">
            {tag}
            <Button
              variant="icon"
              size="xs"
              class="ml-0.5"
              onClick={() => removeTag(tag)}
              aria-label={`Remove ${tag}`}
            >
              x
            </Button>
          </span>
        )}
      </For>
      <input
        id={props.id}
        type="text"
        list={listId}
        value={input()}
        onKeyDown={handleKeyDown}
        onInput={handleInput}
        placeholder={props.values.length === 0 ? props.placeholder : undefined}
        class="min-w-[120px] flex-1 border-none bg-transparent p-0 text-sm text-cf-text-primary placeholder:text-cf-text-muted focus:outline-none"
        autocomplete="off"
      />
      <datalist id={listId}>
        <For each={filteredSuggestions()}>{(name) => <option value={name} />}</For>
      </datalist>
    </div>
  );
}
