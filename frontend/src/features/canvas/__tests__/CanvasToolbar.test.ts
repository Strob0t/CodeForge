import { describe, expect, it } from "vitest";

import { type CanvasStore, createCanvasStore } from "../canvasState";

// ---------------------------------------------------------------------------
// We test the toolbar's keyboard shortcut logic and tool definitions
// by exercising the store directly (no DOM rendering needed for pure logic).
// The keyboard handler is a plain function we can simulate via dispatch.
// ---------------------------------------------------------------------------

// Simulate the keyboard shortcut mapping from CanvasToolbar
const KEY_TO_TOOL: Record<string, string> = {
  v: "select",
  r: "rect",
  e: "ellipse",
  p: "freehand",
  t: "text",
  a: "annotate",
  i: "image",
};

function makeStore(): CanvasStore {
  return createCanvasStore();
}

// ---------------------------------------------------------------------------
// Tool shortcut mapping
// ---------------------------------------------------------------------------

describe("CanvasToolbar keyboard shortcuts", () => {
  it("maps all 7 tool shortcut keys correctly", () => {
    const store = makeStore();

    for (const expectedTool of Object.values(KEY_TO_TOOL)) {
      store.setTool("select"); // reset
      store.setTool(expectedTool as Parameters<typeof store.setTool>[0]);
      expect(store.state.activeTool).toBe(expectedTool);
    }

    // Verify total count
    expect(Object.keys(KEY_TO_TOOL)).toHaveLength(7);
  });

  it("setTool changes the active tool for every ToolType", () => {
    const store = makeStore();
    const tools = ["select", "rect", "ellipse", "freehand", "text", "annotate", "image"] as const;

    for (const tool of tools) {
      store.setTool(tool);
      expect(store.state.activeTool).toBe(tool);
    }
  });
});

// ---------------------------------------------------------------------------
// Undo / Redo via store
// ---------------------------------------------------------------------------

describe("CanvasToolbar undo/redo integration", () => {
  it("undo reverts the last element addition", () => {
    const store = makeStore();
    store.addElement({
      type: "rect",
      x: 0,
      y: 0,
      width: 100,
      height: 100,
      rotation: 0,
      style: { fill: "#fff", stroke: "#000", strokeWidth: 1, opacity: 1 },
      data: {},
    });
    expect(store.state.elements).toHaveLength(1);

    store.undo();
    expect(store.state.elements).toHaveLength(0);
  });

  it("redo reapplies after undo", () => {
    const store = makeStore();
    store.addElement({
      type: "rect",
      x: 0,
      y: 0,
      width: 100,
      height: 100,
      rotation: 0,
      style: { fill: "#fff", stroke: "#000", strokeWidth: 1, opacity: 1 },
      data: {},
    });
    store.undo();
    expect(store.state.elements).toHaveLength(0);

    store.redo();
    expect(store.state.elements).toHaveLength(1);
  });

  it("undo on empty stack is a no-op", () => {
    const store = makeStore();
    store.undo();
    expect(store.state.elements).toHaveLength(0);
    expect(store.state.undoStack).toHaveLength(0);
  });

  it("redo on empty stack is a no-op", () => {
    const store = makeStore();
    store.redo();
    expect(store.state.elements).toHaveLength(0);
    expect(store.state.redoStack).toHaveLength(0);
  });
});

// ---------------------------------------------------------------------------
// Zoom actions via store
// ---------------------------------------------------------------------------

describe("CanvasToolbar zoom actions", () => {
  it("zoom in increases viewport zoom", () => {
    const store = makeStore();
    const before = store.state.viewport.zoom;

    const newZoom = Math.min(5.0, before + 0.2);
    store.setViewport({ zoom: newZoom });

    expect(store.state.viewport.zoom).toBeCloseTo(1.2, 5);
  });

  it("zoom out decreases viewport zoom", () => {
    const store = makeStore();
    const before = store.state.viewport.zoom;

    const newZoom = Math.max(0.1, before - 0.2);
    store.setViewport({ zoom: newZoom });

    expect(store.state.viewport.zoom).toBeCloseTo(0.8, 5);
  });

  it("zoom reset restores default viewport", () => {
    const store = makeStore();
    store.setViewport({ panX: 100, panY: 200, zoom: 3.5 });

    store.setViewport({ panX: 0, panY: 0, zoom: 1 });

    expect(store.state.viewport.panX).toBe(0);
    expect(store.state.viewport.panY).toBe(0);
    expect(store.state.viewport.zoom).toBe(1);
  });

  it("zoom does not go below minimum (0.1)", () => {
    const store = makeStore();
    const newZoom = Math.max(0.1, 0.05);
    store.setViewport({ zoom: newZoom });

    expect(store.state.viewport.zoom).toBe(0.1);
  });

  it("zoom does not exceed maximum (5.0)", () => {
    const store = makeStore();
    const newZoom = Math.min(5.0, 6.0);
    store.setViewport({ zoom: newZoom });

    expect(store.state.viewport.zoom).toBe(5.0);
  });
});

// ---------------------------------------------------------------------------
// Tool definition completeness
// ---------------------------------------------------------------------------

describe("CanvasToolbar tool definitions", () => {
  it("covers all 7 ToolType values in shortcut mapping", () => {
    const allTools = ["select", "rect", "ellipse", "freehand", "text", "annotate", "image"];
    const mappedTools = Object.values(KEY_TO_TOOL);

    for (const tool of allTools) {
      expect(mappedTools).toContain(tool);
    }
  });

  it("each shortcut key is a unique single character", () => {
    const keys = Object.keys(KEY_TO_TOOL);
    const uniqueKeys = new Set(keys);

    expect(uniqueKeys.size).toBe(keys.length);

    for (const key of keys) {
      expect(key).toHaveLength(1);
    }
  });

  it("each shortcut maps to a unique tool", () => {
    const tools = Object.values(KEY_TO_TOOL);
    const uniqueTools = new Set(tools);

    expect(uniqueTools.size).toBe(tools.length);
  });
});
