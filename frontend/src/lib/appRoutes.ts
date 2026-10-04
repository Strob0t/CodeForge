// The pages that render inside the app shell (sidebar, header), behind the
// route guard. Every other path renders on its own: the public pages (login,
// password reset, setup, privacy policy) and the 404 page for unknown paths.
// Every route of src/index.tsx that is not public must be listed here
// (appRoutes.test.ts checks it).

const SHELL_PATHS: ReadonlySet<string> = new Set([
  "/",
  "/projects",
  "/costs",
  "/ai",
  "/activity",
  "/knowledge",
  "/mcp",
  "/a2a",
  "/microagents",
  "/prompts",
  "/settings",
  "/benchmarks",
  "/quarantine",
  "/routing",
  "/search",
  "/design-system",
]);

/** Prefixes of the shell pages with a path parameter (/projects/:id, ...). */
const SHELL_PREFIXES: readonly string[] = ["/projects/", "/approvals/", "/channels/"];

export function isShellPath(pathname: string): boolean {
  return SHELL_PATHS.has(pathname) || SHELL_PREFIXES.some((prefix) => pathname.startsWith(prefix));
}
