import type { KeyCombo, ShortcutDefinition, ShortcutScope } from "./types";

// ---------------------------------------------------------------------------
// Default shortcut definitions
// ---------------------------------------------------------------------------

interface DefaultShortcut {
  id: string;
  labelKey: string;
  defaultCombo: KeyCombo;
  scope: ShortcutScope;
  configurable?: boolean;
}

const DEFAULTS: DefaultShortcut[] = [
  // Global shortcuts
  {
    id: "palette.open",
    labelKey: "shortcuts.palette.open",
    defaultCombo: { mod: true, shift: false, alt: false, key: "k" },
    scope: "global",
  },
  {
    id: "palette.help",
    labelKey: "shortcuts.palette.help",
    defaultCombo: { mod: true, shift: false, alt: false, key: "/" },
    scope: "global",
  },
  {
    id: "nav.dashboard",
    labelKey: "shortcuts.nav.dashboard",
    defaultCombo: { mod: true, shift: false, alt: false, key: "1" },
    scope: "global",
  },
  {
    id: "nav.costs",
    labelKey: "shortcuts.nav.costs",
    defaultCombo: { mod: true, shift: false, alt: false, key: "2" },
    scope: "global",
  },
  {
    id: "nav.models",
    labelKey: "shortcuts.nav.models",
    defaultCombo: { mod: true, shift: false, alt: false, key: "3" },
    scope: "global",
  },
  {
    id: "sidebar.toggle",
    labelKey: "shortcuts.sidebar.toggle",
    defaultCombo: { mod: true, shift: false, alt: false, key: "b" },
    scope: "global",
  },
  {
    id: "theme.toggle",
    labelKey: "shortcuts.theme.toggle",
    defaultCombo: { mod: true, shift: true, alt: false, key: "t" },
    scope: "global",
  },
  // Chat scope
  {
    id: "chat.send",
    labelKey: "shortcuts.chat.send",
    defaultCombo: { mod: false, shift: false, alt: false, key: "Enter" },
    scope: "chat",
  },
  // Editor scope (informational only — Monaco handles its own keybindings)
  {
    id: "editor.save",
    labelKey: "shortcuts.editor.save",
    defaultCombo: { mod: true, shift: false, alt: false, key: "s" },
    scope: "editor",
    configurable: false,
  },
];

export function buildDefaults(): ShortcutDefinition[] {
  return DEFAULTS.map((d) => ({
    id: d.id,
    labelKey: d.labelKey,
    defaultCombo: { ...d.defaultCombo },
    combo: { ...d.defaultCombo },
    scope: d.scope,
    configurable: d.configurable !== false,
  }));
}
