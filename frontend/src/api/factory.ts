/**
 * Tagged template literal for building URL paths with automatic encoding.
 *
 * Usage:
 * ```ts
 * const path = url`/projects/${id}/agents`;
 * // Equivalent to `/projects/${encodeURIComponent(id)}/agents`
 * ```
 */
export function url(strings: TemplateStringsArray, ...values: (string | number)[]): string {
  let result = strings[0];
  for (let i = 0; i < values.length; i++) {
    result += encodeURIComponent(String(values[i])) + strings[i + 1];
  }
  return result;
}
