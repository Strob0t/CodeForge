import { useNavigate } from "@solidjs/router";
import { createEffect, createSignal, For, type JSX, onCleanup, Show } from "solid-js";

import { useSidebar } from "~/components/SidebarProvider";
import { useTheme } from "~/components/ThemeProvider";
import { useI18n } from "~/i18n";
import { useShortcuts } from "~/shortcuts";
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
// CommandPalette
// ---------------------------------------------------------------------------

export function CommandPalette(): JSX.Element {
  const navigate = useNavigate();
  const { toggle: toggleTheme } = useTheme();
  const { toggle: toggleSidebar } = useSidebar();
  const { registerAction, formatCombo, shortcuts: allShortcuts } = useShortcuts();
  const { t } = useI18n();

  const [open, setOpen] = createSignal(false);
  const [query, setQuery] = createSignal("");
  const [selectedIndex, setSelectedIndex] = createSignal(0);

  let inputRef: HTMLInputElement | undefined;
  let listRef: HTMLDivElement | undefined;

  // ---- Open / Close ---------------------------------------------------------

  function openPalette() {
    setOpen(true);
    setQuery("");
    setSelectedIndex(0);
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

  // ---- Register shortcut actions via the registry ---------------------------

  // Helper to get the display string for a shortcut id
  function comboLabel(id: string): string | undefined {
    const def = allShortcuts().find((s) => s.id === id);
    return def ? formatCombo(def.combo) : undefined;
  }

  // Register all shortcut actions
  const cleanups: (() => void)[] = [];

  cleanups.push(
    // eslint-disable-next-line solid/reactivity -- callback reads signal at invocation time
    registerAction("palette.open", () => {
      if (open()) closePalette();
      else openPalette();
    }),
  );
  cleanups.push(registerAction("palette.help", () => openPalette()));
  cleanups.push(
    // eslint-disable-next-line solid/reactivity -- callback reads signal at invocation time
    registerAction("nav.dashboard", () => {
      if (!open()) navigate("/");
    }),
  );
  cleanups.push(
    // eslint-disable-next-line solid/reactivity -- callback reads signal at invocation time
    registerAction("nav.costs", () => {
      if (!open()) navigate("/costs");
    }),
  );
  cleanups.push(
    // eslint-disable-next-line solid/reactivity -- callback reads signal at invocation time
    registerAction("nav.models", () => {
      if (!open()) navigate("/models");
    }),
  );
  cleanups.push(registerAction("sidebar.toggle", () => toggleSidebar()));
  cleanups.push(registerAction("theme.toggle", () => toggleTheme()));

  onCleanup(() => {
    for (const fn of cleanups) fn();
  });

  // ---- Commands (for display in the palette) --------------------------------

  const commands = (): Command[] => [
    {
      id: "nav-dashboard",
      label: t("palette.cmd.dashboard"),
      shortcut: comboLabel("nav.dashboard"),
      section: "navigation",
      action: () => navigate("/"),
    },
    {
      id: "nav-costs",
      label: t("palette.cmd.costs"),
      shortcut: comboLabel("nav.costs"),
      section: "navigation",
      action: () => navigate("/costs"),
    },
    {
      id: "nav-models",
      label: t("palette.cmd.models"),
      shortcut: comboLabel("nav.models"),
      section: "navigation",
      action: () => navigate("/models"),
    },
    {
      id: "theme-toggle",
      label: t("palette.cmd.toggleTheme"),
      shortcut: comboLabel("theme.toggle"),
      section: "theme",
      action: () => toggleTheme(),
    },
    {
      id: "sidebar-toggle",
      label: t("sidebar.toggle"),
      shortcut: comboLabel("sidebar.toggle"),
      section: "actions",
      action: () => toggleSidebar(),
    },
    {
      id: "shortcut-help",
      label: t("palette.cmd.shortcuts"),
      shortcut: comboLabel("palette.help"),
      section: "actions",
      action: () => {
        /* opening the palette IS the action */
      },
    },
  ];

  // ---- Filtered list --------------------------------------------------------

  const filtered = (): Command[] => {
    const q = query().toLowerCase().trim();
    if (!q) return commands();
    return commands().filter((c) => c.label.toLowerCase().includes(q));
  };

  createEffect(() => {
    const _items = filtered();
    void _items;
    setSelectedIndex(0);
  });

  // ---- Scroll selected into view --------------------------------------------

  createEffect(() => {
    const idx = selectedIndex();
    if (!listRef) return;
    const el = listRef.children[idx] as HTMLElement | undefined;
    el?.scrollIntoView({ block: "nearest" });
  });

  // ---- Palette-internal keyboard handler ------------------------------------

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
    } else if (e.key === "Escape") {
      e.preventDefault();
      closePalette();
    } else if (e.key === "Tab") {
      e.preventDefault();
      inputRef?.focus();
    }
  }

  // ---- Section labels -------------------------------------------------------

  const SECTION_LABELS: Record<string, () => string> = {
    navigation: () => t("palette.section.navigation"),
    actions: () => t("palette.section.actions"),
    theme: () => t("palette.section.theme"),
  };

  const grouped = (): { section: string; items: Command[] }[] => {
    const items = filtered();
    const map = new Map<string, Command[]>();
    for (const cmd of items) {
      const arr = map.get(cmd.section);
      if (arr) arr.push(cmd);
      else map.set(cmd.section, [cmd]);
    }
    const result: { section: string; items: Command[] }[] = [];
    for (const [section, cmds] of map) {
      result.push({ section, items: cmds });
    }
    return result;
  };

  const flatIndex = (cmd: Command): number => {
    return filtered().indexOf(cmd);
  };

  // ---- Render ---------------------------------------------------------------

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
                            tabIndex={-1}
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
                                {cmd.shortcut}
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
