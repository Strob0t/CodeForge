import type { Component } from "solid-js";
import { createMemo, createResource, createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import { useFrequencyTracker } from "~/hooks/useFrequencyTracker";

import AutocompletePopover from "./AutocompletePopover";
import { useCommandStore } from "./commandStore";
import type { Item } from "./fuzzySearch";
import TokenBadge from "./TokenBadge";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface SelectedReference {
  type: "@" | "#" | "/";
  id: string;
  label: string;
}

interface ConversationRef {
  id: string;
  title: string;
}

interface ChatInputProps {
  value: string;
  onInput: (value: string) => void;
  onSubmit: () => void;
  disabled?: boolean;
  placeholder?: string;
  onReferencesChange?: (refs: SelectedReference[]) => void;
  /** Project ID used to fetch file list for @ autocomplete. */
  projectId?: string;
  /** Conversations available for # autocomplete. */
  conversations?: ConversationRef[];
}

type TriggerChar = "@" | "#" | "/";

interface TriggerState {
  char: TriggerChar;
  query: string;
  /** Character index of the trigger in the input value. */
  startIndex: number;
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const TRIGGER_CHARS = new Set<string>(["@", "#", "/"]);

/**
 * Scan backwards from the cursor position to find the nearest trigger
 * character (`@`, `#`, or `/`) that sits at position 0 or is preceded
 * by whitespace.  Returns `null` if no valid trigger is found.
 */
function detectTrigger(text: string, cursorPos: number): TriggerState | null {
  // Walk backwards from the cursor to find a trigger character.
  for (let i = cursorPos - 1; i >= 0; i--) {
    const ch = text[i];

    // If we hit whitespace before finding a trigger, stop — no active trigger.
    if (ch === " " || ch === "\n" || ch === "\t") return null;

    if (TRIGGER_CHARS.has(ch)) {
      // Valid only if at position 0 or preceded by whitespace.
      if (i === 0 || text[i - 1] === " " || text[i - 1] === "\n" || text[i - 1] === "\t") {
        return {
          char: ch as TriggerChar,
          query: text.slice(i + 1, cursorPos),
          startIndex: i,
        };
      }
      // Trigger char exists but is not at a word boundary — invalid.
      return null;
    }
  }

  return null;
}

/**
 * Map a trigger character to a TokenBadge-compatible type.
 * TokenBadge only supports "@" | "#", so "/" commands use "#" styling.
 */
function badgeType(trigger: TriggerChar): "@" | "#" {
  return trigger === "@" ? "@" : "#";
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

const ChatInput: Component<ChatInputProps> = (props) => {
  const { commands } = useCommandStore();
  const frequencyTracker = useFrequencyTracker("chat-autocomplete");

  const [references, setReferences] = createSignal<SelectedReference[]>([]);
  const [trigger, setTrigger] = createSignal<TriggerState | null>(null);

  let textareaRef: HTMLTextAreaElement | undefined;

  // --- Project files for @ autocomplete (full tree, cached per projectId) ---

  const [fileItems] = createResource(
    () => props.projectId,
    async (pid): Promise<Item[]> => {
      if (!pid) return [];
      try {
        const entries = await api.files.tree(pid, 10000);
        return entries
          .filter((entry) => !entry.is_dir)
          .map(
            (entry): Item => ({
              id: entry.path,
              label: entry.path,
              category: "file",
            }),
          );
      } catch {
        return [];
      }
    },
  );

  // --- Conversation items for # autocomplete ---

  const conversationItems = createMemo((): Item[] =>
    (props.conversations ?? []).map(
      (conv): Item => ({
        id: conv.id,
        label: conv.title,
        category: "conversation",
      }),
    ),
  );

  // --- Items for the current trigger ---

  const autocompleteItems = createMemo((): Item[] => {
    const t = trigger();
    if (!t) return [];

    switch (t.char) {
      case "/":
        return commands();
      case "@":
        return fileItems() ?? [];
      case "#":
        return conversationItems();
      default:
        return [];
    }
  });

  const frequencyMap = createMemo(() => frequencyTracker.getAll());

  // --- Trigger detection on input ---

  function handleInput(e: InputEvent & { currentTarget: HTMLTextAreaElement }) {
    const value = e.currentTarget.value;
    props.onInput(value);

    const cursorPos = e.currentTarget.selectionStart ?? value.length;
    setTrigger(detectTrigger(value, cursorPos));

    // Sync references: remove badges whose trigger+label no longer appears in text.
    const current = references();
    if (current.length > 0) {
      const kept = current.filter((ref) => value.includes(`${ref.type}${ref.label}`));
      if (kept.length !== current.length) {
        setReferences(kept);
        props.onReferencesChange?.(kept);
      }
    }
  }

  // --- Autocomplete selection ---

  function handleSelect(item: Item) {
    const t = trigger();
    if (!t || !textareaRef) return;

    frequencyTracker.track(item.id);

    // Replace trigger + query text with the completed reference label.
    const before = props.value.slice(0, t.startIndex);
    const after = props.value.slice(t.startIndex + 1 + t.query.length);
    const inserted = `${t.char}${item.label} `;
    const newValue = before + inserted + after;
    props.onInput(newValue);

    // Add to selected references.
    const ref: SelectedReference = {
      type: t.char,
      id: item.id,
      label: item.label,
    };
    const updated = [...references(), ref];
    setReferences(updated);
    props.onReferencesChange?.(updated);

    // Close popover.
    setTrigger(null);

    // Restore cursor position after the inserted text.
    const newCursor = before.length + inserted.length;
    requestAnimationFrame(() => {
      textareaRef?.focus();
      textareaRef?.setSelectionRange(newCursor, newCursor);
    });
  }

  function handleAutocompleteClose() {
    setTrigger(null);
  }

  // --- Reference removal ---

  function removeReference(index: number) {
    const updated = references().filter((_, i) => i !== index);
    setReferences(updated);
    props.onReferencesChange?.(updated);
  }

  // --- Keyboard handling ---

  function handleKeyDown(e: KeyboardEvent) {
    // When autocomplete is open, let AutocompletePopover handle keyboard events
    // via its document-level capture listener.
    if (trigger() && autocompleteItems().length > 0) {
      return;
    }

    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      props.onSubmit();
    }
  }

  // --- Render ---

  return (
    <div class="relative flex flex-col">
      {/* Token badges for selected references */}
      <Show when={references().length > 0}>
        <div class="flex flex-wrap gap-1 px-1 pb-1">
          <For each={references()}>
            {(ref, index) => (
              <TokenBadge
                type={badgeType(ref.type)}
                label={ref.label}
                onRemove={() => removeReference(index())}
              />
            )}
          </For>
        </div>
      </Show>

      {/* Textarea */}
      <div class="relative">
        <textarea
          ref={textareaRef}
          class="flex-1 w-full rounded-cf-md border border-cf-border bg-cf-bg-surface px-3 py-2 text-sm text-cf-text-primary placeholder-cf-text-muted focus:border-cf-accent focus:ring-1 focus:ring-cf-accent resize-none"
          rows={2}
          placeholder={props.placeholder}
          value={props.value}
          onInput={handleInput}
          onKeyDown={handleKeyDown}
          disabled={props.disabled}
        />

        {/* Autocomplete popover anchored above the textarea */}
        <AutocompletePopover
          items={autocompleteItems()}
          query={trigger()?.query ?? ""}
          visible={trigger() !== null}
          frequencyMap={frequencyMap()}
          onSelect={handleSelect}
          onClose={handleAutocompleteClose}
        />
      </div>
    </div>
  );
};

export default ChatInput;
