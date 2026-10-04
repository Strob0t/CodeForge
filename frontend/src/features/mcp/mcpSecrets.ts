// The Go Core shows an MCP server's stored secrets (env and header values, the
// url's credentials, credential arguments) as "***", and keeps the stored value
// for a "***" sent back only while everything that decides where it goes is as
// read: transport, url, command, arguments, and every other env variable and
// header (none added, changed or removed: GITLAB_API_URL or HTTPS_PROXY would
// send a kept token elsewhere). Otherwise it refuses the request
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

/** A server as read, or as the form would send it. */
export interface MCPSecretsView {
  endpoint: MCPEndpoint;
  env: Readonly<Record<string, string>>;
  headers: Readonly<Record<string, string>>;
}

/** Whether the request carries a "***" that stands for a stored secret (Go ServerDef.HasRedacted). */
export function carriesRedacted(view: MCPSecretsView): boolean {
  return (
    Object.values(view.env).includes(REDACTED) ||
    Object.values(view.headers).includes(REDACTED) ||
    view.endpoint.url.includes(REDACTED) ||
    view.endpoint.args.some((arg) => arg.includes(REDACTED))
  );
}

function sameEndpoint(a: MCPEndpoint, b: MCPEndpoint): boolean {
  return (
    a.transport === b.transport &&
    a.url === b.url &&
    a.command === b.command &&
    a.args.length === b.args.length &&
    a.args.every((arg, i) => arg === b.args[i])
  );
}

/** The keys as read, no other, and every value that is not "***" as read. */
function unchangedApartFromRedacted(
  current: Readonly<Record<string, string>>,
  read: Readonly<Record<string, string>>,
): boolean {
  const keys = Object.keys(current);
  return (
    keys.length === Object.keys(read).length &&
    keys.every((k) => k in read && (current[k] === REDACTED || current[k] === read[k]))
  );
}

/**
 * Whether the stored secrets a "***" stands for are kept: only for a saved
 * server (read is the server as read; null for a new one) whose endpoint,
 * env and headers are unchanged apart from the "***" values.
 */
export function keepsStoredSecrets(read: MCPSecretsView | null, current: MCPSecretsView): boolean {
  return (
    read !== null &&
    sameEndpoint(read.endpoint, current.endpoint) &&
    unchangedApartFromRedacted(current.env, read.env) &&
    unchangedApartFromRedacted(current.headers, read.headers)
  );
}
