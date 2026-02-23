import { describe, expect, it } from "vitest";

import {
  formatCompact,
  formatCurrency,
  formatDate,
  formatDateTime,
  formatDuration,
  formatNumber,
  formatPercent,
  formatScore,
  formatTime,
} from "./formatters";

// Use "en-US" as a stable locale for deterministic output.
const LOCALE = "en-US";

describe("formatDate", () => {
  it("formats a Date object", () => {
    const result = formatDate(new Date("2026-02-19T00:00:00Z"), LOCALE);
    expect(result).toContain("Feb");
    expect(result).toContain("19");
    expect(result).toContain("2026");
  });

  it("formats a date string", () => {
    const result = formatDate("2025-12-25T10:00:00Z", LOCALE);
    expect(result).toContain("Dec");
    expect(result).toContain("25");
    expect(result).toContain("2025");
  });
});

describe("formatDateTime", () => {
  it("includes both date and time parts", () => {
    const result = formatDateTime(new Date("2026-01-15T14:30:00Z"), LOCALE);
    expect(result).toContain("Jan");
    expect(result).toContain("15");
    expect(result).toContain("2026");
    // Should contain time portion (hour and minute)
    expect(result).toMatch(/\d+:\d+/);
  });
});

describe("formatTime", () => {
  it("returns time with seconds", () => {
    const result = formatTime(new Date("2026-01-01T08:05:12Z"), LOCALE);
    // Should contain hour:minute:second pattern
    expect(result).toMatch(/\d+:\d{2}:\d{2}/);
  });
});

describe("formatNumber", () => {
  it("formats an integer with grouping", () => {
    expect(formatNumber(1234, LOCALE)).toBe("1,234");
  });

  it("formats zero", () => {
    expect(formatNumber(0, LOCALE)).toBe("0");
  });

  it("formats large numbers", () => {
    expect(formatNumber(1_000_000, LOCALE)).toBe("1,000,000");
  });
});

describe("formatCompact", () => {
  it("formats thousands as K", () => {
    const result = formatCompact(1200, LOCALE);
    expect(result).toContain("1.2");
    expect(result).toContain("K");
  });

  it("formats millions as M", () => {
    const result = formatCompact(3_400_000, LOCALE);
    expect(result).toContain("3.4");
    expect(result).toContain("M");
  });

  it("formats small numbers without suffix", () => {
    const result = formatCompact(42, LOCALE);
    expect(result).toBe("42");
  });
});

describe("formatCurrency", () => {
  it("formats a standard amount with 2 decimal places", () => {
    const result = formatCurrency(12.5, LOCALE);
    expect(result).toContain("$");
    expect(result).toContain("12.50");
  });

  it("uses 4 decimal places for sub-dollar amounts", () => {
    const result = formatCurrency(0.0567, LOCALE);
    expect(result).toContain("$");
    expect(result).toContain("0.0567");
  });

  it("uses 6 decimal places for very small amounts", () => {
    const result = formatCurrency(0.003456, LOCALE);
    expect(result).toContain("$");
    expect(result).toContain("0.003456");
  });
});

describe("formatDuration", () => {
  it("formats sub-second durations as ms", () => {
    expect(formatDuration(500, LOCALE)).toBe("500ms");
  });

  it("formats durations >= 1s in seconds", () => {
    expect(formatDuration(1500, LOCALE)).toBe("1.5s");
  });

  it("formats exactly 1 second", () => {
    expect(formatDuration(1000, LOCALE)).toBe("1.0s");
  });

  it("formats zero milliseconds", () => {
    expect(formatDuration(0, LOCALE)).toBe("0ms");
  });
});

describe("formatScore", () => {
  it("formats with 3 decimal places", () => {
    expect(formatScore(0.847, LOCALE)).toBe("0.847");
  });

  it("pads with trailing zeros", () => {
    expect(formatScore(1, LOCALE)).toBe("1.000");
  });
});

describe("formatPercent", () => {
  it("formats without decimal places", () => {
    expect(formatPercent(85, LOCALE)).toBe("85");
  });

  it("rounds to nearest integer", () => {
    expect(formatPercent(85.7, LOCALE)).toBe("86");
  });
});
