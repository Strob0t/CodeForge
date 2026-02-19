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
// Types
// ---------------------------------------------------------------------------

export type Locale = "en" | "de";

interface I18nContextValue {
  /** Current locale. */
  locale: () => Locale;
  /** Switch locale. */
  setLocale: (l: Locale) => void;
  /** Translate a key with optional interpolation params. */
  t: (key: TranslationKey, params?: Record<string, string | number>) => string;
  /** All available locales. */
  availableLocales: readonly Locale[];
}

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const STORAGE_KEY = "codeforge-locale";
const AVAILABLE_LOCALES: readonly Locale[] = ["en", "de"] as const;

// Lazy-loaded translation bundles
const bundles: Record<Locale, Translations | null> = { en, de: null };

function loadStoredLocale(): Locale {
  if (typeof window === "undefined") return "en";
  const stored = localStorage.getItem(STORAGE_KEY);
  if (stored === "en" || stored === "de") return stored;
  // Auto-detect from browser
  const lang = navigator.language.split("-")[0];
  if (lang === "de") return "de";
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
    if (l === "en") return en;
    const cached = bundles[l];
    if (cached) return cached;
    // Dynamic import for non-English bundles
    const mod = await import(`./locales/${l}.ts`);
    const bundle = mod.default as Translations;
    bundles[l] = bundle;
    return bundle;
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

  const ctx: I18nContextValue = {
    locale,
    setLocale,
    t,
    availableLocales: AVAILABLE_LOCALES,
  };

  return <I18nContext.Provider value={ctx}>{props.children}</I18nContext.Provider>;
}
