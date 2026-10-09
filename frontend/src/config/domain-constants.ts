// Centralized domain constants for constrained form values.
// Stable enums that don't need a backend endpoint live here.

import type { WebhookKind, WebhookProvider } from "~/api/types";

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

/**
 * The providers each kind of inbound webhook accepts (Go
 * internal/domain/webhook/endpoint.go, "providers"; the API has no list).
 */
export const WEBHOOK_PROVIDERS = {
  vcs: ["github", "gitlab"],
  pm: ["github", "gitlab", "plane"],
} as const satisfies Record<WebhookKind, readonly WebhookProvider[]>;

/**
 * Plane generates its webhooks' signing secret itself, so it is given to
 * CodeForge; GitHub and GitLab take the secret CodeForge generates (Go
 * webhook.ProviderGeneratesSecret).
 */
export function webhookProviderGeneratesSecret(provider: WebhookProvider): boolean {
  return provider === "plane";
}

/** The shortest signing secret accepted from Plane (Go webhook.MinProvidedSecretLength). */
export const WEBHOOK_MIN_PROVIDED_SECRET_LENGTH = 16;
