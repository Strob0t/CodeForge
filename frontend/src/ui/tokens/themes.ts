// ---------------------------------------------------------------------------
// Theme definitions for custom themes (built-in + user-defined)
// ---------------------------------------------------------------------------

export interface ThemeDefinition {
  id: string;
  name: string;
  mode: "light" | "dark";
  tokens: Partial<Record<string, string>>;
}

// ---------------------------------------------------------------------------
// Built-in themes
// ---------------------------------------------------------------------------

export const nordTheme: ThemeDefinition = {
  id: "nord",
  name: "Nord",
  mode: "dark",
  tokens: {
    "--cf-bg-primary": "#2e3440",
    "--cf-bg-surface": "#3b4252",
    "--cf-bg-surface-alt": "#434c5e",
    "--cf-bg-inset": "#4c566a",
    "--cf-border": "#4c566a",
    "--cf-border-subtle": "#434c5e",
    "--cf-border-input": "#4c566a",
    "--cf-text-primary": "#eceff4",
    "--cf-text-secondary": "#d8dee9",
    "--cf-text-tertiary": "#a5b1c2",
    "--cf-text-muted": "#7b88a1",
    "--cf-accent": "#88c0d0",
    "--cf-accent-hover": "#81a1c1",
    "--cf-accent-fg": "#2e3440",
    "--cf-success": "#a3be8c",
    "--cf-success-bg": "#2e3440",
    "--cf-success-fg": "#a3be8c",
    "--cf-success-border": "#4c566a",
    "--cf-warning": "#ebcb8b",
    "--cf-warning-bg": "#2e3440",
    "--cf-warning-fg": "#ebcb8b",
    "--cf-warning-border": "#4c566a",
    "--cf-danger": "#bf616a",
    "--cf-danger-bg": "#2e3440",
    "--cf-danger-fg": "#bf616a",
    "--cf-danger-border": "#4c566a",
    "--cf-info": "#81a1c1",
    "--cf-info-bg": "#2e3440",
    "--cf-info-fg": "#81a1c1",
    "--cf-info-border": "#4c566a",
    "--cf-focus-ring": "#88c0d0",
    "--cf-status-running": "#81a1c1",
    "--cf-status-idle": "#a3be8c",
    "--cf-status-waiting": "#ebcb8b",
    "--cf-status-error": "#bf616a",
    "--cf-status-planning": "#b48ead",
  },
};

export const solarizedDarkTheme: ThemeDefinition = {
  id: "solarized-dark",
  name: "Solarized Dark",
  mode: "dark",
  tokens: {
    "--cf-bg-primary": "#002b36",
    "--cf-bg-surface": "#073642",
    "--cf-bg-surface-alt": "#073642",
    "--cf-bg-inset": "#0a4a5c",
    "--cf-border": "#094959",
    "--cf-border-subtle": "#073642",
    "--cf-border-input": "#094959",
    "--cf-text-primary": "#fdf6e3",
    "--cf-text-secondary": "#eee8d5",
    "--cf-text-tertiary": "#93a1a1",
    "--cf-text-muted": "#657b83",
    "--cf-accent": "#268bd2",
    "--cf-accent-hover": "#2aa198",
    "--cf-accent-fg": "#fdf6e3",
    "--cf-success": "#859900",
    "--cf-success-bg": "#002b36",
    "--cf-success-fg": "#859900",
    "--cf-success-border": "#094959",
    "--cf-warning": "#b58900",
    "--cf-warning-bg": "#002b36",
    "--cf-warning-fg": "#b58900",
    "--cf-warning-border": "#094959",
    "--cf-danger": "#dc322f",
    "--cf-danger-bg": "#002b36",
    "--cf-danger-fg": "#dc322f",
    "--cf-danger-border": "#094959",
    "--cf-info": "#268bd2",
    "--cf-info-bg": "#002b36",
    "--cf-info-fg": "#268bd2",
    "--cf-info-border": "#094959",
    "--cf-focus-ring": "#268bd2",
    "--cf-status-running": "#268bd2",
    "--cf-status-idle": "#859900",
    "--cf-status-waiting": "#b58900",
    "--cf-status-error": "#dc322f",
    "--cf-status-planning": "#6c71c4",
  },
};

export const builtInThemes: ThemeDefinition[] = [nordTheme, solarizedDarkTheme];
