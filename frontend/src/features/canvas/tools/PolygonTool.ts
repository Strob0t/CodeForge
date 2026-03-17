import type { CanvasStore } from "../canvasState";
import type { CanvasTool, PolygonData } from "../canvasTypes";
import { eventToSvg, type SvgPoint } from "./coords";

export interface PolygonToolOptions {
  store: CanvasStore;
  svgRef: () => SVGSVGElement | undefined;
}

const CLOSE_THRESHOLD = 10;

interface PolygonState {
  vertices: [number, number][];
  previewId: string | null;
}

export function createPolygonTool(options: PolygonToolOptions): CanvasTool {
  let state: PolygonState | null = null;

  function computeBounds(vertices: [number, number][]): {
    x: number;
    y: number;
    width: number;
    height: number;
  } {
    let minX = Infinity,
      minY = Infinity,
      maxX = -Infinity,
      maxY = -Infinity;
    for (const [px, py] of vertices) {
      if (px < minX) minX = px;
      if (py < minY) minY = py;
      if (px > maxX) maxX = px;
      if (py > maxY) maxY = py;
    }
    return { x: minX, y: minY, width: maxX - minX, height: maxY - minY };
  }

  function finalizePolygon(): void {
    if (!state || !state.previewId) return;
    if (state.vertices.length < 3) {
      options.store.removeElement(state.previewId);
      options.store.batchCommit();
      state = null;
      return;
    }
    const bounds = computeBounds(state.vertices);
    options.store.updateElementSilent(state.previewId, {
      ...bounds,
      data: { vertices: [...state.vertices] } as PolygonData,
    });
    options.store.batchCommit();
    state = null;
  }

  function isNearFirst(point: SvgPoint): boolean {
    if (!state || state.vertices.length < 3) return false;
    const [fx, fy] = state.vertices[0];
    const dx = point.x - fx;
    const dy = point.y - fy;
    return Math.sqrt(dx * dx + dy * dy) < CLOSE_THRESHOLD;
  }

  const tool: CanvasTool = {
    cursor: "crosshair",

    onPointerDown(e: PointerEvent): void {
      const svg = options.svgRef();
      const point = eventToSvg(e, svg);

      if (!state) {
        // Start a new polygon
        const id = options.store.addElement({
          type: "polygon",
          x: point.x,
          y: point.y,
          width: 0,
          height: 0,
          rotation: 0,
          style: { fill: "rgba(255,255,255,0.3)", stroke: "#000000", strokeWidth: 2, opacity: 1 },
          data: { vertices: [[point.x, point.y]] } as PolygonData,
        });
        options.store.batchStart();
        state = {
          vertices: [[point.x, point.y]],
          previewId: id,
        };
        return;
      }

      // Check if clicking near first vertex to close
      if (isNearFirst(point)) {
        finalizePolygon();
        return;
      }

      // Add vertex
      state.vertices.push([point.x, point.y]);
      const bounds = computeBounds(state.vertices);
      options.store.updateElementSilent(state.previewId as string, {
        ...bounds,
        data: { vertices: [...state.vertices] } as PolygonData,
      });
    },

    onPointerMove(e: PointerEvent): void {
      if (!state || !state.previewId) return;

      const svg = options.svgRef();
      const point = eventToSvg(e, svg);

      // Show preview with cursor as temporary last vertex
      const previewVertices: [number, number][] = [...state.vertices, [point.x, point.y]];
      const bounds = computeBounds(previewVertices);
      options.store.updateElementSilent(state.previewId, {
        ...bounds,
        data: { vertices: previewVertices } as PolygonData,
      });
    },

    onPointerUp(): void {
      // No action on pointer up for polygon tool
    },

    onDblClick(e: MouseEvent): void {
      // Double-click finalizes the polygon
      if (state) {
        e.preventDefault();
        finalizePolygon();
      }
    },
  };

  return tool;
}
