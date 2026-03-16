import type { CanvasStore } from "../canvasState";
import type { CanvasTool, FreehandData } from "../canvasTypes";
import { eventToSvg } from "./coords";

// Re-export for convenience — test imports from smoothing.ts directly
export { catmullRomToSvgPath } from "./smoothing";

// ---------------------------------------------------------------------------
// FreehandTool — freehand SVG path drawing via pointer drag
// ---------------------------------------------------------------------------

export interface FreehandToolOptions {
  store: CanvasStore;
  svgRef: () => SVGSVGElement | undefined;
}

interface DrawState {
  points: [number, number][];
  /** ID of the live-preview element in the store. */
  previewId: string | null;
}

export function createFreehandTool(options: FreehandToolOptions): CanvasTool {
  let draw: DrawState | null = null;

  const tool: CanvasTool = {
    cursor: "crosshair",

    onPointerDown(e: PointerEvent): void {
      const svg = options.svgRef();
      const point = eventToSvg(e, svg);

      draw = {
        points: [[point.x, point.y]],
        previewId: null,
      };

      // Create a preview element with a single point
      const id = options.store.addElement({
        type: "freehand",
        x: point.x,
        y: point.y,
        width: 0,
        height: 0,
        rotation: 0,
        style: { fill: "none", stroke: "#000000", strokeWidth: 2, opacity: 1 },
        data: { points: [[point.x, point.y]] } as FreehandData,
      });
      draw.previewId = id;
      options.store.batchStart();

      (e.currentTarget as Element).setPointerCapture(e.pointerId);
    },

    onPointerMove(e: PointerEvent): void {
      if (!draw || !draw.previewId) return;

      const svg = options.svgRef();
      const point = eventToSvg(e, svg);

      draw.points.push([point.x, point.y]);

      // Update preview with raw points (polyline for live feedback)
      options.store.updateElementSilent(draw.previewId, {
        data: { points: [...draw.points] } as FreehandData,
      });
    },

    onPointerUp(e: PointerEvent): void {
      if (!draw || !draw.previewId) return;

      (e.currentTarget as Element).releasePointerCapture(e.pointerId);

      const points = draw.points;

      if (points.length < 2) {
        // Single click -> keep as dot
        options.store.batchCommit();
        draw = null;
        return;
      }

      // Compute bounding box for the element
      let minX = Infinity;
      let minY = Infinity;
      let maxX = -Infinity;
      let maxY = -Infinity;
      for (const [px, py] of points) {
        if (px < minX) minX = px;
        if (py < minY) minY = py;
        if (px > maxX) maxX = px;
        if (py > maxY) maxY = py;
      }

      // Update the preview element with smoothed path data and bounding box
      options.store.updateElementSilent(draw.previewId, {
        x: minX,
        y: minY,
        width: maxX - minX,
        height: maxY - minY,
        data: { points: [...points] } as FreehandData,
      });

      options.store.batchCommit();
      draw = null;
    },
  };

  return tool;
}
