/**
 * Extract a human-readable message from an unknown caught value.
 */
export function extractErrorMessage(e: unknown, fallback = "An error occurred"): string {
  if (e instanceof Error) return e.message;
  if (typeof e === "string") return e;
  return fallback;
}
