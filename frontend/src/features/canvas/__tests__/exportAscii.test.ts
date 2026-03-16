import { describe, expect, it } from "vitest";

import type { CanvasElement, ElementStyle } from "../canvasTypes";
import { exportAscii } from "../export/exportAscii";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function defaultStyle(): ElementStyle {
  return { fill: "#ffffff", stroke: "#000000", strokeWidth: 1, opacity: 1 };
}

function makeRect(overrides?: Partial<CanvasElement>): CanvasElement {
  return {
    id: "rect-1",
    type: "rect",
    x: 0,
    y: 0,
    width: 40, // 5 chars wide (40 / 8)
    height: 48, // 3 chars tall (48 / 16)
    rotation: 0,
    zIndex: 1,
    style: defaultStyle(),
    data: {},
    ...overrides,
  };
}

function makeText(text: string, overrides?: Partial<CanvasElement>): CanvasElement {
  return {
    id: "text-1",
    type: "text",
    x: 0,
    y: 0,
    width: text.length * 8,
    height: 16,
    rotation: 0,
    zIndex: 1,
    style: defaultStyle(),
    data: { text },
    ...overrides,
  };
}

function makeFreehand(
  points: [number, number][],
  overrides?: Partial<CanvasElement>,
): CanvasElement {
  // Compute bounding box from points
  const xs = points.map((p) => p[0]);
  const ys = points.map((p) => p[1]);
  const minX = Math.min(...xs);
  const minY = Math.min(...ys);
  const maxX = Math.max(...xs);
  const maxY = Math.max(...ys);
  return {
    id: "freehand-1",
    type: "freehand",
    x: minX,
    y: minY,
    width: maxX - minX || 1,
    height: maxY - minY || 1,
    rotation: 0,
    zIndex: 1,
    style: defaultStyle(),
    data: { points },
    ...overrides,
  };
}

function makeImage(originalName: string, overrides?: Partial<CanvasElement>): CanvasElement {
  return {
    id: "img-1",
    type: "image",
    x: 0,
    y: 0,
    width: 120, // 15 chars
    height: 48, // 3 chars
    rotation: 0,
    zIndex: 1,
    style: defaultStyle(),
    data: { dataUrl: "data:image/png;base64,abc", originalName },
    ...overrides,
  };
}

function makeAnnotation(text: string, overrides?: Partial<CanvasElement>): CanvasElement {
  return {
    id: "ann-1",
    type: "annotation",
    x: 0,
    y: 0,
    width: 40,
    height: 0,
    rotation: 0,
    zIndex: 1,
    style: defaultStyle(),
    data: { text, targetElementId: "rect-1" },
    ...overrides,
  };
}

function makeEllipse(overrides?: Partial<CanvasElement>): CanvasElement {
  return {
    id: "ellipse-1",
    type: "ellipse",
    x: 0,
    y: 0,
    width: 40, // 5 chars
    height: 48, // 3 chars
    rotation: 0,
    zIndex: 1,
    style: defaultStyle(),
    data: {},
    ...overrides,
  };
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("exportAscii", () => {
  it("returns empty string for empty canvas", () => {
    const result = exportAscii([], 80, 48);
    expect(result).toBe("");
  });

  it("returns empty string for zero-size canvas", () => {
    const result = exportAscii([], 0, 0);
    expect(result).toBe("");
  });

  it("renders a single rect with border chars", () => {
    // 40px wide = 5 chars, 48px tall = 3 rows
    const rect = makeRect({ x: 0, y: 0, width: 40, height: 48 });
    const result = exportAscii([rect], 40, 48);

    const lines = result.split("\n");
    // Should have 3 rows
    expect(lines).toHaveLength(3);
    // Top: +---+
    expect(lines[0]).toBe("+---+");
    // Middle: |   |
    expect(lines[1]).toBe("|   |");
    // Bottom: +---+
    expect(lines[2]).toBe("+---+");
  });

  it("renders a rect offset from origin", () => {
    // Place rect at (8, 16) -> char (1, 1), 24px wide = 3 chars, 48px tall = 3 rows
    const rect = makeRect({ x: 8, y: 16, width: 24, height: 48 });
    const result = exportAscii([rect], 40, 80);

    const lines = result.split("\n");
    // Canvas: 5 cols x 5 rows. Row 0 is empty (trimmed to "").
    // Rect occupies rows 1-3. Rows 4 trailing empty -> stripped.
    expect(lines).toHaveLength(4);
    // Row 0: empty (only spaces, trimmed)
    expect(lines[0]).toBe("");
    // Row 1: space + +-+ (rect top, 3 chars wide)
    expect(lines[1]).toBe(" +-+");
    // Row 2: space + | | (rect middle)
    expect(lines[2]).toBe(" | |");
    // Row 3: space + +-+ (rect bottom)
    expect(lines[3]).toBe(" +-+");
  });

  it("renders a text element at its position", () => {
    const txt = makeText("Hi", { x: 0, y: 0 });
    const result = exportAscii([txt], 16, 16);

    const lines = result.split("\n");
    expect(lines).toHaveLength(1);
    expect(lines[0]).toBe("Hi");
  });

  it("renders text at an offset position", () => {
    // x=16 -> col 2, y=0 -> row 0
    const txt = makeText("AB", { x: 16, y: 0, width: 16, height: 16 });
    const result = exportAscii([txt], 32, 16);

    const lines = result.split("\n");
    expect(lines).toHaveLength(1);
    expect(lines[0]).toBe("  AB");
  });

  it("renders freehand as * at sampled points", () => {
    // Single point at (0,0) -> col 0, row 0
    const fh = makeFreehand([[4, 8]], { x: 0, y: 0, width: 8, height: 16 });
    const result = exportAscii([fh], 16, 16);

    const lines = result.split("\n");
    expect(lines).toHaveLength(1);
    // col 0 should be *, col 1 space
    expect(lines[0][0]).toBe("*");
  });

  it("renders image as [img: name] centered in bounding box", () => {
    // 160px wide = 20 chars, 48px tall = 3 rows
    // Label "[img: logo.png]" is 16 chars — fits easily in 20 chars
    const img = makeImage("logo.png", { x: 0, y: 0, width: 160, height: 48 });
    const result = exportAscii([img], 160, 48);

    const lines = result.split("\n");
    // Row 0 is empty, row 1 has label, row 2 is empty -> stripped.
    // So we get 2 lines: ["", "  [img: logo.png]"]
    expect(lines).toHaveLength(2);
    // The label should be in the second line (middle row of the 3-row image)
    expect(lines[1]).toContain("[img: logo.png]");
  });

  it("renders annotation as --> with label", () => {
    // 40px wide = 5 chars, height 16px = 1 row for arrow
    const ann = makeAnnotation("note", { x: 0, y: 0, width: 40, height: 16 });
    const result = exportAscii([ann], 80, 16);

    // Should contain arrow and text
    expect(result).toContain("-->");
    expect(result).toContain("note");
  });

  it("renders ellipse with box-like pattern", () => {
    // 40px wide = 5 chars, 48px tall = 3 rows
    const ell = makeEllipse({ x: 0, y: 0, width: 40, height: 48 });
    const result = exportAscii([ell], 40, 48);

    const lines = result.split("\n");
    expect(lines).toHaveLength(3);
    // Top: (---)
    expect(lines[0]).toBe("(---)");
    // Middle: |   |
    expect(lines[1]).toBe("|   |");
    // Bottom: (---)
    expect(lines[2]).toBe("(---)");
  });

  it("respects z-order: higher zIndex overwrites lower", () => {
    // Rect at (0,0) with zIndex 1 — 24px = 3 chars wide, 32px = 2 rows tall
    const rect = makeRect({
      id: "r1",
      x: 0,
      y: 0,
      width: 24,
      height: 32,
      zIndex: 1,
    });
    // Text "X" at (0,0) with zIndex 2 — should overwrite rect's top-left corner
    const txt = makeText("X", {
      id: "t1",
      x: 0,
      y: 0,
      width: 8,
      height: 16,
      zIndex: 2,
    });

    const result = exportAscii([rect, txt], 24, 32);
    const lines = result.split("\n");

    // The first char of the first row should be "X" (from text, higher zIndex),
    // not "+" (from rect border)
    expect(lines[0][0]).toBe("X");
  });

  it("sorts elements by zIndex before rendering", () => {
    // Even if text comes first in array but has lower zIndex,
    // rect (higher zIndex) should overwrite
    const txt = makeText("+", {
      id: "t1",
      x: 0,
      y: 0,
      width: 8,
      height: 16,
      zIndex: 1,
    });
    const rect = makeRect({
      id: "r1",
      x: 0,
      y: 0,
      width: 24,
      height: 32,
      zIndex: 2,
    });

    // Text first in array, but rect has higher zIndex
    const result = exportAscii([txt, rect], 24, 32);
    const lines = result.split("\n");

    // Rect's "+" at (0,0) should appear since it has higher zIndex
    expect(lines[0]).toBe("+-+");
  });

  it("handles elements partially outside canvas bounds", () => {
    // Rect starts at negative coordinates — only visible part rendered
    const rect = makeRect({ x: -16, y: 0, width: 32, height: 32 });
    const result = exportAscii([rect], 16, 32);

    // Should not throw, and should produce output for the visible part
    expect(result).toBeTruthy();
    const lines = result.split("\n");
    expect(lines).toHaveLength(2);
  });

  it("handles multiple non-overlapping elements", () => {
    const r1 = makeRect({ id: "r1", x: 0, y: 0, width: 24, height: 32, zIndex: 1 });
    const r2 = makeRect({ id: "r2", x: 32, y: 0, width: 24, height: 32, zIndex: 1 });

    const result = exportAscii([r1, r2], 56, 32);
    const lines = result.split("\n");

    // Two rects side by side with a gap
    expect(lines).toHaveLength(2);
    expect(lines[0]).toContain("+-+");
    // Should see both rects
    const plusCount = (lines[0].match(/\+/g) ?? []).length;
    expect(plusCount).toBe(4); // 2 rects x 2 corners each on top row
  });
});
