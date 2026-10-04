// Which pages render inside the app shell (sidebar, header), behind the route
// guard. It fails closed: every path does except the public pages (login,
// password reset, setup, privacy policy), unknown paths included (their 404
// page then shows after sign-in). The router matches static segments
// case-insensitively and ignores empty ones (/SETTINGS, /settings//), so a
// path is compared in that form; a list of guarded paths would miss such
// variants (S7-G review).

const PUBLIC_PATHS: ReadonlySet<string> = new Set([
  "/login",
  "/change-password",
  "/setup",
  "/forgot-password",
  "/reset-password",
  "/privacy",
]);

/** The path as the router matches it: empty segments dropped, lowercase. */
function normalized(pathname: string): string {
  const segments = pathname.split("/").filter((segment) => segment !== "");
  return "/" + segments.map((segment) => segment.toLowerCase()).join("/");
}

export function isShellPath(pathname: string): boolean {
  return !PUBLIC_PATHS.has(normalized(pathname));
}
