import type { CanvasStore } from "../canvasState";
import type { CanvasElement, CanvasTool } from "../canvasTypes";
import { eventToSvg, type SvgPoint } from "./coords";

// ---------------------------------------------------------------------------
// SelectTool — select, move, (future: resize) elements on the canvas
// ---------------------------------------------------------------------------

export interface SelectToolOptions {
  store: CanvasStore;
  svgRef: () => SVGSVGElement | undefined;
}

interface DragState {
  elementId: string;
  startSvg: SvgPoint;
  startX: number;
  startY: number;
}

/**
 * Hit-test: find the top-most element whose bounding box contains the point.
 * Elements are tested in reverse zIndex order (highest first).
 */
function hitTest(elements: readonly CanvasElement[], point: SvgPoint): CanvasElement | undefined {
  // Sort descending by zIndex to test top elements first
  const sorted = [...elements].sort((a, b) => b.zIndex - a.zIndex);

  for (const el of sorted) {
    if (
      point.x >= el.x &&
      point.x <= el.x + el.width &&
      point.y >= el.y &&
      point.y <= el.y + el.height
    ) {
      return el;
    }
  }

  return undefined;
}

export function createSelectTool(options: SelectToolOptions): CanvasTool {
  let drag: DragState | null = null;

  const tool: CanvasTool = {
    cursor: "default",

    onPointerDown(e: PointerEvent): void {
      const svg = options.svgRef();
      const point = eventToSvg(e, svg);
      const hit = hitTest(options.store.state.elements, point);

      if (hit) {
        options.store.deselectAll();
        options.store.selectElement(hit.id);
        options.store.batchStart();

        drag = {
          elementId: hit.id,
          startSvg: point,
          startX: hit.x,
          startY: hit.y,
        };

        tool.cursor = "move";

        // Capture pointer for reliable drag tracking
        (e.currentTarget as Element).setPointerCapture(e.pointerId);
      } else {
        options.store.deselectAll();
        drag = null;
        tool.cursor = "default";
      }
    },

    onPointerMove(e: PointerEvent): void {
      if (!drag) {
        // Hover cursor: check if over an element
        const svg = options.svgRef();
        const point = eventToSvg(e, svg);
        const hit = hitTest(options.store.state.elements, point);
        tool.cursor = hit ? "move" : "default";
        return;
      }

      // Dragging — update element position
      const svg = options.svgRef();
      const point = eventToSvg(e, svg);

      const dx = point.x - drag.startSvg.x;
      const dy = point.y - drag.startSvg.y;

      options.store.updateElementSilent(drag.elementId, {
        x: drag.startX + dx,
        y: drag.startY + dy,
      });
    },

    onPointerUp(e: PointerEvent): void {
      if (drag) {
        (e.currentTarget as Element).releasePointerCapture(e.pointerId);
        options.store.batchCommit();
        drag = null;
      }
    },
  };

  return tool;
}
