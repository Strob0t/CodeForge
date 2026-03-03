import {
  createEffect,
  createMemo,
  createResource,
  createSignal,
  For,
  type JSX,
  on,
  onCleanup,
  onMount,
  Show,
} from "solid-js";

import { api } from "~/api/client";
import type { DiscoveredModel } from "~/api/types";

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

interface GroupedModels {
  provider: string;
  models: DiscoveredModel[];
}

/** Case-insensitive substring match across model_name and provider. */
function fuzzyMatch(model: DiscoveredModel, query: string): boolean {
  const q = query.toLowerCase();
  if (model.model_name.toLowerCase().includes(q)) return true;
  if (model.provider?.toLowerCase().includes(q)) return true;
  return false;
}

/** Group a flat model list by provider, sorted alphabetically. */
function groupByProvider(models: DiscoveredModel[]): GroupedModels[] {
  const map = new Map<string, DiscoveredModel[]>();
  for (const m of models) {
    const key = m.provider ?? m.source ?? "other";
    const list = map.get(key);
    if (list) list.push(m);
    else map.set(key, [m]);
  }
  return Array.from(map.entries())
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([provider, models]) => ({ provider, models }));
}

/** Flatten grouped models into a single ordered list (for keyboard nav). */
function flattenGroups(groups: GroupedModels[]): DiscoveredModel[] {
  const result: DiscoveredModel[] = [];
  for (const g of groups) {
    for (const m of g.models) result.push(m);
  }
  return result;
}

/**
 * Model selector with fuzzy-search dropdown populated from LiteLLM discover.
 * Users can type freely or pick from the grouped, filterable list.
 */
export function ModelCombobox(props: ModelComboboxProps): JSX.Element {
  let containerRef: HTMLDivElement | undefined;
  let inputRef: HTMLInputElement | undefined;
  let listRef: HTMLDivElement | undefined;

  const [isOpen, setIsOpen] = createSignal(false);
  const [highlightedIndex, setHighlightedIndex] = createSignal(-1);

  const [allModels] = createResource(() => api.llm.discover().then((r) => r.models));

  const filteredGroups = createMemo<GroupedModels[]>(() => {
    const models = allModels() ?? [];
    const query = props.value.trim();
    const matched = query.length > 0 ? models.filter((m) => fuzzyMatch(m, query)) : models;
    return groupByProvider(matched);
  });

  const flatList = createMemo(() => flattenGroups(filteredGroups()));

  // Reset highlight when filter changes
  createEffect(
    on(
      () => props.value,
      () => setHighlightedIndex(-1),
    ),
  );

  // Scroll highlighted item into view
  createEffect(
    on(highlightedIndex, (idx) => {
      if (idx < 0 || !listRef) return;
      const el = listRef.querySelector(`[data-idx="${idx}"]`);
      if (el) el.scrollIntoView({ block: "nearest" });
    }),
  );

  // Click-outside handler
  onMount(() => {
    const handler = (e: MouseEvent) => {
      if (containerRef && !containerRef.contains(e.target as Node)) {
        setIsOpen(false);
      }
    };
    document.addEventListener("mousedown", handler);
    onCleanup(() => document.removeEventListener("mousedown", handler));
  });

  function selectModel(name: string): void {
    props.onInput(name);
    setIsOpen(false);
    inputRef?.blur();
  }

  function handleKeyDown(e: KeyboardEvent): void {
    const list = flatList();
    if (!isOpen()) {
      if (e.key === "ArrowDown" || e.key === "ArrowUp") {
        setIsOpen(true);
        e.preventDefault();
      }
      return;
    }

    switch (e.key) {
      case "ArrowDown":
        e.preventDefault();
        setHighlightedIndex((i) => (i < list.length - 1 ? i + 1 : 0));
        break;
      case "ArrowUp":
        e.preventDefault();
        setHighlightedIndex((i) => (i > 0 ? i - 1 : list.length - 1));
        break;
      case "Enter":
        e.preventDefault();
        if (highlightedIndex() >= 0 && highlightedIndex() < list.length) {
          selectModel(list[highlightedIndex()].model_name);
        } else {
          setIsOpen(false);
        }
        break;
      case "Escape":
        e.preventDefault();
        setIsOpen(false);
        break;
      case "Tab":
        setIsOpen(false);
        break;
    }
  }

  // Build a flat index counter across groups for data-idx
  function getGlobalIndex(groups: GroupedModels[], groupIdx: number, modelIdx: number): number {
    let idx = 0;
    for (let g = 0; g < groupIdx; g++) idx += groups[g].models.length;
    return idx + modelIdx;
  }

  return (
    <div ref={containerRef} class="relative">
      <input
        ref={inputRef}
        id={props.id}
        type="text"
        value={props.value}
        onInput={(e) => {
          props.onInput(e.currentTarget.value);
          setIsOpen(true);
        }}
        onFocus={() => setIsOpen(true)}
        onKeyDown={handleKeyDown}
        placeholder={props.placeholder ?? "e.g. openai/gpt-4o"}
        required={props.required}
        role="combobox"
        aria-expanded={isOpen()}
        aria-autocomplete="list"
        autocomplete="off"
        class={
          "block w-full rounded-cf border border-cf-border bg-cf-bg-surface px-3 py-1.5 text-sm text-cf-text-primary placeholder:text-cf-text-muted focus:border-cf-accent focus:outline-none focus:ring-1 focus:ring-cf-accent" +
          (props.class ? " " + props.class : "")
        }
      />

      <Show when={isOpen()}>
        <div
          ref={listRef}
          role="listbox"
          class="absolute z-50 mt-1 max-h-64 w-full overflow-auto rounded-cf border border-cf-border bg-cf-bg-surface shadow-lg"
        >
          {/* Loading state */}
          <Show when={allModels.loading}>
            <div class="px-3 py-2 text-sm text-cf-text-muted">Loading models...</div>
          </Show>

          {/* Empty state */}
          <Show when={!allModels.loading && flatList().length === 0}>
            <div class="px-3 py-2 text-sm text-cf-text-muted">No models found</div>
          </Show>

          {/* Grouped results */}
          <For each={filteredGroups()}>
            {(group, groupIdx) => (
              <>
                <div class="sticky top-0 bg-cf-bg-secondary px-3 py-1 text-xs font-semibold uppercase tracking-wider text-cf-text-muted">
                  {group.provider}
                </div>
                <For each={group.models}>
                  {(model, modelIdx) => {
                    const globalIdx = () =>
                      getGlobalIndex(filteredGroups(), groupIdx(), modelIdx());
                    return (
                      <div
                        data-idx={globalIdx()}
                        role="option"
                        aria-selected={highlightedIndex() === globalIdx()}
                        class={
                          "flex cursor-pointer items-center gap-2 px-3 py-1.5 text-sm" +
                          (highlightedIndex() === globalIdx()
                            ? " bg-cf-accent/10 text-cf-accent"
                            : " text-cf-text-primary hover:bg-cf-bg-hover")
                        }
                        onMouseEnter={() => setHighlightedIndex(globalIdx())}
                        onMouseDown={(e) => {
                          e.preventDefault(); // prevent blur before click
                          selectModel(model.model_name);
                        }}
                      >
                        {/* Status dot */}
                        <span
                          class={
                            "inline-block h-2 w-2 flex-shrink-0 rounded-full " +
                            (model.status === "reachable" ? "bg-green-500" : "bg-red-400")
                          }
                          title={model.status}
                        />

                        {/* Model name + metadata */}
                        <div class="min-w-0 flex-1">
                          <div class="truncate font-mono text-xs">{model.model_name}</div>
                        </div>

                        {/* Source badge */}
                        <span class="flex-shrink-0 rounded bg-cf-bg-secondary px-1.5 py-0.5 text-[10px] text-cf-text-muted">
                          {model.source}
                        </span>
                      </div>
                    );
                  }}
                </For>
              </>
            )}
          </For>
        </div>
      </Show>
    </div>
  );
}
