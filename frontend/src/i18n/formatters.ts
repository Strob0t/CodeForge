// Locale-aware formatting utilities using Intl APIs.
// All functions accept a BCP 47 locale string and return formatted text.

/** Format a date as short locale-aware string (e.g. "Feb 19, 2026" or "19. Feb. 2026"). */
export function formatDate(date: string | Date, locale: string): string {
  const d = typeof date === "string" ? new Date(date) : date;
  return new Intl.DateTimeFormat(locale, {
    year: "numeric",
    month: "short",
    day: "numeric",
  }).format(d);
}

/** Format a date with time (e.g. "Feb 19, 2026, 3:45 PM"). */
export function formatDateTime(date: string | Date, locale: string): string {
  const d = typeof date === "string" ? new Date(date) : date;
  return new Intl.DateTimeFormat(locale, {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
  }).format(d);
}

/** Format time only (e.g. "3:45:12 PM" or "15:45:12"). */
export function formatTime(date: string | Date, locale: string): string {
  const d = typeof date === "string" ? new Date(date) : date;
  return new Intl.DateTimeFormat(locale, {
    hour: "numeric",
    minute: "2-digit",
    second: "2-digit",
  }).format(d);
}

/** Format a plain number with locale-aware grouping (e.g. "1,234" or "1.234"). */
export function formatNumber(n: number, locale: string): string {
  return new Intl.NumberFormat(locale).format(n);
}

/** Format a number in compact notation (e.g. "1.2K", "3.4M"). */
export function formatCompact(n: number, locale: string): string {
  return new Intl.NumberFormat(locale, {
    notation: "compact",
    maximumFractionDigits: 1,
  }).format(n);
}

/** Format a USD currency amount with appropriate precision. */
export function formatCurrency(usd: number, locale: string): string {
  // Use more decimal places for very small amounts.
  const fractionDigits = usd > 0 && usd < 0.01 ? 6 : usd < 1 ? 4 : 2;
  return new Intl.NumberFormat(locale, {
    style: "currency",
    currency: "USD",
    minimumFractionDigits: fractionDigits,
    maximumFractionDigits: fractionDigits,
  }).format(usd);
}

/** Format a duration in milliseconds as a human-readable string (e.g. "1.2s", "45.0ms"). */
export function formatDuration(ms: number, locale: string): string {
  if (ms < 1000) {
    return `${new Intl.NumberFormat(locale, { maximumFractionDigits: 0 }).format(ms)}ms`;
  }
  return `${new Intl.NumberFormat(locale, { minimumFractionDigits: 1, maximumFractionDigits: 1 }).format(ms / 1000)}s`;
}

/** Format a score/percentage value (e.g. "0.847" or "85.2%"). */
export function formatScore(n: number, locale: string): string {
  return new Intl.NumberFormat(locale, {
    minimumFractionDigits: 3,
    maximumFractionDigits: 3,
  }).format(n);
}

/** Format a percentage value (e.g. "85%"). */
export function formatPercent(n: number, locale: string): string {
  return new Intl.NumberFormat(locale, {
    maximumFractionDigits: 0,
  }).format(n);
}
