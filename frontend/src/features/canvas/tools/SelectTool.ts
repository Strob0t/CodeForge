import type { CanvasStore } from "../canvasState";
import type { CanvasElement, CanvasTool, FreehandData, PolygonData } from "../canvasTypes";
import { eventToSvg, type SvgPoint } from "./coords";

// ---------------------------------------------------------------------------
// SelectTool — select, move, and resize elements on the canvas
// ---------------------------------------------------------------------------

export type HandlePosition = "nw" | "n" | "ne" | "e" | "se" | "s" | "sw" | "w";

const HANDLE_SIZE = 8;

const HANDLE_CURSORS: Record<HandlePosition, string> = {
  nw: "nwse-resize",
  n: "ns-resize",
  ne: "nesw-resize",
  e: "ew-resize",
  se: "nwse-resize",
  s: "ns-resize",
  sw: "nesw-resize",
  w: "ew-resize",
};

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

interface ResizeState {
  elementId: string;
  handle: HandlePosition;
  startSvg: SvgPoint;
  startBounds: { x: number; y: number; width: number; height: number };
  startPoints?: [number, number][];
}

/**
 * Hit-test: find the top-most element whose bounding box contains the point.
 * Elements are tested in reverse zIndex order (highest first).
 */
export function hitTest(
  elements: readonly CanvasElement[],
  point: SvgPoint,
): CanvasElement | undefined {
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

/** Compute the SVG-space positions of the 8 resize handles for an element. */
export function getHandlePositions(el: CanvasElement): Record<HandlePosition, SvgPoint> {
  return {
    nw: { x: el.x, y: el.y },
    n: { x: el.x + el.width / 2, y: el.y },
    ne: { x: el.x + el.width, y: el.y },
    w: { x: el.x, y: el.y + el.height / 2 },
    e: { x: el.x + el.width, y: el.y + el.height / 2 },
    sw: { x: el.x, y: el.y + el.height },
    s: { x: el.x + el.width / 2, y: el.y + el.height },
    se: { x: el.x + el.width, y: el.y + el.height },
  };
}

/** Check if a point is within HANDLE_SIZE of any resize handle on the element. */
function hitTestHandle(el: CanvasElement, point: SvgPoint): HandlePosition | null {
  const positions = getHandlePositions(el);
  for (const [handle, pos] of Object.entries(positions) as [HandlePosition, SvgPoint][]) {
    if (Math.abs(point.x - pos.x) <= HANDLE_SIZE && Math.abs(point.y - pos.y) <= HANDLE_SIZE) {
      return handle;
    }
  }
  return null;
}

export function createSelectTool(options: SelectToolOptions): CanvasTool {
  let drag: DragState | null = null;
  let resize: ResizeState | null = null;

  const tool: CanvasTool = {
    cursor: "default",

    onPointerDown(e: PointerEvent): void {
      const svg = options.svgRef();
      const point = eventToSvg(e, svg);

      // Check resize handles on selected elements first
      const selected = options.store.state.selectedIds;
      if (selected.length === 1) {
        const el = options.store.state.elements.find((elem) => elem.id === selected[0]);
        if (el) {
          const handle = hitTestHandle(el, point);
          if (handle) {
            const freehandData = el.type === "freehand" ? (el.data as FreehandData) : undefined;
            const polygonData = el.type === "polygon" ? (el.data as PolygonData) : undefined;
            resize = {
              elementId: el.id,
              handle,
              startSvg: point,
              startBounds: { x: el.x, y: el.y, width: el.width, height: el.height },
              startPoints: freehandData?.points ?? polygonData?.vertices,
            };
            options.store.batchStart();
            tool.cursor = HANDLE_CURSORS[handle];
            (e.currentTarget as Element).setPointerCapture(e.pointerId);
            return;
          }
        }
      }

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
      // --- Resize mode ---
      if (resize) {
        const svg = options.svgRef();
        const point = eventToSvg(e, svg);
        const dx = point.x - resize.startSvg.x;
        const dy = point.y - resize.startSvg.y;
        const sb = resize.startBounds;
        let { x, y, width, height } = { ...sb };

        // Adjust based on handle direction
        const h = resize.handle;
        if (h.includes("w")) {
          x = sb.x + dx;
          width = sb.width - dx;
        }
        if (h.includes("e")) {
          width = sb.width + dx;
        }
        if (h === "n" || h === "nw" || h === "ne") {
          y = sb.y + dy;
          height = sb.height - dy;
        }
        if (h === "s" || h === "sw" || h === "se") {
          height = sb.height + dy;
        }

        // Enforce minimum size
        if (width < 10) {
          width = 10;
          if (h.includes("w")) x = sb.x + sb.width - 10;
        }
        if (height < 10) {
          height = 10;
          if (h === "n" || h === "nw" || h === "ne") y = sb.y + sb.height - 10;
        }

        // Shift key: constrain aspect ratio
        if (e.shiftKey && sb.width > 0 && sb.height > 0) {
          const ratio = sb.width / sb.height;
          if (h === "n" || h === "s") {
            width = height * ratio;
          } else if (h === "w" || h === "e") {
            height = width / ratio;
          } else {
            // Corner handles
            const newRatio = width / height;
            if (newRatio > ratio) {
              width = height * ratio;
            } else {
              height = width / ratio;
            }
          }
        }

        // Scale point-based elements proportionally
        const patch: Record<string, unknown> = { x, y, width, height };
        const resizeRef = resize; // capture for closure
        if (resizeRef.startPoints && sb.width > 0 && sb.height > 0) {
          const scaleX = width / sb.width;
          const scaleY = height / sb.height;
          const newPoints: [number, number][] = resizeRef.startPoints.map(([px, py]) => [
            x + (px - sb.x) * scaleX,
            y + (py - sb.y) * scaleY,
          ]);
          const el = options.store.state.elements.find((e2) => e2.id === resizeRef.elementId);
          if (el?.type === "freehand") {
            patch.data = { points: newPoints };
          } else if (el?.type === "polygon") {
            patch.data = { vertices: newPoints };
          }
        }

        options.store.updateElementSilent(resizeRef.elementId, patch);
        return;
      }

      // --- Drag mode ---
      if (drag) {
        const svg = options.svgRef();
        const point = eventToSvg(e, svg);

        const dx = point.x - drag.startSvg.x;
        const dy = point.y - drag.startSvg.y;

        options.store.updateElementSilent(drag.elementId, {
          x: drag.startX + dx,
          y: drag.startY + dy,
        });
        return;
      }

      // --- Hover cursor ---
      const svg = options.svgRef();
      const point = eventToSvg(e, svg);

      // Check handle hover on selected elements
      const selected = options.store.state.selectedIds;
      if (selected.length === 1) {
        const el = options.store.state.elements.find((e2) => e2.id === selected[0]);
        if (el) {
          const handle = hitTestHandle(el, point);
          if (handle) {
            tool.cursor = HANDLE_CURSORS[handle];
            return;
          }
        }
      }

      const hit = hitTest(options.store.state.elements, point);
      tool.cursor = hit ? "move" : "default";
    },

    onPointerUp(e: PointerEvent): void {
      if (resize) {
        (e.currentTarget as Element).releasePointerCapture(e.pointerId);
        options.store.batchCommit();
        resize = null;
        return;
      }

      if (drag) {
        (e.currentTarget as Element).releasePointerCapture(e.pointerId);
        options.store.batchCommit();
        drag = null;
      }
    },

    onDblClick(e: MouseEvent): void {
      const svg = options.svgRef();
      if (!svg) return;
      const ctm = svg.getScreenCTM();
      if (!ctm) return;
      const point = { x: (e.clientX - ctm.e) / ctm.a, y: (e.clientY - ctm.f) / ctm.d };
      const hit = hitTest(options.store.state.elements, point);
      if (hit && (hit.type === "text" || hit.type === "annotation")) {
        options.store.setEditingId(hit.id);
      }
    },
  };

  return tool;
}
