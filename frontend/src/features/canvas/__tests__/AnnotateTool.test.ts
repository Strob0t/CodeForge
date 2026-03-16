import { beforeEach, describe, expect, it } from "vitest";

import { type CanvasStore, createCanvasStore } from "../canvasState";
import type { AnnotationData } from "../canvasTypes";
import { createAnnotateTool } from "../tools/AnnotateTool";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeSvgRef(): () => SVGSVGElement | undefined {
  // Return undefined — eventToSvg falls back to clientX/clientY
  return () => undefined;
}

function makePointerEvent(type: string, clientX: number, clientY: number): PointerEvent {
  const captured = { id: -1 };
  const event = new PointerEvent(type, {
    clientX,
    clientY,
    pointerId: 1,
    bubbles: true,
  });

  Object.defineProperty(event, "currentTarget", {
    value: {
      setPointerCapture: (id: number) => {
        captured.id = id;
      },
      releasePointerCapture: () => {
        captured.id = -1;
      },
    },
    writable: false,
  });

  return event;
}

// ---------------------------------------------------------------------------
// Factory & cursor
// ---------------------------------------------------------------------------

describe("createAnnotateTool", () => {
  let store: CanvasStore;

  beforeEach(() => {
    store = createCanvasStore();
  });

  it("returns a CanvasTool with cursor 'crosshair'", () => {
    const tool = createAnnotateTool({ store, svgRef: makeSvgRef() });
    expect(tool.cursor).toBe("crosshair");
  });

  it("has onPointerDown, onPointerMove, onPointerUp methods", () => {
    const tool = createAnnotateTool({ store, svgRef: makeSvgRef() });
    expect(typeof tool.onPointerDown).toBe("function");
    expect(typeof tool.onPointerMove).toBe("function");
    expect(typeof tool.onPointerUp).toBe("function");
  });
});

// ---------------------------------------------------------------------------
// Annotation creation via drag
// ---------------------------------------------------------------------------

describe("AnnotateTool drag creation", () => {
  let store: CanvasStore;

  beforeEach(() => {
    store = createCanvasStore();
  });

  it("creates a preview annotation element on pointer down", () => {
    const tool = createAnnotateTool({ store, svgRef: makeSvgRef() });

    tool.onPointerDown(makePointerEvent("pointerdown", 100, 200));

    expect(store.state.elements).toHaveLength(1);
    const el = store.state.elements[0];
    expect(el.type).toBe("annotation");
    expect(el.x).toBe(100);
    expect(el.y).toBe(200);
    expect(el.width).toBe(0);
    expect(el.height).toBe(0);
  });

  it("sets default annotation text to 'Note'", () => {
    const tool = createAnnotateTool({ store, svgRef: makeSvgRef() });

    tool.onPointerDown(makePointerEvent("pointerdown", 50, 50));

    const data = store.state.elements[0].data as AnnotationData;
    expect(data.text).toBe("Note");
  });

  it("sets arrowPath on the preview element", () => {
    const tool = createAnnotateTool({ store, svgRef: makeSvgRef() });

    tool.onPointerDown(makePointerEvent("pointerdown", 10, 20));

    const data = store.state.elements[0].data as AnnotationData;
    expect(data.arrowPath).toBe("M 10 20 L 10 20");
  });

  it("updates preview during pointer move", () => {
    const tool = createAnnotateTool({ store, svgRef: makeSvgRef() });

    tool.onPointerDown(makePointerEvent("pointerdown", 100, 100));
    tool.onPointerMove(makePointerEvent("pointermove", 200, 150));

    const el = store.state.elements[0];
    expect(el.width).toBe(100);
    expect(el.height).toBe(50);

    const data = el.data as AnnotationData;
    expect(data.arrowPath).toBe("M 100 100 L 200 150");
  });

  it("updates bounding box with min x/y for reverse drags", () => {
    const tool = createAnnotateTool({ store, svgRef: makeSvgRef() });

    tool.onPointerDown(makePointerEvent("pointerdown", 200, 200));
    tool.onPointerMove(makePointerEvent("pointermove", 100, 150));

    const el = store.state.elements[0];
    // Bounding box should use min coordinates
    expect(el.x).toBe(100);
    expect(el.y).toBe(150);
    expect(el.width).toBe(100);
    expect(el.height).toBe(50);
  });

  it("finalizes annotation when drag distance > 5px", () => {
    const tool = createAnnotateTool({ store, svgRef: makeSvgRef() });

    tool.onPointerDown(makePointerEvent("pointerdown", 100, 100));
    tool.onPointerMove(makePointerEvent("pointermove", 110, 100));
    tool.onPointerUp(makePointerEvent("pointerup", 110, 100));

    // Distance = 10px > 5px threshold: element should remain
    expect(store.state.elements).toHaveLength(1);
    const data = store.state.elements[0].data as AnnotationData;
    expect(data.text).toBe("Note");
    expect(data.arrowPath).toContain("M");
    expect(data.arrowPath).toContain("L");
  });

  it("removes annotation when drag distance < 5px (accidental click)", () => {
    const tool = createAnnotateTool({ store, svgRef: makeSvgRef() });

    tool.onPointerDown(makePointerEvent("pointerdown", 100, 100));
    tool.onPointerUp(makePointerEvent("pointerup", 102, 101));

    // Distance = sqrt(4+1) = ~2.24px < 5px: element should be removed
    expect(store.state.elements).toHaveLength(0);
  });

  it("removes annotation when drag distance is exactly 0 (pure click)", () => {
    const tool = createAnnotateTool({ store, svgRef: makeSvgRef() });

    tool.onPointerDown(makePointerEvent("pointerdown", 50, 50));
    tool.onPointerUp(makePointerEvent("pointerup", 50, 50));

    expect(store.state.elements).toHaveLength(0);
  });

  it("keeps annotation at exactly 5px drag distance (boundary)", () => {
    const tool = createAnnotateTool({ store, svgRef: makeSvgRef() });

    // Exactly 5px: distance = sqrt(3^2 + 4^2) = 5
    tool.onPointerDown(makePointerEvent("pointerdown", 100, 100));
    tool.onPointerUp(makePointerEvent("pointerup", 103, 104));

    // Distance exactly equals threshold (5px) — not less than, so it stays
    expect(store.state.elements).toHaveLength(1);
  });

  it("sets correct style properties on the annotation", () => {
    const tool = createAnnotateTool({ store, svgRef: makeSvgRef() });

    tool.onPointerDown(makePointerEvent("pointerdown", 100, 100));

    const el = store.state.elements[0];
    expect(el.style.stroke).toBe("#000000");
    expect(el.style.strokeWidth).toBe(2);
    expect(el.style.fontSize).toBe(12);
    expect(el.style.fontFamily).toBe("sans-serif");
  });
});

// ---------------------------------------------------------------------------
// No-op when no drag in progress
// ---------------------------------------------------------------------------

describe("AnnotateTool no-op scenarios", () => {
  let store: CanvasStore;

  beforeEach(() => {
    store = createCanvasStore();
  });

  it("onPointerMove without prior pointer down is a no-op", () => {
    const tool = createAnnotateTool({ store, svgRef: makeSvgRef() });

    tool.onPointerMove(makePointerEvent("pointermove", 100, 100));

    expect(store.state.elements).toHaveLength(0);
  });

  it("onPointerUp without prior pointer down is a no-op", () => {
    const tool = createAnnotateTool({ store, svgRef: makeSvgRef() });

    tool.onPointerUp(makePointerEvent("pointerup", 100, 100));

    expect(store.state.elements).toHaveLength(0);
  });

  it("allows creating multiple annotations sequentially", () => {
    const tool = createAnnotateTool({ store, svgRef: makeSvgRef() });

    // First annotation (long enough drag)
    tool.onPointerDown(makePointerEvent("pointerdown", 10, 10));
    tool.onPointerMove(makePointerEvent("pointermove", 100, 100));
    tool.onPointerUp(makePointerEvent("pointerup", 100, 100));

    // Second annotation
    tool.onPointerDown(makePointerEvent("pointerdown", 200, 200));
    tool.onPointerMove(makePointerEvent("pointermove", 300, 300));
    tool.onPointerUp(makePointerEvent("pointerup", 300, 300));

    expect(store.state.elements).toHaveLength(2);
    expect(store.state.elements[0].type).toBe("annotation");
    expect(store.state.elements[1].type).toBe("annotation");
  });

  it("clears drag state after pointer up so next interactions work", () => {
    const tool = createAnnotateTool({ store, svgRef: makeSvgRef() });

    // Create and cancel (too short)
    tool.onPointerDown(makePointerEvent("pointerdown", 50, 50));
    tool.onPointerUp(makePointerEvent("pointerup", 50, 50));
    expect(store.state.elements).toHaveLength(0);

    // Subsequent move should not throw or create elements
    tool.onPointerMove(makePointerEvent("pointermove", 200, 200));
    expect(store.state.elements).toHaveLength(0);
  });
});
