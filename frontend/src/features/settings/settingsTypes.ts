import type { TranslationKey } from "~/i18n";

export interface SettingsSection {
  id: string;
  labelKey: TranslationKey;
}

export const SETTINGS_SECTIONS: SettingsSection[] = [
  { id: "settings-general", labelKey: "settings.nav.general" },
  { id: "settings-shortcuts", labelKey: "settings.nav.shortcuts" },
  { id: "settings-vcs", labelKey: "settings.nav.vcs" },
  { id: "settings-providers", labelKey: "settings.nav.providers" },
  { id: "settings-proxy", labelKey: "settings.nav.proxy" },
  { id: "settings-subscriptions", labelKey: "settings.nav.subscriptions" },
  { id: "settings-apikeys", labelKey: "settings.nav.apiKeys" },
  { id: "settings-privacy", labelKey: "settings.nav.privacy" },
  { id: "settings-users", labelKey: "settings.nav.users" },
  { id: "settings-devtools", labelKey: "settings.nav.devTools" },
];

/**
 * The section a link opens (/settings?section=privacy opens settings-privacy),
 * or null when the name is missing or names no section.
 */
export function sectionIdFromQuery(name: string | undefined): string | null {
  if (!name) return null;
  const id = `settings-${name}`;
  return SETTINGS_SECTIONS.some((s) => s.id === id) ? id : null;
}
