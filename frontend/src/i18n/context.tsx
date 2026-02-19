import {
  createContext,
  createEffect,
  createSignal,
  type JSX,
  type ParentProps,
  useContext,
} from "solid-js";

import type { TranslationKey, Translations } from "./en";
import en from "./en";

// ---------------------------------------------------------------------------
// Locale registry — add new languages here (single source of truth)
// ---------------------------------------------------------------------------

/** Locale metadata entry. Label is the native name shown in the switcher. */
interface LocaleEntry {
  label: string;
  /** Lazy loader — returns the translation bundle. English is inline (null). */
  load: (() => Promise<{ default: Translations }>) | null;
}

/**
 * Register all supported locales here. To add a new language:
 * 1. Create `frontend/src/i18n/locales/{code}.ts` exporting Translations.
 * 2. Add an entry below with the language code, native label, and dynamic import.
 */
const LOCALE_REGISTRY: Record<string, LocaleEntry> = {
  en: { label: "EN", load: null },
  de: { label: "DE", load: () => import("./locales/de") },
};

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export type Locale = keyof typeof LOCALE_REGISTRY;

interface I18nContextValue {
  /** Current locale. */
  locale: () => Locale;
  /** Switch locale. */
  setLocale: (l: Locale) => void;
  /** Translate a key with optional interpolation params. */
  t: (key: TranslationKey, params?: Record<string, string | number>) => string;
  /** All available locales. */
  availableLocales: readonly Locale[];
  /** Native label for a locale (e.g. "DE", "FR"). */
  localeLabel: (l: Locale) => string;
}

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const STORAGE_KEY = "codeforge-locale";
const AVAILABLE_LOCALES: readonly Locale[] = Object.keys(LOCALE_REGISTRY);

/** Cache for lazily loaded bundles. */
const bundleCache = new Map<Locale, Translations>();
bundleCache.set("en", en);

function isValidLocale(value: string): value is Locale {
  return value in LOCALE_REGISTRY;
}

function loadStoredLocale(): Locale {
  if (typeof window === "undefined") return "en";
  const stored = localStorage.getItem(STORAGE_KEY);
  if (stored && isValidLocale(stored)) return stored;
  // Auto-detect from browser language
  const lang = navigator.language.split("-")[0];
  if (isValidLocale(lang)) return lang;
  return "en";
}

// ---------------------------------------------------------------------------
// Context
// ---------------------------------------------------------------------------

const I18nContext = createContext<I18nContextValue>();

export function useI18n(): I18nContextValue {
  const ctx = useContext(I18nContext);
  if (!ctx) throw new Error("useI18n must be used within <I18nProvider>");
  return ctx;
}

// ---------------------------------------------------------------------------
// Provider
// ---------------------------------------------------------------------------

export function I18nProvider(props: ParentProps): JSX.Element {
  const initial = loadStoredLocale();
  const [locale, setLocaleSignal] = createSignal<Locale>(initial);
  const [translations, setTranslations] = createSignal<Translations>(en);

  async function loadBundle(l: Locale): Promise<Translations> {
    const cached = bundleCache.get(l);
    if (cached) return cached;
    const entry = LOCALE_REGISTRY[l];
    if (!entry?.load) return en;
    const mod = await entry.load();
    bundleCache.set(l, mod.default);
    return mod.default;
  }

  function setLocale(l: Locale) {
    setLocaleSignal(l);
    localStorage.setItem(STORAGE_KEY, l);
  }

  // Load translation bundle when locale changes
  createEffect(() => {
    const l = locale();
    void loadBundle(l).then((bundle) => setTranslations(bundle));
  });

  function t(key: TranslationKey, params?: Record<string, string | number>): string {
    const bundle = translations();
    let text = bundle[key] ?? en[key] ?? key;
    if (params) {
      for (const [k, v] of Object.entries(params)) {
        text = text.replaceAll(`{{${k}}}`, String(v));
      }
    }
    return text;
  }

  function localeLabel(l: Locale): string {
    return LOCALE_REGISTRY[l]?.label ?? l;
  }

  const ctx: I18nContextValue = {
    locale,
    setLocale,
    t,
    availableLocales: AVAILABLE_LOCALES,
    localeLabel,
  };

  return <I18nContext.Provider value={ctx}>{props.children}</I18nContext.Provider>;
}
