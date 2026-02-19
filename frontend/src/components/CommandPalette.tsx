import { useNavigate } from "@solidjs/router";
import { createEffect, createSignal, For, type JSX, onCleanup, onMount, Show } from "solid-js";

import { useTheme } from "~/components/ThemeProvider";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface Command {
  id: string;
  label: string;
  shortcut?: string;
  section: "navigation" | "actions" | "theme";
  action: () => void;
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const MOD = navigator.userAgent.includes("Mac") ? "\u2318" : "Ctrl";

function formatShortcut(shortcut: string): string {
  return shortcut.replace("Mod", MOD);
}

// ---------------------------------------------------------------------------
// CommandPalette
// ---------------------------------------------------------------------------

export function CommandPalette(): JSX.Element {
  const navigate = useNavigate();
  const { toggle } = useTheme();

  const [open, setOpen] = createSignal(false);
  const [query, setQuery] = createSignal("");
  const [selectedIndex, setSelectedIndex] = createSignal(0);

  let inputRef: HTMLInputElement | undefined;
  let listRef: HTMLDivElement | undefined;

  // ---- Commands -----------------------------------------------------------

  const commands: Command[] = [
    {
      id: "nav-dashboard",
      label: "Go to Dashboard",
      shortcut: "Mod+1",
      section: "navigation",
      action: () => navigate("/"),
    },
    {
      id: "nav-costs",
      label: "Go to Costs",
      shortcut: "Mod+2",
      section: "navigation",
      action: () => navigate("/costs"),
    },
    {
      id: "nav-models",
      label: "Go to Models",
      shortcut: "Mod+3",
      section: "navigation",
      action: () => navigate("/models"),
    },
    {
      id: "theme-toggle",
      label: "Toggle Theme",
      section: "theme",
      action: () => toggle(),
    },
    {
      id: "shortcut-help",
      label: "Show Keyboard Shortcuts",
      shortcut: "Mod+/",
      section: "actions",
      action: () => {
        /* opening the palette IS the action */
      },
    },
  ];

  // ---- Filtered list ------------------------------------------------------

  const filtered = (): Command[] => {
    const q = query().toLowerCase().trim();
    if (!q) return commands;
    return commands.filter((c) => c.label.toLowerCase().includes(q));
  };

  // Reset selection when filter changes
  createEffect(() => {
    const _items = filtered();
    void _items;
    setSelectedIndex(0);
  });

  // ---- Open / Close -------------------------------------------------------

  function openPalette() {
    setOpen(true);
    setQuery("");
    setSelectedIndex(0);
    // Focus input on next tick after render
    requestAnimationFrame(() => inputRef?.focus());
  }

  function closePalette() {
    setOpen(false);
  }

  function executeSelected() {
    const items = filtered();
    const cmd = items[selectedIndex()];
    if (cmd) {
      closePalette();
      cmd.action();
    }
  }

  // ---- Scroll selected into view ------------------------------------------

  createEffect(() => {
    const idx = selectedIndex();
    if (!listRef) return;
    const el = listRef.children[idx] as HTMLElement | undefined;
    el?.scrollIntoView({ block: "nearest" });
  });

  // ---- Global keyboard handler --------------------------------------------

  function handleGlobalKeydown(e: KeyboardEvent) {
    const mod = e.metaKey || e.ctrlKey;

    // Mod+K — toggle palette
    if (mod && e.key === "k") {
      e.preventDefault();
      if (open()) {
        closePalette();
      } else {
        openPalette();
      }
      return;
    }

    // Mod+/ — open palette (shortcut help)
    if (mod && e.key === "/") {
      e.preventDefault();
      openPalette();
      return;
    }

    // Mod+1/2/3 — direct navigation (only when palette is closed)
    if (mod && !open() && e.key >= "1" && e.key <= "3") {
      const idx = parseInt(e.key) - 1;
      const navCommands = commands.filter((c) => c.section === "navigation");
      if (navCommands[idx]) {
        e.preventDefault();
        navCommands[idx].action();
        return;
      }
    }

    // Escape — close palette
    if (e.key === "Escape" && open()) {
      e.preventDefault();
      closePalette();
    }
  }

  onMount(() => {
    document.addEventListener("keydown", handleGlobalKeydown);
  });

  onCleanup(() => {
    document.removeEventListener("keydown", handleGlobalKeydown);
  });

  // ---- Palette-internal keyboard handler ----------------------------------

  function handlePaletteKeydown(e: KeyboardEvent) {
    const items = filtered();
    const len = items.length;

    if (e.key === "ArrowDown") {
      e.preventDefault();
      setSelectedIndex((i) => (i + 1) % len);
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      setSelectedIndex((i) => (i - 1 + len) % len);
    } else if (e.key === "Enter") {
      e.preventDefault();
      executeSelected();
    }
  }

  // ---- Section labels -----------------------------------------------------

  const SECTION_LABELS: Record<string, string> = {
    navigation: "Navigation",
    actions: "Actions",
    theme: "Theme",
  };

  // Group filtered commands by section for display
  const grouped = (): { section: string; items: Command[] }[] => {
    const items = filtered();
    const map = new Map<string, Command[]>();
    for (const cmd of items) {
      const arr = map.get(cmd.section);
      if (arr) {
        arr.push(cmd);
      } else {
        map.set(cmd.section, [cmd]);
      }
    }
    const result: { section: string; items: Command[] }[] = [];
    for (const [section, cmds] of map) {
      result.push({ section, items: cmds });
    }
    return result;
  };

  // Flat index for a command (needed for selection tracking)
  const flatIndex = (cmd: Command): number => {
    return filtered().indexOf(cmd);
  };

  // ---- Render -------------------------------------------------------------

  return (
    <Show when={open()}>
      {/* Backdrop */}
      <div
        class="fixed inset-0 z-50 flex items-start justify-center bg-black/50 pt-[20vh]"
        onClick={closePalette}
        role="presentation"
      >
        {/* Palette */}
        <div
          class="w-full max-w-lg rounded-lg border border-gray-200 bg-white shadow-2xl dark:border-gray-700 dark:bg-gray-800"
          onClick={(e) => e.stopPropagation()}
          onKeyDown={handlePaletteKeydown}
          role="dialog"
          aria-label="Command palette"
          aria-modal="true"
        >
          {/* Search input */}
          <div class="flex items-center border-b border-gray-200 px-4 dark:border-gray-700">
            <span class="mr-2 text-gray-400 dark:text-gray-500" aria-hidden="true">
              /
            </span>
            <input
              ref={inputRef}
              type="text"
              class="w-full bg-transparent py-3 text-sm text-gray-900 placeholder-gray-400 outline-none dark:text-gray-100 dark:placeholder-gray-500"
              placeholder="Type a command..."
              value={query()}
              onInput={(e) => setQuery(e.currentTarget.value)}
              aria-label="Search commands"
              aria-activedescendant={
                filtered()[selectedIndex()] ? `cmd-${filtered()[selectedIndex()].id}` : undefined
              }
              role="combobox"
              aria-expanded="true"
              aria-controls="command-list"
              aria-autocomplete="list"
            />
            <kbd class="ml-2 rounded border border-gray-300 px-1.5 py-0.5 text-xs text-gray-400 dark:border-gray-600 dark:text-gray-500">
              esc
            </kbd>
          </div>

          {/* Command list */}
          <div ref={listRef} id="command-list" class="max-h-64 overflow-y-auto p-2" role="listbox">
            <Show
              when={filtered().length > 0}
              fallback={
                <p class="px-3 py-4 text-center text-sm text-gray-400 dark:text-gray-500">
                  No matching commands
                </p>
              }
            >
              <For each={grouped()}>
                {(group) => (
                  <>
                    <div class="mb-1 mt-2 px-3 text-xs font-medium text-gray-400 first:mt-0 dark:text-gray-500">
                      {SECTION_LABELS[group.section] ?? group.section}
                    </div>
                    <For each={group.items}>
                      {(cmd) => {
                        const idx = () => flatIndex(cmd);
                        return (
                          <div
                            id={`cmd-${cmd.id}`}
                            class={`flex cursor-pointer items-center justify-between rounded-md px-3 py-2 text-sm ${
                              idx() === selectedIndex()
                                ? "bg-blue-50 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400"
                                : "text-gray-700 hover:bg-gray-50 dark:text-gray-300 dark:hover:bg-gray-700"
                            }`}
                            role="option"
                            aria-selected={idx() === selectedIndex()}
                            onMouseEnter={() => setSelectedIndex(idx())}
                            onClick={() => {
                              closePalette();
                              cmd.action();
                            }}
                          >
                            <span>{cmd.label}</span>
                            <Show when={cmd.shortcut}>
                              <kbd class="rounded border border-gray-200 px-1.5 py-0.5 text-xs text-gray-400 dark:border-gray-600 dark:text-gray-500">
                                {formatShortcut(cmd.shortcut ?? "")}
                              </kbd>
                            </Show>
                          </div>
                        );
                      }}
                    </For>
                  </>
                )}
              </For>
            </Show>
          </div>

          {/* Footer hint */}
          <div class="flex items-center gap-3 border-t border-gray-200 px-4 py-2 text-xs text-gray-400 dark:border-gray-700 dark:text-gray-500">
            <span>
              <kbd class="rounded border border-gray-300 px-1 py-0.5 dark:border-gray-600">
                &uarr;&darr;
              </kbd>{" "}
              navigate
            </span>
            <span>
              <kbd class="rounded border border-gray-300 px-1 py-0.5 dark:border-gray-600">
                &crarr;
              </kbd>{" "}
              select
            </span>
            <span>
              <kbd class="rounded border border-gray-300 px-1 py-0.5 dark:border-gray-600">esc</kbd>{" "}
              close
            </span>
          </div>
        </div>
      </div>
    </Show>
  );
}
