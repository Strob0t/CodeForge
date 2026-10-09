/**
 * Returns next when it is a path of this app to return to after sign-in,
 * or null: never another origin ("//host", "/\host", schemes), never the
 * login page itself, no control characters.
 */
export function safeNextPath(next: string | undefined): string | null {
  if (!next || !next.startsWith("/") || next.startsWith("//") || next.startsWith("/\\")) {
    return null;
  }
  if (/[\u0000-\u001f\u007f]/.test(next)) return null;
  if (next === "/login" || next.startsWith("/login?") || next.startsWith("/login/")) return null;
  return next;
}
