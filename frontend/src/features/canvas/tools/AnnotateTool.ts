import type { CanvasStore } from "../canvasState";
import type { AnnotationData, CanvasTool } from "../canvasTypes";
import { eventToSvg, type SvgPoint } from "./coords";

// ---------------------------------------------------------------------------
// AnnotateTool — drag to create arrow annotations with text labels
// ---------------------------------------------------------------------------

export interface AnnotateToolOptions {
  store: CanvasStore;
  svgRef: () => SVGSVGElement | undefined;
}

/** Minimum drag distance (pixels in SVG space) to avoid accidental annotations */
const MIN_DRAG_DISTANCE = 5;

const DEFAULT_ANNOTATION_TEXT = "Note";

interface DragState {
  start: SvgPoint;
  previewId: string | null;
}

export function createAnnotateTool(options: AnnotateToolOptions): CanvasTool {
  let drag: DragState | null = null;

  const tool: CanvasTool = {
    cursor: "crosshair",

    onPointerDown(e: PointerEvent): void {
      const svg = options.svgRef();
      const start = eventToSvg(e, svg);

      // Create a zero-size preview annotation at the start point
      const arrowPath = `M ${start.x} ${start.y} L ${start.x} ${start.y}`;
      const id = options.store.addElement({
        type: "annotation",
        x: start.x,
        y: start.y,
        width: 0,
        height: 0,
        rotation: 0,
        style: {
          fill: "#000000",
          stroke: "#000000",
          strokeWidth: 2,
          opacity: 1,
          fontSize: 12,
          fontFamily: "sans-serif",
        },
        data: {
          text: DEFAULT_ANNOTATION_TEXT,
          arrowPath,
        } as AnnotationData,
      });

      drag = { start, previewId: id };

      (e.currentTarget as Element).setPointerCapture(e.pointerId);
    },

    onPointerMove(e: PointerEvent): void {
      if (!drag || !drag.previewId) return;

      const svg = options.svgRef();
      const current = eventToSvg(e, svg);

      // Update preview arrow path and bounding box
      const arrowPath = `M ${drag.start.x} ${drag.start.y} L ${current.x} ${current.y}`;

      const x = Math.min(drag.start.x, current.x);
      const y = Math.min(drag.start.y, current.y);
      const width = Math.abs(current.x - drag.start.x);
      const height = Math.abs(current.y - drag.start.y);

      options.store.updateElement(drag.previewId, {
        x,
        y,
        width,
        height,
        data: {
          text: DEFAULT_ANNOTATION_TEXT,
          arrowPath,
        } as AnnotationData,
      });
    },

    onPointerUp(e: PointerEvent): void {
      if (!drag || !drag.previewId) return;

      (e.currentTarget as Element).releasePointerCapture(e.pointerId);

      const svg = options.svgRef();
      const end = eventToSvg(e, svg);

      // Calculate drag distance
      const dx = end.x - drag.start.x;
      const dy = end.y - drag.start.y;
      const distance = Math.sqrt(dx * dx + dy * dy);

      // Remove the preview if the drag was too short (accidental click)
      if (distance < MIN_DRAG_DISTANCE) {
        options.store.removeElement(drag.previewId);
      }

      drag = null;
    },
  };

  return tool;
}
