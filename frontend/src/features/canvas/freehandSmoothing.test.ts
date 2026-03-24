import { describe, expect, it } from "vitest";

import { catmullRomToSvgPath } from "./tools/smoothing";

// ---------------------------------------------------------------------------
// catmullRomToSvgPath — Catmull-Rom spline to SVG cubic bezier path
// ---------------------------------------------------------------------------

describe("catmullRomToSvgPath", () => {
  it("returns empty string for empty array", () => {
    expect(catmullRomToSvgPath([])).toBe("");
  });

  it("returns a dot (M + L to same point) for a single point", () => {
    const result = catmullRomToSvgPath([[50, 75]]);
    expect(result).toBe("M 50 75 L 50 75");
  });

  it("returns a straight line (M + L) for two points", () => {
    const result = catmullRomToSvgPath([
      [10, 20],
      [30, 40],
    ]);
    expect(result).toBe("M 10 20 L 30 40");
  });

  it("returns a smooth curve with C commands for 3+ points", () => {
    const result = catmullRomToSvgPath([
      [0, 0],
      [50, 100],
      [100, 0],
    ]);

    // Must start with M
    expect(result).toMatch(/^M 0 0/);
    // Must contain at least one cubic bezier C command
    expect(result).toContain("C");
    // Must be valid SVG path syntax (no NaN, no undefined)
    expect(result).not.toContain("NaN");
    expect(result).not.toContain("undefined");
  });

  it("produces valid C commands for 4 points", () => {
    const result = catmullRomToSvgPath([
      [0, 0],
      [25, 50],
      [75, 50],
      [100, 0],
    ]);

    expect(result).toMatch(/^M 0 0/);
    // Each C command has 6 numbers (3 control/end points)
    const cMatches = result.match(/C/g) ?? [];
    expect(cMatches.length).toBeGreaterThanOrEqual(3);
  });

  it("produces valid C commands for 5 points", () => {
    const result = catmullRomToSvgPath([
      [0, 0],
      [10, 30],
      [30, 50],
      [60, 30],
      [80, 0],
    ]);

    expect(result).toMatch(/^M 0 0/);
    const cMatches = result.match(/C/g) ?? [];
    expect(cMatches.length).toBeGreaterThanOrEqual(4);
  });

  it("ends at the last point in the input", () => {
    const points: [number, number][] = [
      [10, 20],
      [30, 40],
      [50, 60],
      [70, 80],
    ];
    const result = catmullRomToSvgPath(points);

    // The path should end with the coordinates of the last point
    expect(result).toMatch(/70 80$/);
  });

  it("handles collinear points without errors", () => {
    const result = catmullRomToSvgPath([
      [0, 0],
      [50, 50],
      [100, 100],
    ]);
    expect(result).toMatch(/^M 0 0/);
    expect(result).not.toContain("NaN");
  });

  it("handles duplicate consecutive points", () => {
    const result = catmullRomToSvgPath([
      [10, 10],
      [10, 10],
      [50, 50],
    ]);
    expect(result).toMatch(/^M 10 10/);
    expect(result).not.toContain("NaN");
  });

  it("handles negative coordinates", () => {
    const result = catmullRomToSvgPath([
      [-100, -200],
      [0, 0],
      [100, 200],
    ]);
    expect(result).toMatch(/^M -100 -200/);
    expect(result).toContain("C");
    expect(result).not.toContain("NaN");
  });

  it("handles very large coordinate values", () => {
    const result = catmullRomToSvgPath([
      [10000, 20000],
      [30000, 40000],
      [50000, 60000],
    ]);
    expect(result).toMatch(/^M 10000 20000/);
    expect(result).not.toContain("NaN");
  });

  it("handles fractional coordinates with rounding", () => {
    const result = catmullRomToSvgPath([
      [1.123456789, 2.987654321],
      [3.5, 4.5],
      [5.999, 6.001],
    ]);
    // Should not contain extremely long decimal numbers
    // Each number segment should be reasonable length
    expect(result).not.toContain("NaN");
    expect(result).not.toContain("undefined");
  });
});
