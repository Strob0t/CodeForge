import { createSignal, For, type JSX, Show } from "solid-js";

import { useI18n } from "~/i18n";
import type { KeyCombo, ShortcutDefinition, ShortcutScope } from "~/shortcuts";
import { combosEqual, useShortcuts } from "~/shortcuts";
import { Button } from "~/ui";

import { ShortcutRecorder } from "./ShortcutRecorder";

// ---------------------------------------------------------------------------
// ShortcutsSection — Settings page section for keyboard shortcuts
// ---------------------------------------------------------------------------

const SCOPE_ORDER: ShortcutScope[] = ["global", "chat", "editor"];

const SCOPE_LABEL_KEYS: Record<ShortcutScope, string> = {
  global: "settings.shortcuts.scope.global",
  palette: "settings.shortcuts.scope.palette",
  chat: "settings.shortcuts.scope.chat",
  editor: "settings.shortcuts.scope.editor",
};

export function ShortcutsSection(): JSX.Element {
  const { t } = useI18n();
  const { shortcuts, updateCombo, resetOne, resetAll, findConflict, formatCombo } = useShortcuts();

  const [editingId, setEditingId] = createSignal<string | null>(null);
  const [conflict, setConflict] = createSignal<{
    shortcut: ShortcutDefinition;
    pendingCombo: KeyCombo;
    targetId: string;
  } | null>(null);

  function groupedByScope(): { scope: ShortcutScope; items: ShortcutDefinition[] }[] {
    const all = shortcuts();
    const groups: { scope: ShortcutScope; items: ShortcutDefinition[] }[] = [];
    for (const scope of SCOPE_ORDER) {
      const items = all.filter((s) => s.scope === scope);
      if (items.length > 0) {
        groups.push({ scope, items });
      }
    }
    return groups;
  }

  function handleRecord(id: string, combo: KeyCombo): void {
    const target = shortcuts().find((s) => s.id === id);
    if (!target) return;
    const existing = findConflict(combo, target.scope, id);
    if (existing) {
      setConflict({ shortcut: existing, pendingCombo: combo, targetId: id });
      setEditingId(null);
      return;
    }
    updateCombo(id, combo);
    setEditingId(null);
    setConflict(null);
  }

  function handleOverride(): void {
    const c = conflict();
    if (!c) return;
    // Reset the conflicting shortcut, then apply the new combo
    resetOne(c.shortcut.id);
    updateCombo(c.targetId, c.pendingCombo);
    setConflict(null);
  }

  function isCustomized(def: ShortcutDefinition): boolean {
    return !combosEqual(def.combo, def.defaultCombo);
  }

  return (
    <section>
      <div class="mb-4 flex items-center justify-between">
        <p class="text-sm text-cf-text-muted">{t("settings.shortcuts.description")}</p>
        <Button variant="ghost" size="sm" onClick={resetAll}>
          {t("settings.shortcuts.resetAll")}
        </Button>
      </div>

      {/* Conflict warning */}
      <Show when={conflict()}>
        {(c) => (
          <div
            class="mb-4 flex items-center gap-3 rounded-cf-md border border-yellow-300 bg-yellow-50 px-4 py-2 text-sm dark:border-yellow-700 dark:bg-yellow-900/20"
            role="alert"
          >
            <span class="flex-1">
              {t("settings.shortcuts.conflict", {
                action: t(c().shortcut.labelKey as Parameters<typeof t>[0]),
              })}
            </span>
            <Button variant="danger" size="sm" onClick={handleOverride}>
              {t("settings.shortcuts.override")}
            </Button>
            <Button variant="ghost" size="sm" onClick={() => setConflict(null)}>
              {t("settings.shortcuts.cancel")}
            </Button>
          </div>
        )}
      </Show>

      <For each={groupedByScope()}>
        {(group) => (
          <div class="mb-6">
            <h3 class="mb-2 text-xs font-medium uppercase tracking-wider text-cf-text-muted">
              {t(SCOPE_LABEL_KEYS[group.scope] as Parameters<typeof t>[0])}
            </h3>
            <div class="divide-y divide-cf-border rounded-cf-md border border-cf-border bg-cf-bg-surface">
              <For each={group.items}>
                {(def) => (
                  <div class="flex items-center justify-between px-4 py-2.5">
                    <span class="text-sm text-cf-text-primary">
                      {t(def.labelKey as Parameters<typeof t>[0])}
                    </span>
                    <div class="flex items-center gap-2">
                      <Show
                        when={editingId() === def.id}
                        fallback={
                          <kbd
                            class={
                              "rounded-cf-sm border px-2 py-0.5 text-xs " +
                              (isCustomized(def)
                                ? "border-cf-accent bg-cf-accent/10 text-cf-accent"
                                : "border-cf-border text-cf-text-muted")
                            }
                          >
                            {formatCombo(def.combo)}
                          </kbd>
                        }
                      >
                        <ShortcutRecorder
                          onRecord={(combo) => handleRecord(def.id, combo)}
                          onCancel={() => setEditingId(null)}
                        />
                      </Show>
                      <Show when={def.configurable}>
                        <Button
                          variant="link"
                          size="xs"
                          onClick={() => {
                            setConflict(null);
                            setEditingId(editingId() === def.id ? null : def.id);
                          }}
                        >
                          {t("settings.shortcuts.edit")}
                        </Button>
                      </Show>
                      <Show when={isCustomized(def)}>
                        <Button
                          variant="icon"
                          size="xs"
                          onClick={() => resetOne(def.id)}
                          title={t("settings.shortcuts.resetOne")}
                        >
                          {"\u21A9"}
                        </Button>
                      </Show>
                    </div>
                  </div>
                )}
              </For>
            </div>
          </div>
        )}
      </For>
    </section>
  );
}
