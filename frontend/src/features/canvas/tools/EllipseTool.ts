import type { CanvasStore } from "../canvasState";
import type { CanvasTool } from "../canvasTypes";
import { eventToSvg, type SvgPoint } from "./coords";

// ---------------------------------------------------------------------------
// EllipseTool — ellipse/circle creation via drag
// ---------------------------------------------------------------------------

const MIN_SIZE = 5;

export interface EllipseToolOptions {
  store: CanvasStore;
  svgRef: () => SVGSVGElement | undefined;
}

interface DragState {
  start: SvgPoint;
  previewId: string | null;
}

export function createEllipseTool(options: EllipseToolOptions): CanvasTool {
  let drag: DragState | null = null;

  const tool: CanvasTool = {
    cursor: "crosshair",

    onPointerDown(e: PointerEvent): void {
      const svg = options.svgRef();
      const point = eventToSvg(e, svg);

      // Create a zero-size preview element at the start point
      const id = options.store.addElement({
        type: "ellipse",
        x: point.x,
        y: point.y,
        width: 0,
        height: 0,
        rotation: 0,
        style: { fill: "#ffffff", stroke: "#000000", strokeWidth: 2, opacity: 1 },
        data: {},
      });

      drag = { start: point, previewId: id };

      (e.currentTarget as Element).setPointerCapture(e.pointerId);
    },

    onPointerMove(e: PointerEvent): void {
      if (!drag || !drag.previewId) return;

      const svg = options.svgRef();
      const current = eventToSvg(e, svg);

      let width = Math.abs(current.x - drag.start.x);
      let height = Math.abs(current.y - drag.start.y);

      // Hold Shift to constrain to a circle
      if (e.shiftKey) {
        const side = Math.min(width, height);
        width = side;
        height = side;
      }

      // Compute top-left corner (support any drag direction)
      const x = current.x < drag.start.x ? drag.start.x - width : drag.start.x;
      const y = current.y < drag.start.y ? drag.start.y - height : drag.start.y;

      options.store.updateElement(drag.previewId, { x, y, width, height });
    },

    onPointerUp(e: PointerEvent): void {
      if (!drag || !drag.previewId) return;

      (e.currentTarget as Element).releasePointerCapture(e.pointerId);

      const svg = options.svgRef();
      const current = eventToSvg(e, svg);

      let width = Math.abs(current.x - drag.start.x);
      let height = Math.abs(current.y - drag.start.y);

      if (e.shiftKey) {
        const side = Math.min(width, height);
        width = side;
        height = side;
      }

      // Remove the preview if the drag was too small (accidental click)
      if (width < MIN_SIZE && height < MIN_SIZE) {
        options.store.removeElement(drag.previewId);
      }

      drag = null;
    },
  };

  return tool;
}
