import {
  createContext,
  createEffect,
  createSignal,
  type JSX,
  onCleanup,
  onMount,
  type ParentProps,
  useContext,
} from "solid-js";

import type { TranslationKey } from "~/i18n";
import { useI18n } from "~/i18n";
import { builtInThemes, type ThemeDefinition } from "~/ui/tokens";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export type Theme = "light" | "dark" | "system";

interface ThemeContextValue {
  /** Current user preference: "light" | "dark" | "system". */
  theme: () => Theme;
  /** Resolved value after applying system preference (never "system"). */
  resolved: () => "light" | "dark";
  /** Active custom theme ID, or null if using default light/dark. */
  customTheme: () => string | null;
  /** All registered custom themes. */
  customThemes: () => ThemeDefinition[];
  /** Switch theme preference. */
  setTheme: (t: Theme) => void;
  /** Cycle through light -> dark -> system. */
  toggle: () => void;
  /** Apply a custom theme by ID (null to clear). */
  applyCustomTheme: (themeId: string | null) => void;
  /** Register a user-defined theme. */
  registerTheme: (theme: ThemeDefinition) => void;
}

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const STORAGE_KEY = "codeforge-theme";
const CUSTOM_THEME_KEY = "codeforge-custom-theme";
const USER_THEMES_KEY = "codeforge-user-themes";
const DARK_CLASS = "dark";

function getSystemPreference(): "light" | "dark" {
  if (typeof window === "undefined") return "light";
  return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}

function loadStoredTheme(): Theme {
  if (typeof window === "undefined") return "system";
  const stored = localStorage.getItem(STORAGE_KEY);
  if (stored === "light" || stored === "dark" || stored === "system") return stored;
  return "system";
}

function loadStoredCustomTheme(): string | null {
  if (typeof window === "undefined") return null;
  return localStorage.getItem(CUSTOM_THEME_KEY);
}

function loadUserThemes(): ThemeDefinition[] {
  if (typeof window === "undefined") return [];
  const raw = localStorage.getItem(USER_THEMES_KEY);
  if (!raw) return [];
  try {
    return JSON.parse(raw) as ThemeDefinition[];
  } catch {
    return [];
  }
}

function resolve(theme: Theme): "light" | "dark" {
  return theme === "system" ? getSystemPreference() : theme;
}

// ---------------------------------------------------------------------------
// Context
// ---------------------------------------------------------------------------

const ThemeContext = createContext<ThemeContextValue>();

export function useTheme(): ThemeContextValue {
  const ctx = useContext(ThemeContext);
  if (!ctx) throw new Error("useTheme must be used within <ThemeProvider>");
  return ctx;
}

// ---------------------------------------------------------------------------
// Provider
// ---------------------------------------------------------------------------

export function ThemeProvider(props: ParentProps): JSX.Element {
  const initialTheme = loadStoredTheme();
  const [theme, setThemeSignal] = createSignal<Theme>(initialTheme);
  const [resolved, setResolved] = createSignal<"light" | "dark">(resolve(initialTheme));
  const [customTheme, setCustomThemeSignal] = createSignal<string | null>(loadStoredCustomTheme());
  const [userThemes, setUserThemes] = createSignal<ThemeDefinition[]>(loadUserThemes());

  const allCustomThemes = (): ThemeDefinition[] => [...builtInThemes, ...userThemes()];

  function applyToDOM(r: "light" | "dark") {
    const root = document.documentElement;
    if (r === "dark") {
      root.classList.add(DARK_CLASS);
    } else {
      root.classList.remove(DARK_CLASS);
    }
  }

  function applyCustomTokens(themeId: string | null) {
    const root = document.documentElement;
    // Clear any previously applied custom tokens
    root.removeAttribute("data-cf-theme");
    const existing = root.getAttribute("style") ?? "";
    const cleaned = existing
      .split(";")
      .filter((s) => !s.trim().startsWith("--cf-"))
      .join(";")
      .trim();
    root.setAttribute("style", cleaned || "");

    if (!themeId) return;

    const def = allCustomThemes().find((t) => t.id === themeId);
    if (!def) return;

    root.setAttribute("data-cf-theme", themeId);
    for (const [prop, value] of Object.entries(def.tokens)) {
      root.style.setProperty(prop, value ?? null);
    }
  }

  function setTheme(t: Theme) {
    setThemeSignal(t);
    localStorage.setItem(STORAGE_KEY, t);
  }

  function toggle() {
    const order: Theme[] = ["light", "dark", "system"];
    const idx = order.indexOf(theme());
    setTheme(order[(idx + 1) % order.length]);
  }

  function applyCustomTheme(themeId: string | null) {
    setCustomThemeSignal(themeId);
    if (themeId) {
      localStorage.setItem(CUSTOM_THEME_KEY, themeId);
      // Switch base mode to match custom theme
      const def = allCustomThemes().find((t) => t.id === themeId);
      if (def) {
        setTheme(def.mode);
      }
    } else {
      localStorage.removeItem(CUSTOM_THEME_KEY);
    }
  }

  function registerTheme(themeDef: ThemeDefinition) {
    const existing = userThemes().filter((t) => t.id !== themeDef.id);
    const updated = [...existing, themeDef];
    setUserThemes(updated);
    localStorage.setItem(USER_THEMES_KEY, JSON.stringify(updated));
  }

  // Re-resolve whenever theme preference changes
  createEffect(() => {
    const r = resolve(theme());
    setResolved(r);
    applyToDOM(r);
  });

  // Apply custom theme tokens whenever customTheme changes
  createEffect(() => {
    applyCustomTokens(customTheme());
  });

  // Listen for system preference changes (only matters when theme === "system")
  onMount(() => {
    const mq = window.matchMedia("(prefers-color-scheme: dark)");

    const handler = () => {
      if (theme() === "system") {
        const r = getSystemPreference();
        setResolved(r);
        applyToDOM(r);
      }
    };

    mq.addEventListener("change", handler);
    onCleanup(() => mq.removeEventListener("change", handler));
  });

  const ctx: ThemeContextValue = {
    theme,
    resolved,
    customTheme,
    customThemes: allCustomThemes,
    setTheme,
    toggle,
    applyCustomTheme,
    registerTheme,
  };

  return <ThemeContext.Provider value={ctx}>{props.children}</ThemeContext.Provider>;
}

// ---------------------------------------------------------------------------
// Toggle button (for sidebar / settings)
// ---------------------------------------------------------------------------

const ICONS: Record<Theme, string> = {
  light: "\u2600", // sun
  dark: "\u263E", // moon
  system: "\u2699", // gear
};

export function ThemeToggle(): JSX.Element {
  const { theme, toggle } = useTheme();
  const { t } = useI18n();

  const THEME_KEYS: Record<Theme, TranslationKey> = {
    light: "theme.light",
    dark: "theme.dark",
    system: "theme.system",
  };

  return (
    <button
      type="button"
      class="flex items-center gap-1.5 rounded-cf-md px-2 py-1 text-xs text-cf-text-muted hover:bg-cf-bg-surface-alt hover:text-cf-text-secondary"
      onClick={toggle}
      aria-label={t("theme.toggle", { name: t(THEME_KEYS[theme()]) })}
      title={t("theme.toggle", { name: t(THEME_KEYS[theme()]) })}
    >
      <span aria-hidden="true">{ICONS[theme()]}</span>
      <span>{t(THEME_KEYS[theme()])}</span>
    </button>
  );
}
