import type { Component } from "solid-js";
import { createEffect, createMemo, createSignal, For, onCleanup, Show } from "solid-js";

import type { Item } from "./fuzzySearch";
import { fuzzyMatch } from "./fuzzySearch";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface AnchorRect {
  top: number;
  left: number;
  height: number;
}

interface AutocompletePopoverProps {
  items: Item[];
  query: string;
  visible: boolean;
  frequencyMap: Map<string, number>;
  onSelect: (item: Item) => void;
  onClose: () => void;
  anchorRect?: AnchorRect;
}

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const MAX_RESULTS = 10;

/** Category display prefixes for visual scannability. */
const CATEGORY_ICONS: Record<string, string> = {
  command: "/",
  file: "#",
  user: "@",
  agent: "@",
  mode: "/",
};

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

interface GroupedItems {
  category: string;
  items: Item[];
  /** Starting flat index for this group (for keyboard navigation). */
  startIndex: number;
}

/**
 * Group items by category, preserving insertion order.
 * Each group carries its `startIndex` within the full flat list.
 */
function groupByCategory(items: Item[]): GroupedItems[] {
  const map = new Map<string, Item[]>();
  for (const item of items) {
    const cat = item.category ?? "";
    const arr = map.get(cat);
    if (arr) {
      arr.push(item);
    } else {
      map.set(cat, [item]);
    }
  }
  const result: GroupedItems[] = [];
  let offset = 0;
  for (const [category, groupItems] of map) {
    result.push({ category, items: groupItems, startIndex: offset });
    offset += groupItems.length;
  }
  return result;
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

const AutocompletePopover: Component<AutocompletePopoverProps> = (props) => {
  const [selectedIndex, setSelectedIndex] = createSignal(0);
  let containerRef: HTMLDivElement | undefined;

  // Filtered + capped results.
  const filtered = createMemo(() =>
    fuzzyMatch(props.query, props.items, props.frequencyMap).slice(0, MAX_RESULTS),
  );

  // Grouped for display (with start indices for flat navigation).
  const groups = createMemo(() => groupByCategory(filtered()));

  // Total count for wrap-around navigation.
  const totalCount = createMemo(() => filtered().length);

  // Reset selection when the filtered list changes.
  createEffect(() => {
    void filtered();
    setSelectedIndex(0);
  });

  // Scroll the selected item into view.
  createEffect(() => {
    const idx = selectedIndex();
    if (!containerRef) return;
    const el = containerRef.querySelector(`[data-idx="${idx}"]`) as HTMLElement | null;
    el?.scrollIntoView({ block: "nearest" });
  });

  // ---- Keyboard handler (attached to document, capture phase) -------------

  function handleKeyDown(e: KeyboardEvent) {
    if (!props.visible) return;

    const len = totalCount();
    if (len === 0) return;

    if (e.key === "ArrowDown") {
      e.preventDefault();
      setSelectedIndex((i) => (i + 1) % len);
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      setSelectedIndex((i) => (i - 1 + len) % len);
    } else if (e.key === "Enter" || e.key === "Tab") {
      e.preventDefault();
      const items = filtered();
      const item = items[selectedIndex()];
      if (item) {
        props.onSelect(item);
      }
    } else if (e.key === "Escape") {
      e.preventDefault();
      props.onClose();
    }
  }

  // ---- Click-outside handler ----------------------------------------------

  function handleClickOutside(e: MouseEvent) {
    if (!props.visible) return;
    if (containerRef && !containerRef.contains(e.target as Node)) {
      props.onClose();
    }
  }

  // ---- Lifecycle: attach / detach global listeners -------------------------

  document.addEventListener("keydown", handleKeyDown, true);
  document.addEventListener("mousedown", handleClickOutside);

  onCleanup(() => {
    document.removeEventListener("keydown", handleKeyDown, true);
    document.removeEventListener("mousedown", handleClickOutside);
  });

  // ---- Positioning (above the anchor by default) --------------------------

  const popoverStyle = createMemo((): string => {
    const anchor = props.anchorRect;
    if (!anchor) {
      return "position:absolute;bottom:100%;left:0;";
    }
    return `position:fixed;top:${anchor.top}px;left:${anchor.left}px;transform:translateY(-100%);`;
  });

  // ---- Render --------------------------------------------------------------

  return (
    <Show when={props.visible && totalCount() > 0}>
      <div
        ref={containerRef}
        class="z-50 min-w-[200px] max-w-[320px] max-h-[280px] overflow-y-auto rounded-cf-md border border-cf-border bg-cf-bg-surface shadow-cf-lg"
        style={popoverStyle()}
        role="listbox"
        aria-label="Autocomplete suggestions"
      >
        <For each={groups()}>
          {(group) => (
            <>
              <Show when={group.category}>
                <div class="px-3 py-1 text-xs uppercase text-cf-text-muted">{group.category}</div>
              </Show>
              <For each={group.items}>
                {(item, itemIndex) => {
                  const flatIndex = () => group.startIndex + itemIndex();
                  return (
                    <div
                      data-idx={flatIndex()}
                      class={`flex cursor-pointer items-center gap-2 px-3 py-1.5 text-sm ${
                        flatIndex() === selectedIndex()
                          ? "bg-cf-accent/10 text-cf-text-primary"
                          : "text-cf-text-secondary hover:bg-cf-bg-surface-alt"
                      }`}
                      role="option"
                      aria-selected={flatIndex() === selectedIndex()}
                      onMouseEnter={() => setSelectedIndex(flatIndex())}
                      onClick={() => props.onSelect(item)}
                    >
                      <Show when={item.category ? CATEGORY_ICONS[item.category] : undefined}>
                        {(icon) => (
                          <span class="w-4 shrink-0 text-center text-cf-text-muted">{icon()}</span>
                        )}
                      </Show>
                      <span class="truncate">{item.label}</span>
                    </div>
                  );
                }}
              </For>
            </>
          )}
        </For>
      </div>
    </Show>
  );
};

export default AutocompletePopover;
