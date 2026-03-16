import type { CanvasStore } from "../canvasState";
import type { CanvasTool } from "../canvasTypes";
import { eventToSvg, type SvgPoint } from "./coords";

// ---------------------------------------------------------------------------
// RectTool — rectangle creation via drag
// ---------------------------------------------------------------------------

const MIN_SIZE = 5;

export interface RectToolOptions {
  store: CanvasStore;
  svgRef: () => SVGSVGElement | undefined;
}

interface DragState {
  start: SvgPoint;
  previewId: string | null;
}

export function createRectTool(options: RectToolOptions): CanvasTool {
  let drag: DragState | null = null;

  const tool: CanvasTool = {
    cursor: "crosshair",

    onPointerDown(e: PointerEvent): void {
      const svg = options.svgRef();
      const point = eventToSvg(e, svg);

      // Create a zero-size preview element at the start point
      const id = options.store.addElement({
        type: "rect",
        x: point.x,
        y: point.y,
        width: 0,
        height: 0,
        rotation: 0,
        style: { fill: "#ffffff", stroke: "#000000", strokeWidth: 2, opacity: 1 },
        data: {},
      });

      options.store.batchStart();
      drag = { start: point, previewId: id };

      (e.currentTarget as Element).setPointerCapture(e.pointerId);
    },

    onPointerMove(e: PointerEvent): void {
      if (!drag || !drag.previewId) return;

      const svg = options.svgRef();
      const current = eventToSvg(e, svg);

      // Compute top-left corner and dimensions (support any drag direction)
      const x = Math.min(drag.start.x, current.x);
      const y = Math.min(drag.start.y, current.y);
      const width = Math.abs(current.x - drag.start.x);
      const height = Math.abs(current.y - drag.start.y);

      options.store.updateElementSilent(drag.previewId, { x, y, width, height });
    },

    onPointerUp(e: PointerEvent): void {
      if (!drag || !drag.previewId) return;

      (e.currentTarget as Element).releasePointerCapture(e.pointerId);

      const svg = options.svgRef();
      const current = eventToSvg(e, svg);

      const width = Math.abs(current.x - drag.start.x);
      const height = Math.abs(current.y - drag.start.y);

      // Remove the preview if the drag was too small (accidental click)
      if (width < MIN_SIZE && height < MIN_SIZE) {
        options.store.removeElement(drag.previewId);
      }

      options.store.batchCommit();
      drag = null;
    },
  };

  return tool;
}
