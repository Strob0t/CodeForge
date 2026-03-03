// ---------------------------------------------------------------------------
// Keyboard shortcut types
// ---------------------------------------------------------------------------

/** A key combination */
export interface KeyCombo {
  /** Whether Ctrl (Windows/Linux) or Cmd (Mac) is required */
  mod: boolean;
  /** Whether Shift is required */
  shift: boolean;
  /** Whether Alt/Option is required */
  alt: boolean;
  /** The key value (e.g., "k", "1", "/", "Enter", "Escape") */
  key: string;
}

/** Scope in which a shortcut is active */
export type ShortcutScope = "global" | "palette" | "chat" | "editor";

/** A registered shortcut definition */
export interface ShortcutDefinition {
  /** Unique action identifier (e.g., "palette.open", "nav.dashboard") */
  id: string;
  /** i18n key for the human-readable label */
  labelKey: string;
  /** Default key combo (immutable) */
  defaultCombo: KeyCombo;
  /** Current (possibly user-modified) key combo */
  combo: KeyCombo;
  /** Where this shortcut is active */
  scope: ShortcutScope;
  /** Whether this shortcut can be reconfigured (false for editor-managed shortcuts) */
  configurable: boolean;
}

/** Stored user overrides (sparse: only customized shortcuts) */
export type StoredShortcuts = Record<string, KeyCombo>;
