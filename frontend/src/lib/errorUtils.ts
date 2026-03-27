/**
 * Extract a human-readable message from an unknown caught value.
 */
export function extractErrorMessage(e: unknown, fallback = "An error occurred"): string {
  if (e instanceof Error) return e.message;
  if (typeof e === "string") return e;
  return fallback;
}

/**
 * Log an error with a structured context prefix.
 * Use in catch blocks to avoid silently swallowing errors.
 */
export function logError(context: string, error: unknown): void {
  if (error instanceof Error) {
    console.error(`[CodeForge] ${context}:`, error.message);
  } else {
    console.error(`[CodeForge] ${context}:`, error);
  }
}
