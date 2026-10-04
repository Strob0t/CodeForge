/** The first value of a search parameter (the router gives a repeated one as a list); undefined when empty. */
export function firstParam(value: string | string[] | undefined): string | undefined {
  const first = Array.isArray(value) ? value[0] : value;
  return first ? first : undefined;
}
