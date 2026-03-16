import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { type CanvasStore, createCanvasStore } from "../canvasState";
import type { ImageData } from "../canvasTypes";
import { createImageTool } from "../tools/ImageTool";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeSvgRef(): () => SVGSVGElement | undefined {
  // Return undefined — eventToSvg falls back to clientX/clientY
  return () => undefined;
}

function makePointerEvent(clientX: number, clientY: number): PointerEvent {
  const captured = { id: -1 };
  const event = new PointerEvent("pointerdown", {
    clientX,
    clientY,
    pointerId: 1,
    bubbles: true,
  });

  // Stub currentTarget with pointer capture methods
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

/** Assert capturedInput is not null and return it typed. */
function requireInput(input: HTMLInputElement | null): HTMLInputElement {
  expect(input).not.toBeNull();
  return input as HTMLInputElement;
}

// ---------------------------------------------------------------------------
// Factory & cursor
// ---------------------------------------------------------------------------

describe("createImageTool", () => {
  let store: CanvasStore;

  beforeEach(() => {
    store = createCanvasStore();
  });

  it("returns a CanvasTool with cursor 'copy'", () => {
    const tool = createImageTool({ store, svgRef: makeSvgRef() });
    expect(tool.cursor).toBe("copy");
  });

  it("has onPointerDown, onPointerMove, onPointerUp methods", () => {
    const tool = createImageTool({ store, svgRef: makeSvgRef() });
    expect(typeof tool.onPointerDown).toBe("function");
    expect(typeof tool.onPointerMove).toBe("function");
    expect(typeof tool.onPointerUp).toBe("function");
  });

  it("onPointerMove and onPointerUp are no-ops", () => {
    const tool = createImageTool({ store, svgRef: makeSvgRef() });
    const moveEvent = new PointerEvent("pointermove", { clientX: 0, clientY: 0 });
    const upEvent = new PointerEvent("pointerup", { clientX: 0, clientY: 0 });

    // Should not throw
    tool.onPointerMove(moveEvent);
    tool.onPointerUp(upEvent);
    expect(store.state.elements).toHaveLength(0);
  });
});

// ---------------------------------------------------------------------------
// File input creation and handling
// ---------------------------------------------------------------------------

describe("ImageTool file handling", () => {
  let store: CanvasStore;
  let appendChildSpy: ReturnType<typeof vi.spyOn>;
  let capturedInput: HTMLInputElement | null;

  beforeEach(() => {
    store = createCanvasStore();
    capturedInput = null;

    // Intercept document.body.appendChild to capture the file input
    appendChildSpy = vi.spyOn(document.body, "appendChild").mockImplementation((node) => {
      if (node instanceof HTMLInputElement && node.type === "file") {
        capturedInput = node;
        // Suppress the actual click — noop implementation required
        vi.spyOn(node, "click").mockImplementation(function noop() {
          /* intentionally empty */
        });
      }
      return node;
    });
  });

  afterEach(() => {
    appendChildSpy.mockRestore();
  });

  it("creates a hidden file input on pointer down", () => {
    const tool = createImageTool({ store, svgRef: makeSvgRef() });
    const event = makePointerEvent(100, 200);

    tool.onPointerDown(event);

    const input = requireInput(capturedInput);
    expect(input.type).toBe("file");
    expect(input.accept).toBe("image/*");
    expect(input.style.display).toBe("none");
  });

  it("calls click() on the file input to open file dialog", () => {
    const tool = createImageTool({ store, svgRef: makeSvgRef() });
    const event = makePointerEvent(100, 200);

    tool.onPointerDown(event);

    const input = requireInput(capturedInput);
    expect(input.click).toHaveBeenCalled();
  });

  it("adds an image element when a valid file is selected", () => {
    const tool = createImageTool({ store, svgRef: makeSvgRef() });
    const event = makePointerEvent(50, 75);

    tool.onPointerDown(event);

    const input = requireInput(capturedInput);

    // Simulate file selection
    const file = new File(["image-data"], "photo.png", { type: "image/png" });
    Object.defineProperty(input, "files", { value: [file] });

    // Mock FileReader
    const mockDataUrl = "data:image/png;base64,abc123";
    const originalFileReader = globalThis.FileReader;
    const mockReader = {
      onload: null as (() => void) | null,
      onerror: null as (() => void) | null,
      result: mockDataUrl,
      readAsDataURL: vi.fn(function (this: { onload: (() => void) | null }) {
        // Trigger onload synchronously for testing
        if (this.onload) this.onload();
      }),
    };
    globalThis.FileReader = vi.fn(function () {
      return mockReader;
    }) as unknown as typeof FileReader;

    // Trigger change event
    input.dispatchEvent(new Event("change"));

    // Restore FileReader
    globalThis.FileReader = originalFileReader;

    // Verify element was added
    expect(store.state.elements).toHaveLength(1);
    const el = store.state.elements[0];
    expect(el.type).toBe("image");
    expect(el.x).toBe(50);
    expect(el.y).toBe(75);
    expect(el.width).toBe(200);
    expect(el.height).toBe(200);
    const data = el.data as ImageData;
    expect(data.dataUrl).toBe(mockDataUrl);
    expect(data.originalName).toBe("photo.png");
  });

  it("rejects files larger than 5MB", () => {
    const tool = createImageTool({ store, svgRef: makeSvgRef() });
    const event = makePointerEvent(50, 75);

    tool.onPointerDown(event);

    const input = requireInput(capturedInput);

    // Create a file that claims to be > 5MB
    const bigFile = new File(["x"], "huge.png", { type: "image/png" });
    Object.defineProperty(bigFile, "size", { value: 6 * 1024 * 1024 });
    Object.defineProperty(input, "files", { value: [bigFile] });

    // Spy on FileReader to ensure it is NOT called
    const originalFileReader = globalThis.FileReader;
    const readerSpy = vi.fn();
    globalThis.FileReader = vi.fn(function () {
      return {
        onload: null,
        onerror: null,
        result: null,
        readAsDataURL: readerSpy,
      };
    }) as unknown as typeof FileReader;

    input.dispatchEvent(new Event("change"));

    globalThis.FileReader = originalFileReader;

    expect(readerSpy).not.toHaveBeenCalled();
    expect(store.state.elements).toHaveLength(0);
  });

  it("accepts files exactly at 5MB limit", () => {
    const tool = createImageTool({ store, svgRef: makeSvgRef() });
    const event = makePointerEvent(10, 20);

    tool.onPointerDown(event);

    const input = requireInput(capturedInput);

    const exactFile = new File(["x"], "exact.png", { type: "image/png" });
    Object.defineProperty(exactFile, "size", { value: 5 * 1024 * 1024 });
    Object.defineProperty(input, "files", { value: [exactFile] });

    const mockDataUrl = "data:image/png;base64,exact";
    const originalFileReader = globalThis.FileReader;
    const mockReader = {
      onload: null as (() => void) | null,
      onerror: null as (() => void) | null,
      result: mockDataUrl,
      readAsDataURL: vi.fn(function (this: { onload: (() => void) | null }) {
        if (this.onload) this.onload();
      }),
    };
    globalThis.FileReader = vi.fn(function () {
      return mockReader;
    }) as unknown as typeof FileReader;

    input.dispatchEvent(new Event("change"));

    globalThis.FileReader = originalFileReader;

    expect(store.state.elements).toHaveLength(1);
  });

  it("does nothing when no file is selected (cancel)", () => {
    const tool = createImageTool({ store, svgRef: makeSvgRef() });
    const event = makePointerEvent(50, 75);

    tool.onPointerDown(event);

    const input = requireInput(capturedInput);

    // Simulate cancel: files is empty
    Object.defineProperty(input, "files", { value: [] });
    input.dispatchEvent(new Event("change"));

    expect(store.state.elements).toHaveLength(0);
  });

  it("does nothing on FileReader error", () => {
    const tool = createImageTool({ store, svgRef: makeSvgRef() });
    const event = makePointerEvent(50, 75);

    tool.onPointerDown(event);

    const input = requireInput(capturedInput);

    const file = new File(["data"], "bad.png", { type: "image/png" });
    Object.defineProperty(input, "files", { value: [file] });

    const originalFileReader = globalThis.FileReader;
    const mockReader = {
      onload: null as (() => void) | null,
      onerror: null as (() => void) | null,
      result: null,
      readAsDataURL: vi.fn(function (this: { onerror: (() => void) | null }) {
        if (this.onerror) this.onerror();
      }),
    };
    globalThis.FileReader = vi.fn(function () {
      return mockReader;
    }) as unknown as typeof FileReader;

    input.dispatchEvent(new Event("change"));

    globalThis.FileReader = originalFileReader;

    expect(store.state.elements).toHaveLength(0);
  });
});
