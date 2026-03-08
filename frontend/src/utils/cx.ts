/** Joins class names, filtering falsy values. Zero-dependency clsx alternative. */
export function cx(...classes: (string | false | undefined | null)[]): string {
  return classes.filter(Boolean).join(" ");
}
