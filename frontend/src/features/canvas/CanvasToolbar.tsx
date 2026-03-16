import { createMemo, For, type JSX, onCleanup, onMount } from "solid-js";

import type { CanvasStore } from "./canvasState";
import type { ToolType } from "./canvasTypes";

// ---------------------------------------------------------------------------
// Tool definitions — label, shortcut, inline SVG icon
// ---------------------------------------------------------------------------

interface ToolDef {
  type: ToolType;
  label: string;
  shortcut: string;
  icon: () => JSX.Element;
}

/** Inline SVG icons — simple recognizable shapes, no icon library. */
const TOOL_DEFS: ToolDef[] = [
  {
    type: "select",
    label: "Select",
    shortcut: "V",
    icon: () => (
      <svg
        viewBox="0 0 24 24"
        width="18"
        height="18"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
      >
        <path d="M5 3l14 9-7 2-3 7z" />
      </svg>
    ),
  },
  {
    type: "rect",
    label: "Rectangle",
    shortcut: "R",
    icon: () => (
      <svg
        viewBox="0 0 24 24"
        width="18"
        height="18"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
      >
        <rect x="3" y="3" width="18" height="18" rx="2" />
      </svg>
    ),
  },
  {
    type: "ellipse",
    label: "Ellipse",
    shortcut: "E",
    icon: () => (
      <svg
        viewBox="0 0 24 24"
        width="18"
        height="18"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
      >
        <ellipse cx="12" cy="12" rx="10" ry="8" />
      </svg>
    ),
  },
  {
    type: "freehand",
    label: "Pen",
    shortcut: "P",
    icon: () => (
      <svg
        viewBox="0 0 24 24"
        width="18"
        height="18"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
      >
        <path d="M3 17c3-3 6 2 9-1s3-6 6-4" />
      </svg>
    ),
  },
  {
    type: "text",
    label: "Text",
    shortcut: "T",
    icon: () => (
      <svg
        viewBox="0 0 24 24"
        width="18"
        height="18"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
      >
        <line x1="6" y1="4" x2="18" y2="4" />
        <line x1="12" y1="4" x2="12" y2="20" />
      </svg>
    ),
  },
  {
    type: "annotate",
    label: "Annotate",
    shortcut: "A",
    icon: () => (
      <svg
        viewBox="0 0 24 24"
        width="18"
        height="18"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
      >
        <line x1="5" y1="19" x2="19" y2="5" />
        <polyline points="15 5 19 5 19 9" />
      </svg>
    ),
  },
  {
    type: "image",
    label: "Image",
    shortcut: "I",
    icon: () => (
      <svg
        viewBox="0 0 24 24"
        width="18"
        height="18"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
      >
        <rect x="3" y="3" width="18" height="18" rx="2" />
        <circle cx="8.5" cy="8.5" r="1.5" />
        <path d="M21 15l-5-5-8 8" />
      </svg>
    ),
  },
];

// ---------------------------------------------------------------------------
// Action button definitions — undo, redo, zoom
// ---------------------------------------------------------------------------

interface ActionDef {
  id: string;
  label: string;
  icon: () => JSX.Element;
  handler: (store: CanvasStore) => void;
}

const ZOOM_STEP = 0.2;
const ZOOM_MIN = 0.1;
const ZOOM_MAX = 5.0;

const ACTION_DEFS: ActionDef[] = [
  {
    id: "undo",
    label: "Undo (Ctrl+Z)",
    icon: () => (
      <svg
        viewBox="0 0 24 24"
        width="18"
        height="18"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
      >
        <path d="M3 10h12a5 5 0 015 5v0a5 5 0 01-5 5H10" />
        <polyline points="7 14 3 10 7 6" />
      </svg>
    ),
    handler: (store) => store.undo(),
  },
  {
    id: "redo",
    label: "Redo (Ctrl+Shift+Z)",
    icon: () => (
      <svg
        viewBox="0 0 24 24"
        width="18"
        height="18"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
      >
        <path d="M21 10H9a5 5 0 00-5 5v0a5 5 0 005 5h5" />
        <polyline points="17 14 21 10 17 6" />
      </svg>
    ),
    handler: (store) => store.redo(),
  },
  {
    id: "zoom-in",
    label: "Zoom In",
    icon: () => (
      <svg
        viewBox="0 0 24 24"
        width="18"
        height="18"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
      >
        <circle cx="11" cy="11" r="8" />
        <line x1="8" y1="11" x2="14" y2="11" />
        <line x1="11" y1="8" x2="11" y2="14" />
      </svg>
    ),
    handler: (store) => {
      const zoom = Math.min(ZOOM_MAX, store.state.viewport.zoom + ZOOM_STEP);
      store.setViewport({ zoom });
    },
  },
  {
    id: "zoom-out",
    label: "Zoom Out",
    icon: () => (
      <svg
        viewBox="0 0 24 24"
        width="18"
        height="18"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
      >
        <circle cx="11" cy="11" r="8" />
        <line x1="8" y1="11" x2="14" y2="11" />
      </svg>
    ),
    handler: (store) => {
      const zoom = Math.max(ZOOM_MIN, store.state.viewport.zoom - ZOOM_STEP);
      store.setViewport({ zoom });
    },
  },
  {
    id: "zoom-reset",
    label: "Reset Zoom",
    icon: () => (
      <svg
        viewBox="0 0 24 24"
        width="18"
        height="18"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
      >
        <circle cx="11" cy="11" r="8" />
        <text x="11" y="15" text-anchor="middle" font-size="10" fill="currentColor" stroke="none">
          1:1
        </text>
      </svg>
    ),
    handler: (store) => store.setViewport({ panX: 0, panY: 0, zoom: 1 }),
  },
];

// ---------------------------------------------------------------------------
// Keyboard shortcut mapping: key -> ToolType
// ---------------------------------------------------------------------------

const KEY_TO_TOOL: Record<string, ToolType> = {
  v: "select",
  r: "rect",
  e: "ellipse",
  p: "freehand",
  t: "text",
  a: "annotate",
  i: "image",
};

// ---------------------------------------------------------------------------
// CanvasToolbar component
// ---------------------------------------------------------------------------

export interface CanvasToolbarProps {
  store: CanvasStore;
}

export function CanvasToolbar(props: CanvasToolbarProps): JSX.Element {
  // Active tool as a reactive memo
  const activeTool = createMemo(() => props.store.state.activeTool);

  // Keyboard shortcut handler
  function handleKeyDown(e: KeyboardEvent): void {
    // Skip when user is typing in an input/textarea
    const tag = (e.target as HTMLElement)?.tagName;
    if (tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT") return;

    // Ctrl+Z / Ctrl+Shift+Z for undo/redo
    if ((e.ctrlKey || e.metaKey) && e.key === "z") {
      e.preventDefault();
      if (e.shiftKey) {
        props.store.redo();
      } else {
        props.store.undo();
      }
      return;
    }

    // Single-key tool shortcuts (no modifier)
    if (e.ctrlKey || e.metaKey || e.altKey) return;

    const tool = KEY_TO_TOOL[e.key.toLowerCase()];
    if (tool) {
      e.preventDefault();
      props.store.setTool(tool);
    }
  }

  onMount(() => {
    document.addEventListener("keydown", handleKeyDown);
    onCleanup(() => {
      document.removeEventListener("keydown", handleKeyDown);
    });
  });

  return (
    <div
      class="flex items-center gap-1 rounded-lg border border-cf-border bg-cf-bg-surface px-2 py-1 shadow-sm"
      role="toolbar"
      aria-label="Canvas tools"
    >
      {/* Tool buttons */}
      <For each={TOOL_DEFS}>
        {(def) => (
          <button
            type="button"
            class={
              "flex items-center justify-center rounded-md p-1.5 transition-colors " +
              (activeTool() === def.type
                ? "bg-blue-600 text-white"
                : "text-cf-text-secondary hover:bg-cf-bg-surface-alt hover:text-cf-text-primary")
            }
            title={`${def.label} (${def.shortcut})`}
            aria-label={def.label}
            aria-pressed={activeTool() === def.type}
            onClick={() => props.store.setTool(def.type)}
          >
            {def.icon()}
          </button>
        )}
      </For>

      {/* Separator */}
      <div class="mx-1 h-6 w-px bg-cf-border" role="separator" />

      {/* Action buttons: undo, redo, zoom */}
      <For each={ACTION_DEFS}>
        {(def) => (
          <button
            type="button"
            class="flex items-center justify-center rounded-md p-1.5 text-cf-text-secondary transition-colors hover:bg-cf-bg-surface-alt hover:text-cf-text-primary"
            title={def.label}
            aria-label={def.label}
            onClick={() => def.handler(props.store)}
          >
            {def.icon()}
          </button>
        )}
      </For>

      {/* Zoom percentage display */}
      <span class="ml-1 min-w-[3rem] text-center text-xs text-cf-text-muted" aria-live="polite">
        {Math.round(props.store.state.viewport.zoom * 100)}%
      </span>
    </div>
  );
}
