import {
  createContext,
  createSignal,
  type JSX,
  onCleanup,
  onMount,
  type ParentProps,
  useContext,
} from "solid-js";

import { SHORTCUTS_STORAGE_KEY } from "~/config/constants";

import { buildDefaults } from "./defaults";
import type { KeyCombo, ShortcutDefinition, ShortcutScope, StoredShortcuts } from "./types";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const IS_MAC = typeof navigator !== "undefined" && /Mac|iPhone|iPad/.test(navigator.userAgent);

export function combosEqual(a: KeyCombo, b: KeyCombo): boolean {
  return (
    a.mod === b.mod &&
    a.shift === b.shift &&
    a.alt === b.alt &&
    a.key.toLowerCase() === b.key.toLowerCase()
  );
}

export function formatCombo(combo: KeyCombo): string {
  const parts: string[] = [];
  if (combo.mod) parts.push(IS_MAC ? "\u2318" : "Ctrl");
  if (combo.shift) parts.push(IS_MAC ? "\u21E7" : "Shift");
  if (combo.alt) parts.push(IS_MAC ? "\u2325" : "Alt");
  const k = combo.key;
  parts.push(k.length === 1 ? k.toUpperCase() : k);
  return parts.join(IS_MAC ? "" : "+");
}

function eventToCombo(e: KeyboardEvent): KeyCombo | null {
  // Ignore modifier-only presses
  if (["Control", "Meta", "Shift", "Alt"].includes(e.key)) return null;
  return {
    mod: e.metaKey || e.ctrlKey,
    shift: e.shiftKey,
    alt: e.altKey,
    key: e.key,
  };
}

function loadOverrides(): StoredShortcuts {
  if (typeof window === "undefined") return {};
  const raw = localStorage.getItem(SHORTCUTS_STORAGE_KEY);
  if (!raw) return {};
  try {
    return JSON.parse(raw) as StoredShortcuts;
  } catch {
    return {};
  }
}

function saveOverrides(overrides: StoredShortcuts): void {
  if (Object.keys(overrides).length === 0) {
    localStorage.removeItem(SHORTCUTS_STORAGE_KEY);
  } else {
    localStorage.setItem(SHORTCUTS_STORAGE_KEY, JSON.stringify(overrides));
  }
}

function getActiveScope(el: Element | null): ShortcutScope | null {
  if (!el) return null;
  const scoped = (el as HTMLElement).closest?.("[data-shortcut-scope]");
  if (!scoped) return null;
  return scoped.getAttribute("data-shortcut-scope") as ShortcutScope;
}

// ---------------------------------------------------------------------------
// Context
// ---------------------------------------------------------------------------

type ActionHandler = () => void;

interface ShortcutContextValue {
  /** All shortcut definitions */
  shortcuts: () => ShortcutDefinition[];
  /** Register an action handler for a shortcut id. Returns cleanup function. */
  registerAction: (id: string, handler: ActionHandler) => () => void;
  /** Update a shortcut's key combo */
  updateCombo: (id: string, combo: KeyCombo) => void;
  /** Reset one shortcut to default */
  resetOne: (id: string) => void;
  /** Reset all shortcuts to defaults */
  resetAll: () => void;
  /** Find a conflicting shortcut */
  findConflict: (
    combo: KeyCombo,
    scope: ShortcutScope,
    excludeId?: string,
  ) => ShortcutDefinition | null;
  /** Format a combo for display */
  formatCombo: (combo: KeyCombo) => string;
}

const ShortcutContext = createContext<ShortcutContextValue>();

export function useShortcuts(): ShortcutContextValue {
  const ctx = useContext(ShortcutContext);
  if (!ctx) throw new Error("useShortcuts must be used within <ShortcutProvider>");
  return ctx;
}

// ---------------------------------------------------------------------------
// Provider
// ---------------------------------------------------------------------------

export function ShortcutProvider(props: ParentProps): JSX.Element {
  // Build initial definitions with user overrides applied
  const overrides = loadOverrides();
  const initial = buildDefaults().map((d) => {
    const override = overrides[d.id];
    return override ? { ...d, combo: { ...override } } : d;
  });

  const [shortcuts, setShortcuts] = createSignal<ShortcutDefinition[]>(initial);
  const handlers = new Map<string, ActionHandler>();

  function registerAction(id: string, handler: ActionHandler): () => void {
    handlers.set(id, handler);
    return () => {
      handlers.delete(id);
    };
  }

  function updateCombo(id: string, combo: KeyCombo): void {
    setShortcuts((prev) => prev.map((s) => (s.id === id ? { ...s, combo: { ...combo } } : s)));
    // Persist only the delta from defaults
    const current = shortcuts();
    const delta: StoredShortcuts = {};
    for (const s of current) {
      if (s.id === id) {
        if (!combosEqual(combo, s.defaultCombo)) {
          delta[s.id] = combo;
        }
      } else if (!combosEqual(s.combo, s.defaultCombo)) {
        delta[s.id] = s.combo;
      }
    }
    saveOverrides(delta);
  }

  function resetOne(id: string): void {
    const def = shortcuts().find((s) => s.id === id);
    if (def) {
      updateCombo(id, { ...def.defaultCombo });
    }
  }

  function resetAll(): void {
    setShortcuts(buildDefaults());
    saveOverrides({});
  }

  function findConflict(
    combo: KeyCombo,
    scope: ShortcutScope,
    excludeId?: string,
  ): ShortcutDefinition | null {
    return (
      shortcuts().find(
        (s) =>
          s.id !== excludeId &&
          (s.scope === scope || s.scope === "global" || scope === "global") &&
          combosEqual(s.combo, combo),
      ) ?? null
    );
  }

  // Global keydown handler
  function handleKeydown(e: KeyboardEvent): void {
    const combo = eventToCombo(e);
    if (!combo) return;

    const activeScope = getActiveScope(document.activeElement);
    const all = shortcuts();

    // Find matching shortcut — specific scope first, then global
    let match: ShortcutDefinition | undefined;
    if (activeScope) {
      match = all.find((s) => s.scope === activeScope && combosEqual(s.combo, combo));
    }
    if (!match) {
      match = all.find((s) => s.scope === "global" && combosEqual(s.combo, combo));
    }

    if (match) {
      const handler = handlers.get(match.id);
      if (handler) {
        e.preventDefault();
        handler();
      }
    }
  }

  onMount(() => {
    document.addEventListener("keydown", handleKeydown);
  });

  onCleanup(() => {
    document.removeEventListener("keydown", handleKeydown);
  });

  const ctx: ShortcutContextValue = {
    shortcuts,
    registerAction,
    updateCombo,
    resetOne,
    resetAll,
    findConflict,
    formatCombo,
  };

  return <ShortcutContext.Provider value={ctx}>{props.children}</ShortcutContext.Provider>;
}
