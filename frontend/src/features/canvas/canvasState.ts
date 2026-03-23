import { createStore, produce } from "solid-js/store";

import type {
  CanvasElement,
  CanvasStoreState,
  ElementStyle,
  ToolType,
  Viewport,
} from "./canvasTypes";

/** Apply a partial patch to a CanvasElement inside a produce() callback. */
function applyElementPatch(el: CanvasElement, patch: ElementPatch): void {
  const { style: stylePatch, ...rest } = patch;
  // Merge top-level fields via Object.assign (safe: keys come from Partial<CanvasElement>)
  if (Object.keys(rest).length > 0) {
    Object.assign(el, rest);
  }
  // Merge style partially
  if (stylePatch && Object.keys(stylePatch).length > 0) {
    Object.assign(el.style as ElementStyle, stylePatch);
  }
}

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const MAX_UNDO_STACK = 50;

// ---------------------------------------------------------------------------
// Store factory — each call returns an independent store instance
// ---------------------------------------------------------------------------

// Patch type: all CanvasElement fields optional except id, with style also partial
export type ElementPatch = Partial<Omit<CanvasElement, "id" | "style">> & {
  style?: Partial<CanvasElement["style"]>;
};

export interface CanvasStore {
  state: CanvasStoreState;
  addElement: (input: Omit<CanvasElement, "id" | "zIndex">) => string;
  updateElement: (id: string, patch: ElementPatch) => void;
  updateElementSilent: (id: string, patch: ElementPatch) => void;
  removeElement: (id: string) => void;
  undo: () => void;
  redo: () => void;
  setTool: (tool: ToolType) => void;
  setViewport: (patch: Partial<Viewport>) => void;
  selectElement: (id: string) => void;
  deselectElement: (id: string) => void;
  deselectAll: () => void;
  clearCanvas: () => void;
  batchStart: () => void;
  batchCommit: () => void;
  setEditingId: (id: string | null) => void;
}

let globalIdCounter = 0;

function nextId(): string {
  return `el-${Date.now()}-${++globalIdCounter}`;
}

export function createCanvasStore(): CanvasStore {
  let zIndexCounter = 0;
  let batchSnapshot: CanvasElement[] | null = null;

  const [state, setState] = createStore<CanvasStoreState>({
    elements: [],
    selectedIds: [],
    activeTool: "select",
    viewport: { panX: 0, panY: 0, zoom: 1 },
    undoStack: [],
    redoStack: [],
    editingId: null,
  });

  // Snapshot the current elements array (deep clone via JSON round-trip)
  function snapshotElements(): CanvasElement[] {
    return JSON.parse(JSON.stringify(state.elements)) as CanvasElement[];
  }

  // Push current elements onto the undo stack, capping at MAX_UNDO_STACK
  function pushUndo(): void {
    const snapshot = snapshotElements();
    setState(
      produce((s) => {
        s.undoStack.push(snapshot);
        if (s.undoStack.length > MAX_UNDO_STACK) {
          s.undoStack.splice(0, s.undoStack.length - MAX_UNDO_STACK);
        }
        // Any mutation invalidates the redo stack
        s.redoStack = [];
      }),
    );
  }

  function addElement(input: Omit<CanvasElement, "id" | "zIndex">): string {
    const id = nextId();
    const element: CanvasElement = {
      ...input,
      id,
      zIndex: ++zIndexCounter,
    };

    pushUndo();

    setState(
      produce((s) => {
        s.elements.push(element);
      }),
    );

    return id;
  }

  function removeElement(id: string): void {
    const exists = state.elements.some((e) => e.id === id);
    if (!exists) return;

    pushUndo();

    setState(
      produce((s) => {
        const idx = s.elements.findIndex((e) => e.id === id);
        if (idx !== -1) {
          s.elements.splice(idx, 1);
        }
        const selIdx = s.selectedIds.indexOf(id);
        if (selIdx !== -1) {
          s.selectedIds.splice(selIdx, 1);
        }
        if (s.editingId === id) {
          s.editingId = null;
        }
      }),
    );
  }

  function updateElement(id: string, patch: ElementPatch): void {
    const idx = state.elements.findIndex((e) => e.id === id);
    if (idx === -1) return;

    pushUndo();

    setState(
      produce((s) => {
        applyElementPatch(s.elements[idx], patch);
      }),
    );
  }

  function updateElementSilent(id: string, patch: ElementPatch): void {
    const idx = state.elements.findIndex((e) => e.id === id);
    if (idx === -1) return;

    setState(
      produce((s) => {
        applyElementPatch(s.elements[idx], patch);
      }),
    );
  }

  function batchStart(): void {
    batchSnapshot = snapshotElements();
    setState(
      produce((s) => {
        s.redoStack = [];
      }),
    );
  }

  function batchCommit(): void {
    if (batchSnapshot === null) return;

    const snapshot = batchSnapshot;
    batchSnapshot = null;

    setState(
      produce((s) => {
        s.undoStack.push(snapshot);
        if (s.undoStack.length > MAX_UNDO_STACK) {
          s.undoStack.splice(0, s.undoStack.length - MAX_UNDO_STACK);
        }
      }),
    );
  }

  function undo(): void {
    if (state.undoStack.length === 0) return;

    const current = snapshotElements();
    const previous = state.undoStack[state.undoStack.length - 1];

    setState(
      produce((s) => {
        s.undoStack.pop();
        s.redoStack.push(current);
        s.elements = previous;
      }),
    );
  }

  function redo(): void {
    if (state.redoStack.length === 0) return;

    const current = snapshotElements();
    const next = state.redoStack[state.redoStack.length - 1];

    setState(
      produce((s) => {
        s.redoStack.pop();
        s.undoStack.push(current);
        s.elements = next;
      }),
    );
  }

  function setTool(tool: ToolType): void {
    setState("activeTool", tool);
  }

  function setViewport(patch: Partial<Viewport>): void {
    setState(
      produce((s) => {
        if (patch.panX !== undefined) s.viewport.panX = patch.panX;
        if (patch.panY !== undefined) s.viewport.panY = patch.panY;
        if (patch.zoom !== undefined) s.viewport.zoom = patch.zoom;
      }),
    );
  }

  function selectElement(id: string): void {
    if (state.selectedIds.includes(id)) return;
    setState(
      produce((s) => {
        s.selectedIds.push(id);
      }),
    );
  }

  function deselectElement(id: string): void {
    const idx = state.selectedIds.indexOf(id);
    if (idx === -1) return;
    setState(
      produce((s) => {
        s.selectedIds.splice(idx, 1);
      }),
    );
  }

  function deselectAll(): void {
    setState("selectedIds", []);
    setState("editingId", null);
  }

  function clearCanvas(): void {
    pushUndo();
    setState(
      produce((s) => {
        s.elements = [];
        s.selectedIds = [];
        s.editingId = null;
      }),
    );
  }

  function setEditingId(id: string | null): void {
    setState("editingId", id);
  }

  return {
    state,
    addElement,
    updateElement,
    updateElementSilent,
    removeElement,
    undo,
    redo,
    setTool,
    setViewport,
    selectElement,
    deselectElement,
    deselectAll,
    clearCanvas,
    batchStart,
    batchCommit,
    setEditingId,
  };
}
