export interface SettingsSection {
  id: string;
  label: string;
}

export const SETTINGS_SECTIONS: SettingsSection[] = [
  { id: "settings-general", label: "General" },
  { id: "settings-shortcuts", label: "Shortcuts" },
  { id: "settings-vcs", label: "VCS" },
  { id: "settings-providers", label: "Providers" },
  { id: "settings-proxy", label: "LLM Proxy" },
  { id: "settings-subscriptions", label: "Subscriptions" },
  { id: "settings-apikeys", label: "API Keys" },
  { id: "settings-users", label: "Users" },
  { id: "settings-devtools", label: "Dev Tools" },
];
