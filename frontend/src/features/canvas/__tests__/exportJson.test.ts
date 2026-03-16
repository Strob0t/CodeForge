import { describe, expect, it } from "vitest";

import type { CanvasElement, ElementStyle } from "../canvasTypes";
import type { CanvasJsonExport } from "../export/exportJson";
import { exportJson } from "../export/exportJson";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function defaultStyle(): ElementStyle {
  return { fill: "#ffffff", stroke: "#000000", strokeWidth: 1, opacity: 1 };
}

function makeElement(
  overrides: Partial<CanvasElement> & { type: CanvasElement["type"] },
): CanvasElement {
  const base: CanvasElement = {
    id: "el-1",
    type: overrides.type,
    x: 10,
    y: 20,
    width: 100,
    height: 50,
    rotation: 0,
    zIndex: 1,
    style: defaultStyle(),
    data: {},
  };
  return { ...base, ...overrides };
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("exportJson", () => {
  it("includes canvas dimensions", () => {
    const result = exportJson([], 800, 600);

    expect(result.canvas.width).toBe(800);
    expect(result.canvas.height).toBe(600);
  });

  it("returns empty arrays for empty canvas", () => {
    const result = exportJson([], 800, 600);

    expect(result.elements).toHaveLength(0);
    expect(result.annotations).toHaveLength(0);
  });

  it("exports a rect element with position, dimensions, type, and style", () => {
    const rect = makeElement({
      id: "r1",
      type: "rect",
      x: 10,
      y: 20,
      width: 100,
      height: 50,
      rotation: 15,
      zIndex: 3,
      style: { fill: "#ff0000", stroke: "#000", strokeWidth: 2, opacity: 0.8 },
    });

    const result = exportJson([rect], 800, 600);

    expect(result.elements).toHaveLength(1);
    const exported = result.elements[0];
    expect(exported.id).toBe("r1");
    expect(exported.type).toBe("rect");
    expect(exported.x).toBe(10);
    expect(exported.y).toBe(20);
    expect(exported.width).toBe(100);
    expect(exported.height).toBe(50);
    expect(exported.rotation).toBe(15);
    expect(exported.zIndex).toBe(3);
    expect(exported.style.fill).toBe("#ff0000");
    expect(exported.style.stroke).toBe("#000");
    expect(exported.style.strokeWidth).toBe(2);
    expect(exported.style.opacity).toBe(0.8);
  });

  it("exports a text element with text content", () => {
    const txt = makeElement({
      id: "t1",
      type: "text",
      data: { text: "Hello World" },
    });

    const result = exportJson([txt], 800, 600);

    expect(result.elements).toHaveLength(1);
    expect(result.elements[0].type).toBe("text");
    expect(result.elements[0].data).toEqual({ text: "Hello World" });
  });

  it("exports an ellipse element", () => {
    const ell = makeElement({ id: "e1", type: "ellipse" });

    const result = exportJson([ell], 400, 300);

    expect(result.elements).toHaveLength(1);
    expect(result.elements[0].type).toBe("ellipse");
    expect(result.elements[0].id).toBe("e1");
  });

  it("exports a freehand element with points", () => {
    const fh = makeElement({
      id: "fh1",
      type: "freehand",
      data: {
        points: [
          [0, 0],
          [10, 10],
          [20, 5],
        ],
      },
    });

    const result = exportJson([fh], 800, 600);

    expect(result.elements).toHaveLength(1);
    expect(result.elements[0].type).toBe("freehand");
    expect(result.elements[0].data).toEqual({
      points: [
        [0, 0],
        [10, 10],
        [20, 5],
      ],
    });
  });

  it("exports an image element with originalName (strips dataUrl)", () => {
    const img = makeElement({
      id: "img1",
      type: "image",
      data: { dataUrl: "data:image/png;base64,verylongstringhere", originalName: "screenshot.png" },
    });

    const result = exportJson([img], 800, 600);

    expect(result.elements).toHaveLength(1);
    expect(result.elements[0].type).toBe("image");
    // dataUrl should be stripped for LLM consumption (too large)
    expect(result.elements[0].data).toEqual({ originalName: "screenshot.png" });
  });

  it("separates annotations into their own array with target references", () => {
    const rect = makeElement({ id: "r1", type: "rect", zIndex: 1 });
    const ann = makeElement({
      id: "ann1",
      type: "annotation",
      x: 50,
      y: 10,
      width: 30,
      height: 20,
      zIndex: 2,
      data: { text: "This is the header", targetElementId: "r1" },
    });

    const result = exportJson([rect, ann], 800, 600);

    // Annotations should be in the annotations array
    expect(result.annotations).toHaveLength(1);
    expect(result.annotations[0].id).toBe("ann1");
    expect(result.annotations[0].text).toBe("This is the header");
    expect(result.annotations[0].targetElementId).toBe("r1");
    expect(result.annotations[0].x).toBe(50);
    expect(result.annotations[0].y).toBe(10);

    // Non-annotation elements only in elements array
    expect(result.elements).toHaveLength(1);
    expect(result.elements[0].id).toBe("r1");
  });

  it("handles annotation without target element", () => {
    const ann = makeElement({
      id: "ann1",
      type: "annotation",
      data: { text: "Floating note" },
    });

    const result = exportJson([ann], 800, 600);

    expect(result.annotations).toHaveLength(1);
    expect(result.annotations[0].text).toBe("Floating note");
    expect(result.annotations[0].targetElementId).toBeUndefined();
  });

  it("exports all element types in a mixed canvas", () => {
    const elements: CanvasElement[] = [
      makeElement({ id: "r1", type: "rect", zIndex: 1 }),
      makeElement({ id: "e1", type: "ellipse", zIndex: 2 }),
      makeElement({ id: "fh1", type: "freehand", zIndex: 3, data: { points: [[0, 0]] } }),
      makeElement({ id: "t1", type: "text", zIndex: 4, data: { text: "Hello" } }),
      makeElement({
        id: "img1",
        type: "image",
        zIndex: 5,
        data: { dataUrl: "data:image/png;base64,abc", originalName: "pic.png" },
      }),
      makeElement({
        id: "ann1",
        type: "annotation",
        zIndex: 6,
        data: { text: "Note", targetElementId: "r1" },
      }),
    ];

    const result = exportJson(elements, 1024, 768);

    // 5 regular elements + 1 annotation
    expect(result.elements).toHaveLength(5);
    expect(result.annotations).toHaveLength(1);
    expect(result.canvas).toEqual({ width: 1024, height: 768 });
  });

  it("preserves element order by zIndex in export", () => {
    const elements: CanvasElement[] = [
      makeElement({ id: "a", type: "rect", zIndex: 3 }),
      makeElement({ id: "b", type: "rect", zIndex: 1 }),
      makeElement({ id: "c", type: "rect", zIndex: 2 }),
    ];

    const result = exportJson(elements, 800, 600);

    // Elements should be sorted by zIndex ascending
    expect(result.elements[0].id).toBe("b");
    expect(result.elements[1].id).toBe("c");
    expect(result.elements[2].id).toBe("a");
  });

  it("returns well-formed CanvasJsonExport type", () => {
    const result: CanvasJsonExport = exportJson([], 100, 100);

    expect(result).toHaveProperty("canvas");
    expect(result).toHaveProperty("elements");
    expect(result).toHaveProperty("annotations");
    expect(typeof result.canvas.width).toBe("number");
    expect(typeof result.canvas.height).toBe("number");
    expect(Array.isArray(result.elements)).toBe(true);
    expect(Array.isArray(result.annotations)).toBe(true);
  });
});
