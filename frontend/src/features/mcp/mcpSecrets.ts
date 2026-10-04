// The Go Core shows an MCP server's stored secrets (env and header values, the
// url's credentials, credential arguments) as "***", and keeps the stored value
// for a "***" sent back only while the server's transport, url, command and
// arguments are the ones it was read with; otherwise it refuses the request
// (internal/domain/mcp/redact.go, ServerDef.KeepRedacted). The form applies the
// same rule, so it can say so before it sends anything.

/** What a read shows for a stored secret (Go mcp.RedactedValue). */
export const REDACTED = "***";

/** Where a server's stored secrets go. */
export interface MCPEndpoint {
  transport: string;
  url: string;
  command: string;
  args: readonly string[];
}

/** Whether the request carries a "***" that stands for a stored secret (Go ServerDef.HasRedacted). */
export function carriesRedacted(endpoint: MCPEndpoint, values: readonly string[]): boolean {
  return (
    values.includes(REDACTED) ||
    endpoint.url.includes(REDACTED) ||
    endpoint.args.some((arg) => arg.includes(REDACTED))
  );
}

/**
 * Whether the stored secrets a "***" stands for are kept: only for a saved
 * server (read is the endpoint it was read with; null for a new one) whose
 * endpoint is unchanged.
 */
export function keepsStoredSecrets(read: MCPEndpoint | null, current: MCPEndpoint): boolean {
  return (
    read !== null &&
    read.transport === current.transport &&
    read.url === current.url &&
    read.command === current.command &&
    read.args.length === current.args.length &&
    read.args.every((arg, i) => arg === current.args[i])
  );
}
