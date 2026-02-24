import type { JSX } from "solid-js";

import { useI18n } from "./context";

export function LocaleSwitcher(): JSX.Element {
  const { locale, setLocale, availableLocales, localeLabel } = useI18n();

  const next = () => {
    const idx = availableLocales.indexOf(locale());
    return availableLocales[(idx + 1) % availableLocales.length];
  };

  return (
    <button
      type="button"
      class="flex items-center gap-1 rounded-cf-md px-2 py-1 text-xs text-cf-text-muted hover:bg-cf-bg-surface-alt hover:text-cf-text-secondary"
      onClick={() => setLocale(next())}
      aria-label={`Language: ${localeLabel(locale())}. Click to switch.`}
      title={`Language: ${localeLabel(locale())}`}
    >
      <span>{localeLabel(locale())}</span>
    </button>
  );
}
