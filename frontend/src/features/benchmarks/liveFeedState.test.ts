import { describe, expect, it } from "vitest";

import { computeEta, formatTokens } from "./liveFeedState";

describe("formatTokens", () => {
  it("returns raw number below 1000", () => {
    expect(formatTokens(0)).toBe("0");
    expect(formatTokens(999)).toBe("999");
  });
  it("formats thousands with k suffix", () => {
    expect(formatTokens(1000)).toBe("1.0k");
    expect(formatTokens(1500)).toBe("1.5k");
    expect(formatTokens(24300)).toBe("24.3k");
    expect(formatTokens(999_999)).toBe("1000.0k");
  });
  it("formats millions with M suffix", () => {
    expect(formatTokens(1_000_000)).toBe("1.0M");
    expect(formatTokens(2_500_000)).toBe("2.5M");
  });
});

describe("computeEta", () => {
  it("returns null when total_tasks is null", () => {
    expect(computeEta(3, null, 120)).toBeNull();
  });
  it("returns null when 0 completed", () => {
    expect(computeEta(0, 5, 120)).toBeNull();
  });
  it("returns null when all completed", () => {
    expect(computeEta(5, 5, 120)).toBeNull();
  });
  it("calculates remaining seconds", () => {
    expect(computeEta(3, 5, 120)).toBe(80);
  });
  it("rounds to nearest second", () => {
    expect(computeEta(2, 3, 100)).toBe(50);
  });
});
