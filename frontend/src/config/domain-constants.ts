// Centralized domain constants for constrained form values.
// Stable enums that don't need a backend endpoint live here.

/** Scope types for configuration scopes. */
export const SCOPE_TYPES = ["shared", "global"] as const;
export type ScopeType = (typeof SCOPE_TYPES)[number];

/** MCP transport types (defined by the MCP specification). */
export const MCP_TRANSPORTS = ["stdio", "sse", "streamable_http"] as const;
export type MCPTransport = (typeof MCP_TRANSPORTS)[number];

/** Knowledge base categories. */
export const KB_CATEGORIES = ["framework", "paradigm", "language", "security", "custom"] as const;

/** Autonomy levels with string name values (used in global settings). */
export const AUTONOMY_LEVELS = [
  { value: "supervised", label: "1 - Supervised" },
  { value: "semi-auto", label: "2 - Semi-Auto" },
  { value: "auto-edit", label: "3 - Auto-Edit" },
  { value: "full-auto", label: "4 - Full-Auto" },
  { value: "headless", label: "5 - Headless" },
] as const;

/** Autonomy levels with numeric values and i18n keys (used in per-project settings). */
export const AUTONOMY_LEVELS_NUMERIC = [
  { value: "1", labelKey: "dashboard.form.autonomy.1" as const },
  { value: "2", labelKey: "dashboard.form.autonomy.2" as const },
  { value: "3", labelKey: "dashboard.form.autonomy.3" as const },
  { value: "4", labelKey: "dashboard.form.autonomy.4" as const },
  { value: "5", labelKey: "dashboard.form.autonomy.5" as const },
] as const;

/** Common denied actions for mode configuration (suggestions, not exhaustive). */
export const COMMON_DENIED_ACTIONS = [
  "rm",
  "rm -rf",
  "curl",
  "wget",
  "curl | bash",
  "wget | bash",
  "chmod",
  "chown",
  "sudo",
  "kill",
  "pkill",
] as const;
