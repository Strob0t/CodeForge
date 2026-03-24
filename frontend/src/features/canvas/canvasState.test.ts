import { describe, expect, it } from "vitest";

import { createCanvasStore } from "./canvasState";
import type { CanvasElement, ElementStyle } from "./canvasTypes";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function defaultStyle(): ElementStyle {
  return { fill: "#ffffff", stroke: "#000000", strokeWidth: 1, opacity: 1 };
}

function makeElement(overrides?: Partial<CanvasElement>): Omit<CanvasElement, "id" | "zIndex"> {
  return {
    type: "rect",
    x: 10,
    y: 20,
    width: 100,
    height: 50,
    rotation: 0,
    style: defaultStyle(),
    data: {},
    ...overrides,
  };
}

// ---------------------------------------------------------------------------
// addElement
// ---------------------------------------------------------------------------

describe("addElement", () => {
  it("generates a unique id for each element", () => {
    const { state, addElement } = createCanvasStore();
    addElement(makeElement());
    addElement(makeElement());

    expect(state.elements).toHaveLength(2);
    expect(state.elements[0].id).toBeTruthy();
    expect(state.elements[1].id).toBeTruthy();
    expect(state.elements[0].id).not.toBe(state.elements[1].id);
  });

  it("increments zIndex for each added element", () => {
    const { state, addElement } = createCanvasStore();
    addElement(makeElement());
    addElement(makeElement());
    addElement(makeElement());

    expect(state.elements[0].zIndex).toBe(1);
    expect(state.elements[1].zIndex).toBe(2);
    expect(state.elements[2].zIndex).toBe(3);
  });

  it("returns the generated id", () => {
    const { state, addElement } = createCanvasStore();
    const id = addElement(makeElement());

    expect(id).toBe(state.elements[0].id);
  });

  it("pushes to undo stack on add", () => {
    const { state, addElement } = createCanvasStore();
    addElement(makeElement());

    // Undo stack should contain the snapshot BEFORE the add (empty array)
    expect(state.undoStack).toHaveLength(1);
    expect(state.undoStack[0]).toHaveLength(0);
  });

  it("clears redo stack on add", () => {
    const { state, addElement, undo } = createCanvasStore();
    addElement(makeElement());
    undo();
    // After undo, redo stack has one entry
    expect(state.redoStack).toHaveLength(1);

    // Adding a new element clears redo
    addElement(makeElement());
    expect(state.redoStack).toHaveLength(0);
  });
});

// ---------------------------------------------------------------------------
// removeElement
// ---------------------------------------------------------------------------

describe("removeElement", () => {
  it("removes the element by id", () => {
    const { state, addElement, removeElement } = createCanvasStore();
    const id = addElement(makeElement());
    addElement(makeElement());

    removeElement(id);

    expect(state.elements).toHaveLength(1);
    expect(state.elements.find((e) => e.id === id)).toBeUndefined();
  });

  it("clears selection if removed element was selected", () => {
    const { state, addElement, removeElement, selectElement } = createCanvasStore();
    const id = addElement(makeElement());
    selectElement(id);
    expect(state.selectedIds).toContain(id);

    removeElement(id);

    expect(state.selectedIds).not.toContain(id);
  });

  it("preserves selection of other elements when one is removed", () => {
    const { state, addElement, removeElement, selectElement } = createCanvasStore();
    const id1 = addElement(makeElement());
    const id2 = addElement(makeElement());
    selectElement(id1);
    selectElement(id2);

    removeElement(id1);

    expect(state.selectedIds).not.toContain(id1);
    expect(state.selectedIds).toContain(id2);
  });

  it("is a no-op when id does not exist", () => {
    const { state, addElement, removeElement } = createCanvasStore();
    addElement(makeElement());

    removeElement("nonexistent-id");

    expect(state.elements).toHaveLength(1);
  });

  it("pushes to undo stack on remove", () => {
    const { state, addElement, removeElement } = createCanvasStore();
    const id = addElement(makeElement());
    const stackBefore = state.undoStack.length;

    removeElement(id);

    expect(state.undoStack).toHaveLength(stackBefore + 1);
  });
});

// ---------------------------------------------------------------------------
// updateElement
// ---------------------------------------------------------------------------

describe("updateElement", () => {
  it("merges partial updates into element", () => {
    const { state, addElement, updateElement } = createCanvasStore();
    const id = addElement(makeElement({ x: 10, y: 20 }));

    updateElement(id, { x: 50, y: 60 });

    const el = state.elements.find((e) => e.id === id);
    expect(el?.x).toBe(50);
    expect(el?.y).toBe(60);
    // Unchanged fields preserved
    expect(el?.width).toBe(100);
    expect(el?.height).toBe(50);
  });

  it("merges style partially", () => {
    const { state, addElement, updateElement } = createCanvasStore();
    const id = addElement(makeElement());

    updateElement(id, { style: { fill: "#ff0000" } });

    const el = state.elements.find((e) => e.id === id);
    expect(el?.style.fill).toBe("#ff0000");
    // Other style props preserved
    expect(el?.style.stroke).toBe("#000000");
    expect(el?.style.strokeWidth).toBe(1);
  });

  it("is a no-op when id does not exist", () => {
    const { state, addElement, updateElement } = createCanvasStore();
    addElement(makeElement());
    const snapshot = JSON.stringify(state.elements);

    updateElement("nonexistent-id", { x: 999 });

    expect(JSON.stringify(state.elements)).toBe(snapshot);
  });

  it("pushes to undo stack on update", () => {
    const { state, addElement, updateElement } = createCanvasStore();
    const id = addElement(makeElement());
    const stackBefore = state.undoStack.length;

    updateElement(id, { x: 99 });

    expect(state.undoStack).toHaveLength(stackBefore + 1);
  });

  it("does not push to undo stack when id does not exist", () => {
    const { state, addElement, updateElement } = createCanvasStore();
    addElement(makeElement());
    const stackBefore = state.undoStack.length;

    updateElement("nonexistent-id", { x: 99 });

    expect(state.undoStack).toHaveLength(stackBefore);
  });
});

// ---------------------------------------------------------------------------
// undo / redo
// ---------------------------------------------------------------------------

describe("undo", () => {
  it("reverts the last change", () => {
    const { state, addElement, undo } = createCanvasStore();
    addElement(makeElement());
    expect(state.elements).toHaveLength(1);

    undo();

    expect(state.elements).toHaveLength(0);
  });

  it("is a no-op on empty undo stack", () => {
    const { state, undo } = createCanvasStore();
    undo();

    expect(state.elements).toHaveLength(0);
    expect(state.redoStack).toHaveLength(0);
  });

  it("pushes current state onto redo stack", () => {
    const { state, addElement, undo } = createCanvasStore();
    addElement(makeElement());

    undo();

    expect(state.redoStack).toHaveLength(1);
    expect(state.redoStack[0]).toHaveLength(1);
  });

  it("supports multiple sequential undos", () => {
    const { state, addElement, undo } = createCanvasStore();
    addElement(makeElement({ x: 1 }));
    addElement(makeElement({ x: 2 }));
    addElement(makeElement({ x: 3 }));

    undo(); // revert 3rd add
    expect(state.elements).toHaveLength(2);

    undo(); // revert 2nd add
    expect(state.elements).toHaveLength(1);

    undo(); // revert 1st add
    expect(state.elements).toHaveLength(0);
  });
});

describe("redo", () => {
  it("reapplies after undo", () => {
    const { state, addElement, undo, redo } = createCanvasStore();
    addElement(makeElement());
    undo();
    expect(state.elements).toHaveLength(0);

    redo();

    expect(state.elements).toHaveLength(1);
  });

  it("is a no-op on empty redo stack", () => {
    const { state, redo } = createCanvasStore();
    redo();

    expect(state.elements).toHaveLength(0);
    expect(state.undoStack).toHaveLength(0);
  });

  it("pushes current state onto undo stack", () => {
    const { state, addElement, undo, redo } = createCanvasStore();
    addElement(makeElement());
    undo();
    const undoLenBefore = state.undoStack.length;

    redo();

    expect(state.undoStack).toHaveLength(undoLenBefore + 1);
  });

  it("supports undo-redo-undo-redo cycle", () => {
    const { state, addElement, undo, redo } = createCanvasStore();
    addElement(makeElement());

    undo();
    expect(state.elements).toHaveLength(0);
    redo();
    expect(state.elements).toHaveLength(1);
    undo();
    expect(state.elements).toHaveLength(0);
    redo();
    expect(state.elements).toHaveLength(1);
  });
});

// ---------------------------------------------------------------------------
// Undo stack cap
// ---------------------------------------------------------------------------

describe("undo stack cap", () => {
  it("caps undo stack at 50 entries", () => {
    const { state, addElement } = createCanvasStore();

    for (let i = 0; i < 60; i++) {
      addElement(makeElement({ x: i }));
    }

    expect(state.undoStack.length).toBeLessThanOrEqual(50);
    expect(state.elements).toHaveLength(60);
  });
});

// ---------------------------------------------------------------------------
// selectElement / deselectElement / deselectAll
// ---------------------------------------------------------------------------

describe("selection", () => {
  it("selectElement adds id to selectedIds", () => {
    const { state, addElement, selectElement } = createCanvasStore();
    const id = addElement(makeElement());

    selectElement(id);

    expect(state.selectedIds).toContain(id);
  });

  it("supports multi-selection", () => {
    const { state, addElement, selectElement } = createCanvasStore();
    const id1 = addElement(makeElement());
    const id2 = addElement(makeElement());

    selectElement(id1);
    selectElement(id2);

    expect(state.selectedIds).toContain(id1);
    expect(state.selectedIds).toContain(id2);
    expect(state.selectedIds).toHaveLength(2);
  });

  it("selectElement is idempotent", () => {
    const { state, addElement, selectElement } = createCanvasStore();
    const id = addElement(makeElement());

    selectElement(id);
    selectElement(id);

    expect(state.selectedIds).toHaveLength(1);
  });

  it("deselectElement removes id from selectedIds", () => {
    const { state, addElement, selectElement, deselectElement } = createCanvasStore();
    const id = addElement(makeElement());
    selectElement(id);

    deselectElement(id);

    expect(state.selectedIds).not.toContain(id);
  });

  it("deselectElement is a no-op for non-selected id", () => {
    const { state, addElement, deselectElement } = createCanvasStore();
    addElement(makeElement());

    deselectElement("nonexistent-id");

    expect(state.selectedIds).toHaveLength(0);
  });

  it("deselectAll clears all selections", () => {
    const { state, addElement, selectElement, deselectAll } = createCanvasStore();
    const id1 = addElement(makeElement());
    const id2 = addElement(makeElement());
    selectElement(id1);
    selectElement(id2);

    deselectAll();

    expect(state.selectedIds).toHaveLength(0);
  });
});

// ---------------------------------------------------------------------------
// setTool
// ---------------------------------------------------------------------------

describe("setTool", () => {
  it("changes the active tool", () => {
    const { state, setTool } = createCanvasStore();
    expect(state.activeTool).toBe("select");

    setTool("rect");
    expect(state.activeTool).toBe("rect");

    setTool("freehand");
    expect(state.activeTool).toBe("freehand");
  });

  it("defaults to select", () => {
    const { state } = createCanvasStore();
    expect(state.activeTool).toBe("select");
  });
});

// ---------------------------------------------------------------------------
// setViewport
// ---------------------------------------------------------------------------

describe("setViewport", () => {
  it("updates viewport with partial values", () => {
    const { state, setViewport } = createCanvasStore();

    setViewport({ panX: 100 });

    expect(state.viewport.panX).toBe(100);
    expect(state.viewport.panY).toBe(0);
    expect(state.viewport.zoom).toBe(1);
  });

  it("updates all viewport fields at once", () => {
    const { state, setViewport } = createCanvasStore();

    setViewport({ panX: 50, panY: 75, zoom: 2 });

    expect(state.viewport.panX).toBe(50);
    expect(state.viewport.panY).toBe(75);
    expect(state.viewport.zoom).toBe(2);
  });

  it("has correct default viewport", () => {
    const { state } = createCanvasStore();

    expect(state.viewport.panX).toBe(0);
    expect(state.viewport.panY).toBe(0);
    expect(state.viewport.zoom).toBe(1);
  });
});

// ---------------------------------------------------------------------------
// clearCanvas
// ---------------------------------------------------------------------------

describe("clearCanvas", () => {
  it("removes all elements and clears selection", () => {
    const { state, addElement, selectElement, clearCanvas } = createCanvasStore();
    const id = addElement(makeElement());
    addElement(makeElement());
    selectElement(id);

    clearCanvas();

    expect(state.elements).toHaveLength(0);
    expect(state.selectedIds).toHaveLength(0);
  });

  it("pushes to undo stack so clear can be undone", () => {
    const { state, addElement, clearCanvas } = createCanvasStore();
    addElement(makeElement());
    addElement(makeElement());
    const stackBefore = state.undoStack.length;

    clearCanvas();

    expect(state.undoStack).toHaveLength(stackBefore + 1);
  });

  it("can be undone to restore elements", () => {
    const { state, addElement, clearCanvas, undo } = createCanvasStore();
    addElement(makeElement());
    addElement(makeElement());

    clearCanvas();
    expect(state.elements).toHaveLength(0);

    undo();
    expect(state.elements).toHaveLength(2);
  });
});

// ---------------------------------------------------------------------------
// batchStart / updateElementSilent / batchCommit
// ---------------------------------------------------------------------------

describe("batch undo", () => {
  it("updateElementSilent does not push to undo stack", () => {
    const store = createCanvasStore();
    const id = store.addElement(makeElement());
    const stackBefore = store.state.undoStack.length;

    store.updateElementSilent(id, { x: 50 });

    expect(store.state.undoStack).toHaveLength(stackBefore);
    expect(store.state.elements.find((e) => e.id === id)?.x).toBe(50);
  });

  it("batchStart + updateElementSilent + batchCommit produces one undo entry", () => {
    const store = createCanvasStore();
    const id = store.addElement(makeElement({ x: 0 }));
    const stackBefore = store.state.undoStack.length;

    store.batchStart();
    store.updateElementSilent(id, { x: 10 });
    store.updateElementSilent(id, { x: 20 });
    store.updateElementSilent(id, { x: 30 });
    store.batchCommit();

    // Only ONE undo entry added (not 3)
    expect(store.state.undoStack).toHaveLength(stackBefore + 1);
    expect(store.state.elements.find((e) => e.id === id)?.x).toBe(30);
  });

  it("undoing a batch reverts to state before batchStart", () => {
    const store = createCanvasStore();
    const id = store.addElement(makeElement({ x: 0 }));

    store.batchStart();
    store.updateElementSilent(id, { x: 100 });
    store.updateElementSilent(id, { x: 200 });
    store.batchCommit();

    store.undo();

    expect(store.state.elements.find((e) => e.id === id)?.x).toBe(0);
  });

  it("batchCommit without batchStart is a no-op", () => {
    const store = createCanvasStore();
    store.addElement(makeElement());
    const stackBefore = store.state.undoStack.length;

    store.batchCommit();

    expect(store.state.undoStack).toHaveLength(stackBefore);
  });

  it("batchStart clears redo stack", () => {
    const store = createCanvasStore();
    store.addElement(makeElement());
    store.undo();
    expect(store.state.redoStack).toHaveLength(1);

    store.batchStart();
    // Redo stack cleared because a new mutation is starting
    expect(store.state.redoStack).toHaveLength(0);
  });
});

// ---------------------------------------------------------------------------
// Independent store instances
// ---------------------------------------------------------------------------

describe("createCanvasStore isolation", () => {
  it("returns independent stores", () => {
    const store1 = createCanvasStore();
    const store2 = createCanvasStore();

    store1.addElement(makeElement());

    expect(store1.state.elements).toHaveLength(1);
    expect(store2.state.elements).toHaveLength(0);
  });
});
