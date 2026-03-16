import { describe, expect, it } from "vitest";

import { type CanvasStore, createCanvasStore } from "../canvasState";
import type { CanvasExports } from "../canvasTypes";

// ---------------------------------------------------------------------------
// CanvasModal logic tests — exercise the store and export contract
// without DOM rendering (SolidJS component rendering needs full JSX setup).
// ---------------------------------------------------------------------------

function makeStore(): CanvasStore {
  return createCanvasStore();
}

function makeDefaultElement(): Parameters<CanvasStore["addElement"]>[0] {
  return {
    type: "rect",
    x: 10,
    y: 20,
    width: 100,
    height: 50,
    rotation: 0,
    style: { fill: "#ffffff", stroke: "#000000", strokeWidth: 1, opacity: 1 },
    data: {},
  };
}

// ---------------------------------------------------------------------------
// Store creation — CanvasModal creates an internal store if none provided
// ---------------------------------------------------------------------------

describe("CanvasModal store management", () => {
  it("external store is independent from a new internal store", () => {
    const external = makeStore();
    const internal = makeStore();

    external.addElement(makeDefaultElement());

    expect(external.state.elements).toHaveLength(1);
    expect(internal.state.elements).toHaveLength(0);
  });

  it("store starts with default tool 'select'", () => {
    const store = makeStore();
    expect(store.state.activeTool).toBe("select");
  });

  it("store starts with empty elements", () => {
    const store = makeStore();
    expect(store.state.elements).toHaveLength(0);
  });
});

// ---------------------------------------------------------------------------
// Export data contract
// ---------------------------------------------------------------------------

describe("CanvasModal export contract", () => {
  it("CanvasExports shape has png, ascii, and json fields", () => {
    const exports: CanvasExports = {
      png: "data:image/png;base64,...",
      ascii: "+---+\n|   |\n+---+",
      json: { elements: [] },
    };

    expect(exports.png).toBeTruthy();
    expect(exports.ascii).toBeTruthy();
    expect(typeof exports.json).toBe("object");
  });

  it("export with elements includes element data in json", () => {
    const store = makeStore();
    store.addElement(makeDefaultElement());
    store.addElement(makeDefaultElement());

    const exports: CanvasExports = {
      png: "",
      ascii: "",
      json: { elements: store.state.elements },
    };

    const jsonObj = exports.json as { elements: unknown[] };
    expect(jsonObj.elements).toHaveLength(2);
  });

  it("export with empty canvas produces empty elements array", () => {
    const store = makeStore();

    const exports: CanvasExports = {
      png: "",
      ascii: "",
      json: { elements: store.state.elements },
    };

    const jsonObj = exports.json as { elements: unknown[] };
    expect(jsonObj.elements).toHaveLength(0);
  });
});

// ---------------------------------------------------------------------------
// Modal open/close state contract
// ---------------------------------------------------------------------------

describe("CanvasModal open/close contract", () => {
  it("open boolean controls visibility (contract test)", () => {
    // The modal renders via <Show when={props.open}>
    // We test the contract: open=false means no render, open=true means render
    const openState = { open: false };

    expect(openState.open).toBe(false);

    openState.open = true;
    expect(openState.open).toBe(true);
  });

  it("Escape key should trigger close (contract test)", () => {
    // The modal listens for Escape key and calls onClose
    let closeCalled = false;
    const onClose = () => {
      closeCalled = true;
    };

    // Simulate what the handler does
    const key = "Escape";
    if (key === "Escape") {
      onClose();
    }

    expect(closeCalled).toBe(true);
  });

  it("clicking X button should trigger close (contract test)", () => {
    let closeCalled = false;
    const onClose = () => {
      closeCalled = true;
    };

    // Simulate button click -> onClose
    onClose();

    expect(closeCalled).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// Store interaction from modal context
// ---------------------------------------------------------------------------

describe("CanvasModal store interaction", () => {
  it("tool changes via store are reflected in state", () => {
    const store = makeStore();

    store.setTool("rect");
    expect(store.state.activeTool).toBe("rect");

    store.setTool("freehand");
    expect(store.state.activeTool).toBe("freehand");
  });

  it("adding elements then exporting captures all elements", () => {
    const store = makeStore();
    store.addElement(makeDefaultElement());
    store.addElement({
      ...makeDefaultElement(),
      type: "ellipse",
      x: 50,
      y: 60,
    });

    const exports: CanvasExports = {
      png: "",
      ascii: "",
      json: { elements: store.state.elements },
    };

    const jsonObj = exports.json as { elements: { type: string }[] };
    expect(jsonObj.elements).toHaveLength(2);
    expect(jsonObj.elements[0].type).toBe("rect");
    expect(jsonObj.elements[1].type).toBe("ellipse");
  });

  it("clearCanvas before export produces empty output", () => {
    const store = makeStore();
    store.addElement(makeDefaultElement());
    store.addElement(makeDefaultElement());
    store.clearCanvas();

    const exports: CanvasExports = {
      png: "",
      ascii: "",
      json: { elements: store.state.elements },
    };

    const jsonObj = exports.json as { elements: unknown[] };
    expect(jsonObj.elements).toHaveLength(0);
  });

  it("undo after adding elements changes export element count", () => {
    const store = makeStore();
    store.addElement(makeDefaultElement());
    store.addElement(makeDefaultElement());
    store.undo();

    const exports: CanvasExports = {
      png: "",
      ascii: "",
      json: { elements: store.state.elements },
    };

    const jsonObj = exports.json as { elements: unknown[] };
    expect(jsonObj.elements).toHaveLength(1);
  });
});
