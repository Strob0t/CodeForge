import { useNavigate } from "@solidjs/router";
import { createEffect, createSignal, For, type JSX, onCleanup, onMount, Show } from "solid-js";

import { useTheme } from "~/components/ThemeProvider";
import { useI18n } from "~/i18n";
import { Input } from "~/ui";

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
  const { t } = useI18n();

  const [open, setOpen] = createSignal(false);
  const [query, setQuery] = createSignal("");
  const [selectedIndex, setSelectedIndex] = createSignal(0);

  let inputRef: HTMLInputElement | undefined;
  let listRef: HTMLDivElement | undefined;

  // ---- Commands -----------------------------------------------------------

  const commands = (): Command[] => [
    {
      id: "nav-dashboard",
      label: t("palette.cmd.dashboard"),
      shortcut: "Mod+1",
      section: "navigation",
      action: () => navigate("/"),
    },
    {
      id: "nav-costs",
      label: t("palette.cmd.costs"),
      shortcut: "Mod+2",
      section: "navigation",
      action: () => navigate("/costs"),
    },
    {
      id: "nav-models",
      label: t("palette.cmd.models"),
      shortcut: "Mod+3",
      section: "navigation",
      action: () => navigate("/models"),
    },
    {
      id: "theme-toggle",
      label: t("palette.cmd.toggleTheme"),
      section: "theme",
      action: () => toggle(),
    },
    {
      id: "shortcut-help",
      label: t("palette.cmd.shortcuts"),
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
    if (!q) return commands();
    return commands().filter((c) => c.label.toLowerCase().includes(q));
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
      const navCommands = commands().filter((c) => c.section === "navigation");
      if (navCommands[idx]) {
        e.preventDefault();
        navCommands[idx].action();
        return;
      }
    }

    // Enter — execute selected command (when palette is open)
    if (e.key === "Enter" && open()) {
      e.preventDefault();
      executeSelected();
      return;
    }

    // ArrowDown / ArrowUp — navigate commands (when palette is open)
    if ((e.key === "ArrowDown" || e.key === "ArrowUp") && open()) {
      e.preventDefault();
      const items = filtered();
      const len = items.length;
      if (len === 0) return;
      if (e.key === "ArrowDown") {
        setSelectedIndex((i) => (i + 1) % len);
      } else {
        setSelectedIndex((i) => (i - 1 + len) % len);
      }
      return;
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
    } else if (e.key === "Tab") {
      // Trap focus inside the palette — prevent Tab from escaping
      e.preventDefault();
      inputRef?.focus();
    }
  }

  // ---- Section labels -----------------------------------------------------

  const SECTION_LABELS: Record<string, () => string> = {
    navigation: () => t("palette.section.navigation"),
    actions: () => t("palette.section.actions"),
    theme: () => t("palette.section.theme"),
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
          class="w-full max-w-lg rounded-cf-lg border border-cf-border bg-cf-bg-surface shadow-cf-lg"
          onClick={(e) => e.stopPropagation()}
          onKeyDown={handlePaletteKeydown}
          role="dialog"
          aria-label={t("palette.title")}
          aria-modal="true"
        >
          {/* Search input */}
          <div class="flex items-center border-b border-cf-border px-4">
            <span class="mr-2 text-cf-text-muted" aria-hidden="true">
              /
            </span>
            <Input
              ref={inputRef}
              type="text"
              class="!border-0 !bg-transparent !px-0 !py-3 !ring-0 !shadow-none !rounded-none"
              placeholder={t("palette.placeholder")}
              value={query()}
              onInput={(e: InputEvent & { currentTarget: HTMLInputElement }) =>
                setQuery(e.currentTarget.value)
              }
              onKeyDown={handlePaletteKeydown}
              aria-label={t("palette.searchLabel")}
              aria-activedescendant={
                filtered()[selectedIndex()] ? `cmd-${filtered()[selectedIndex()].id}` : undefined
              }
              role="combobox"
              aria-expanded="true"
              aria-controls="command-list"
              aria-autocomplete="list"
            />
            <kbd class="ml-2 rounded-cf-sm border border-cf-border px-1.5 py-0.5 text-xs text-cf-text-muted">
              esc
            </kbd>
          </div>

          {/* Command list */}
          <div ref={listRef} id="command-list" class="max-h-64 overflow-y-auto p-2" role="listbox">
            <Show
              when={filtered().length > 0}
              fallback={
                <p class="px-3 py-4 text-center text-sm text-cf-text-muted">
                  {t("palette.noResults")}
                </p>
              }
            >
              <For each={grouped()}>
                {(group) => (
                  <>
                    <div class="mb-1 mt-2 px-3 text-xs font-medium text-cf-text-muted first:mt-0">
                      {SECTION_LABELS[group.section]?.() ?? group.section}
                    </div>
                    <For each={group.items}>
                      {(cmd) => {
                        const idx = () => flatIndex(cmd);
                        return (
                          <div
                            id={`cmd-${cmd.id}`}
                            class={`flex cursor-pointer items-center justify-between rounded-cf-md px-3 py-2 text-sm ${
                              idx() === selectedIndex()
                                ? "bg-cf-info-bg text-cf-accent"
                                : "text-cf-text-secondary hover:bg-cf-bg-surface-alt"
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
                              <kbd class="rounded-cf-sm border border-cf-border px-1.5 py-0.5 text-xs text-cf-text-muted">
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
          <div class="flex items-center gap-3 border-t border-cf-border px-4 py-2 text-xs text-cf-text-muted">
            <span>
              <kbd class="rounded-cf-sm border border-cf-border px-1 py-0.5">&uarr;&darr;</kbd>{" "}
              {t("palette.navigate")}
            </span>
            <span>
              <kbd class="rounded-cf-sm border border-cf-border px-1 py-0.5">&crarr;</kbd>{" "}
              {t("palette.select")}
            </span>
            <span>
              <kbd class="rounded-cf-sm border border-cf-border px-1 py-0.5">esc</kbd>{" "}
              {t("palette.close")}
            </span>
          </div>
        </div>
      </div>
    </Show>
  );
}
