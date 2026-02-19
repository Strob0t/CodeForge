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
      class="flex items-center gap-1 rounded-md px-2 py-1 text-xs text-gray-500 hover:bg-gray-100 hover:text-gray-700 dark:text-gray-400 dark:hover:bg-gray-700 dark:hover:text-gray-200"
      onClick={() => setLocale(next())}
      aria-label={`Language: ${localeLabel(locale())}. Click to switch.`}
      title={`Language: ${localeLabel(locale())}`}
    >
      <span>{localeLabel(locale())}</span>
    </button>
  );
}
