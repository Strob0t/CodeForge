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

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export type Theme = "light" | "dark" | "system";

interface ThemeContextValue {
  /** Current user preference: "light" | "dark" | "system". */
  theme: () => Theme;
  /** Resolved value after applying system preference (never "system"). */
  resolved: () => "light" | "dark";
  /** Switch theme preference. */
  setTheme: (t: Theme) => void;
  /** Cycle through light -> dark -> system. */
  toggle: () => void;
}

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const STORAGE_KEY = "codeforge-theme";
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

  function applyToDOM(r: "light" | "dark") {
    const root = document.documentElement;
    if (r === "dark") {
      root.classList.add(DARK_CLASS);
    } else {
      root.classList.remove(DARK_CLASS);
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

  // Re-resolve whenever theme preference changes
  createEffect(() => {
    const r = resolve(theme());
    setResolved(r);
    applyToDOM(r);
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

  const ctx: ThemeContextValue = { theme, resolved, setTheme, toggle };

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
      class="flex items-center gap-1.5 rounded-md px-2 py-1 text-xs text-gray-500 hover:bg-gray-100 hover:text-gray-700 dark:text-gray-400 dark:hover:bg-gray-700 dark:hover:text-gray-200"
      onClick={toggle}
      aria-label={t("theme.toggle", { name: t(THEME_KEYS[theme()]) })}
      title={t("theme.toggle", { name: t(THEME_KEYS[theme()]) })}
    >
      <span aria-hidden="true">{ICONS[theme()]}</span>
      <span>{t(THEME_KEYS[theme()])}</span>
    </button>
  );
}
