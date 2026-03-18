import { createMemo, createSignal, type JSX, onCleanup, onMount, Show } from "solid-js";
import { Portal } from "solid-js/web";

import { CanvasExportPanel } from "./CanvasExportPanel";
import type { CanvasStore } from "./canvasState";
import { createCanvasStore } from "./canvasState";
import { CanvasToolbar } from "./CanvasToolbar";
import type { CanvasExports, CanvasTool, ToolType } from "./canvasTypes";
import { DesignCanvas } from "./DesignCanvas";
import { exportAscii } from "./export/exportAscii";
import { exportJson } from "./export/exportJson";
import { exportPng } from "./export/exportPng";
import { createAnnotateTool } from "./tools/AnnotateTool";
import { createEllipseTool } from "./tools/EllipseTool";
import { createFreehandTool } from "./tools/FreehandTool";
import { createImageTool } from "./tools/ImageTool";
import { createNodeTool } from "./tools/NodeTool";
import { createPolygonTool } from "./tools/PolygonTool";
import { createRectTool } from "./tools/RectTool";
import { createSelectTool } from "./tools/SelectTool";
import { createTextTool } from "./tools/TextTool";

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface CanvasModalProps {
  open: boolean;
  onClose: () => void;
  onExport: (exports: CanvasExports) => void;
  /** Optional external store; if omitted a fresh store is created internally. */
  store?: CanvasStore;
}

// ---------------------------------------------------------------------------
// CanvasModal — fullscreen overlay wrapping toolbar + canvas
// ---------------------------------------------------------------------------

export function CanvasModal(props: CanvasModalProps): JSX.Element {
  // Use provided store or create one internally
  const internalStore = createCanvasStore();
  const resolvedStore = createMemo((): CanvasStore => props.store ?? internalStore);

  const [svgRef, setSvgRef] = createSignal<SVGSVGElement | undefined>(undefined);
  const [exportPanelOpen, setExportPanelOpen] = createSignal(false);
  const [exportPanelWidth, setExportPanelWidth] = createSignal(256);

  // Create tool instances — each tool needs store + svgRef
  const toolOpts = {
    get store() {
      return resolvedStore();
    },
    svgRef,
  };
  const toolInstances: Record<ToolType, CanvasTool> = {
    select: createSelectTool(toolOpts),
    rect: createRectTool(toolOpts),
    ellipse: createEllipseTool(toolOpts),
    freehand: createFreehandTool(toolOpts),
    text: createTextTool(toolOpts),
    annotate: createAnnotateTool(toolOpts),
    image: createImageTool(toolOpts),
    polygon: createPolygonTool(toolOpts),
    node: createNodeTool(toolOpts),
  };

  // Reactive: return the current tool instance based on store.state.activeTool
  const currentTool = createMemo((): CanvasTool => {
    const toolType = resolvedStore().state.activeTool;
    return toolInstances[toolType] ?? toolInstances.select;
  });

  // Close on Escape key
  function handleKeyDown(e: KeyboardEvent): void {
    if (e.key === "Escape") {
      e.stopPropagation();
      props.onClose();
    }
  }

  onMount(() => {
    document.addEventListener("keydown", handleKeyDown);
    onCleanup(() => {
      document.removeEventListener("keydown", handleKeyDown);
    });
  });

  // Export handler — collects all export formats and passes to callback
  function handleSendToAgent(): void {
    const store = resolvedStore();
    const elements = store.state.elements;
    const svg = svgRef();

    // Synchronous exports
    const w = svg?.viewBox.baseVal.width ?? 800;
    const h = svg?.viewBox.baseVal.height ?? 600;
    const ascii = exportAscii(elements, w, h);
    const json = exportJson(elements, w, h);

    // PNG is async — capture callback synchronously, fire after resolve
    const onExport = props.onExport;
    if (svg) {
      void exportPng(svg)
        .then((pngDataUrl) => {
          const exports: CanvasExports = { png: pngDataUrl, ascii, json };
          onExport(exports);
        })
        .catch(() => {
          // Fallback: export without PNG
          const exports: CanvasExports = { png: "", ascii, json };
          onExport(exports);
        });
    } else {
      const exports: CanvasExports = { png: "", ascii, json };
      props.onExport(exports);
    }
  }

  return (
    <Show when={props.open}>
      <Portal>
        <div
          class="fixed inset-0 z-50 flex flex-col bg-black/80"
          role="dialog"
          aria-modal="true"
          aria-label="Design Canvas"
          data-testid="canvas-modal"
        >
          {/* Top bar: toolbar + close/export buttons */}
          <div class="flex items-center justify-between border-b border-white/10 bg-cf-bg-surface px-3 py-2">
            <CanvasToolbar
              store={resolvedStore()}
              onToggleExport={() => setExportPanelOpen((prev) => !prev)}
              exportOpen={exportPanelOpen()}
            />

            <div class="flex items-center gap-2">
              {/* Send to Agent button */}
              <button
                type="button"
                class="rounded-md bg-cf-accent px-3 py-1.5 text-sm font-medium text-cf-accent-fg transition-colors hover:bg-cf-accent-hover"
                onClick={handleSendToAgent}
                aria-label="Send to Agent"
              >
                Send to Agent
              </button>

              {/* Close button */}
              <button
                type="button"
                class="flex items-center justify-center rounded-md p-1.5 text-cf-text-muted transition-colors hover:bg-white/10 hover:text-cf-text-primary"
                onClick={() => props.onClose()}
                aria-label="Close canvas"
              >
                <svg
                  viewBox="0 0 24 24"
                  width="20"
                  height="20"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="2"
                  stroke-linecap="round"
                  stroke-linejoin="round"
                >
                  <line x1="18" y1="6" x2="6" y2="18" />
                  <line x1="6" y1="6" x2="18" y2="18" />
                </svg>
              </button>
            </div>
          </div>

          {/* Main area: canvas + right sidebar with export panel */}
          <div class="flex flex-1 overflow-hidden">
            {/* Canvas fills center */}
            <div class="flex-1 overflow-hidden">
              <DesignCanvas
                store={resolvedStore()}
                activeTool={currentTool()}
                onSvgRef={setSvgRef}
              />
            </div>

            {/* Right sidebar: export panel (collapsible + resizable) */}
            <Show when={exportPanelOpen()}>
              <CanvasExportPanel
                store={resolvedStore()}
                svgRef={svgRef}
                width={exportPanelWidth()}
                onResize={setExportPanelWidth}
                onClose={() => setExportPanelOpen(false)}
              />
            </Show>
          </div>
        </div>
      </Portal>
    </Show>
  );
}
