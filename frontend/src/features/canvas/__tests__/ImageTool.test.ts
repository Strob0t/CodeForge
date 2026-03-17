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

function makePointerEvent(type: string, clientX: number, clientY: number): PointerEvent {
  const captured = { id: -1 };
  const event = new PointerEvent(type, {
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

/** Simulate a click (pointerDown + pointerUp at the same point). */
function simulateClick(
  tool: ReturnType<typeof createImageTool>,
  clientX: number,
  clientY: number,
): void {
  tool.onPointerDown(makePointerEvent("pointerdown", clientX, clientY));
  tool.onPointerUp(makePointerEvent("pointerup", clientX, clientY));
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

  it("creates a preview rect on pointer down, removed on pointer up", () => {
    const tool = createImageTool({ store, svgRef: makeSvgRef() });
    tool.onPointerDown(makePointerEvent("pointerdown", 100, 200));

    // Preview rect should exist (type "rect" used for placeholder)
    expect(store.state.elements).toHaveLength(1);
    expect(store.state.elements[0].type).toBe("rect");

    // After pointer up, preview rect is removed
    const appendChildSpy = vi.spyOn(document.body, "appendChild").mockImplementation((node) => {
      if (node instanceof HTMLInputElement && node.type === "file") {
        vi.spyOn(node, "click").mockImplementation(function noop() {
          /* intentionally empty */
        });
      }
      return node;
    });

    tool.onPointerUp(makePointerEvent("pointerup", 100, 200));
    expect(store.state.elements).toHaveLength(0);

    appendChildSpy.mockRestore();
  });
});

// ---------------------------------------------------------------------------
// File input creation and handling (drag-to-size)
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

  it("opens file dialog on pointer up (click = no drag)", () => {
    const tool = createImageTool({ store, svgRef: makeSvgRef() });

    simulateClick(tool, 100, 200);

    const input = requireInput(capturedInput);
    expect(input.type).toBe("file");
    expect(input.accept).toBe("image/*");
    expect(input.style.display).toBe("none");
    expect(input.click).toHaveBeenCalled();
  });

  it("adds an image element with default size on click", () => {
    const tool = createImageTool({ store, svgRef: makeSvgRef() });

    simulateClick(tool, 50, 75);

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

    // Preview rect was removed, only image element remains
    const imageEls = store.state.elements.filter((e) => e.type === "image");
    expect(imageEls).toHaveLength(1);
    const el = imageEls[0];
    expect(el.x).toBe(50);
    expect(el.y).toBe(75);
    expect(el.width).toBe(200);
    expect(el.height).toBe(200);
    const data = el.data as ImageData;
    expect(data.dataUrl).toBe(mockDataUrl);
    expect(data.originalName).toBe("photo.png");
  });

  it("uses dragged dimensions when drag is large enough", () => {
    const tool = createImageTool({ store, svgRef: makeSvgRef() });

    // Drag from (10,10) to (110,60) — a 100x50 area
    tool.onPointerDown(makePointerEvent("pointerdown", 10, 10));
    tool.onPointerMove(makePointerEvent("pointermove", 110, 60));
    tool.onPointerUp(makePointerEvent("pointerup", 110, 60));

    const input = requireInput(capturedInput);

    // Simulate file selection
    const file = new File(["image-data"], "wide.png", { type: "image/png" });
    Object.defineProperty(input, "files", { value: [file] });

    const mockDataUrl = "data:image/png;base64,wide";
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

    const imageEls = store.state.elements.filter((e) => e.type === "image");
    expect(imageEls).toHaveLength(1);
    expect(imageEls[0].x).toBe(10);
    expect(imageEls[0].y).toBe(10);
    expect(imageEls[0].width).toBe(100);
    expect(imageEls[0].height).toBe(50);
  });

  it("rejects files larger than 5MB", () => {
    const tool = createImageTool({ store, svgRef: makeSvgRef() });

    simulateClick(tool, 50, 75);

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
    // Only the removed preview rect was in the store; no image was added
    const imageEls = store.state.elements.filter((e) => e.type === "image");
    expect(imageEls).toHaveLength(0);
  });

  it("accepts files exactly at 5MB limit", () => {
    const tool = createImageTool({ store, svgRef: makeSvgRef() });

    simulateClick(tool, 10, 20);

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

    const imageEls = store.state.elements.filter((e) => e.type === "image");
    expect(imageEls).toHaveLength(1);
  });

  it("does nothing when no file is selected (cancel)", () => {
    const tool = createImageTool({ store, svgRef: makeSvgRef() });

    simulateClick(tool, 50, 75);

    const input = requireInput(capturedInput);

    // Simulate cancel: files is empty
    Object.defineProperty(input, "files", { value: [] });
    input.dispatchEvent(new Event("change"));

    const imageEls = store.state.elements.filter((e) => e.type === "image");
    expect(imageEls).toHaveLength(0);
  });

  it("does nothing on FileReader error", () => {
    const tool = createImageTool({ store, svgRef: makeSvgRef() });

    simulateClick(tool, 50, 75);

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

    const imageEls = store.state.elements.filter((e) => e.type === "image");
    expect(imageEls).toHaveLength(0);
  });
});
