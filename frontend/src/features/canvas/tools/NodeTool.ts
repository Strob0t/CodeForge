import type { CanvasStore } from "../canvasState";
import type {
  AnnotationData,
  CanvasElement,
  CanvasTool,
  FreehandData,
  PolygonData,
} from "../canvasTypes";
import { eventToSvg, type SvgPoint } from "./coords";
import { hitTest } from "./SelectTool";

export interface NodeToolOptions {
  store: CanvasStore;
  svgRef: () => SVGSVGElement | undefined;
}

const NODE_HIT_RADIUS = 8;

/** Get the editable nodes for an element. For freehand, thin out dense paths (every 5th point). */
export function getEditableNodes(el: CanvasElement): [number, number][] {
  if (el.type === "polygon") {
    return (el.data as PolygonData).vertices ?? [];
  }
  if (el.type === "freehand") {
    const pts = (el.data as FreehandData).points ?? [];
    if (pts.length <= 20) return pts;
    // Show every 5th point for dense paths, plus always include last point
    const sparse: [number, number][] = [];
    for (let i = 0; i < pts.length; i++) {
      if (i % 5 === 0) sparse.push(pts[i]);
    }
    if ((pts.length - 1) % 5 !== 0) sparse.push(pts[pts.length - 1]);
    return sparse;
  }
  if (el.type === "annotation") {
    // Annotation: start and end points
    // Start is at el.x, el.y; end is at el.x + el.width, el.y + el.height
    return [
      [el.x, el.y],
      [el.x + el.width, el.y + el.height],
    ];
  }
  return [];
}

interface NodeDragState {
  elementId: string;
  nodeIndex: number;
  /** Index in the original full data array (different from display index for thinned freehand) */
  dataIndex: number;
  startSvg: SvgPoint;
  startPoint: [number, number];
}

export function createNodeTool(options: NodeToolOptions): CanvasTool {
  let nodeDrag: NodeDragState | null = null;

  function findClosestNode(
    el: CanvasElement,
    point: SvgPoint,
  ): { nodeIndex: number; dataIndex: number } | null {
    const nodes = getEditableNodes(el);
    let bestIdx = -1;
    let bestDist = Infinity;

    for (let i = 0; i < nodes.length; i++) {
      const [nx, ny] = nodes[i];
      const dist = Math.sqrt((point.x - nx) ** 2 + (point.y - ny) ** 2);
      if (dist < NODE_HIT_RADIUS && dist < bestDist) {
        bestDist = dist;
        bestIdx = i;
      }
    }

    if (bestIdx === -1) return null;

    // Map display index to data index
    if (el.type === "freehand") {
      const pts = (el.data as FreehandData).points ?? [];
      if (pts.length <= 20) return { nodeIndex: bestIdx, dataIndex: bestIdx };
      // Thinned: reconstruct the actual index
      const displayNode = nodes[bestIdx];
      const dataIdx = pts.findIndex(([px, py]) => px === displayNode[0] && py === displayNode[1]);
      return { nodeIndex: bestIdx, dataIndex: dataIdx >= 0 ? dataIdx : bestIdx };
    }

    return { nodeIndex: bestIdx, dataIndex: bestIdx };
  }

  function recomputeBounds(el: CanvasElement): void {
    let points: [number, number][] = [];
    if (el.type === "polygon") points = (el.data as PolygonData).vertices ?? [];
    else if (el.type === "freehand") points = (el.data as FreehandData).points ?? [];
    else return;

    if (points.length === 0) return;

    let minX = Infinity,
      minY = Infinity,
      maxX = -Infinity,
      maxY = -Infinity;
    for (const [px, py] of points) {
      if (px < minX) minX = px;
      if (py < minY) minY = py;
      if (px > maxX) maxX = px;
      if (py > maxY) maxY = py;
    }
    options.store.updateElementSilent(el.id, {
      x: minX,
      y: minY,
      width: maxX - minX,
      height: maxY - minY,
    });
  }

  const tool: CanvasTool = {
    cursor: "crosshair",

    onPointerDown(e: PointerEvent): void {
      const svg = options.svgRef();
      const point = eventToSvg(e, svg);

      // Check if clicking a node on the selected element
      const selected = options.store.state.selectedIds;
      if (selected.length === 1) {
        const el = options.store.state.elements.find((elem) => elem.id === selected[0]);
        if (el) {
          const nodeHit = findClosestNode(el, point);
          if (nodeHit) {
            const allNodes = getEditableNodes(el);
            options.store.batchStart();
            nodeDrag = {
              elementId: el.id,
              nodeIndex: nodeHit.nodeIndex,
              dataIndex: nodeHit.dataIndex,
              startSvg: point,
              startPoint: [...allNodes[nodeHit.nodeIndex]] as [number, number],
            };
            tool.cursor = "grabbing";
            (e.currentTarget as Element).setPointerCapture(e.pointerId);
            return;
          }
        }
      }

      // Otherwise, try to select an element
      const hit = hitTest(options.store.state.elements, point);
      if (hit) {
        const hasNodes =
          hit.type === "polygon" || hit.type === "freehand" || hit.type === "annotation";
        if (hasNodes) {
          options.store.deselectAll();
          options.store.selectElement(hit.id);
          tool.cursor = "crosshair";
        }
      } else {
        options.store.deselectAll();
      }
    },

    onPointerMove(e: PointerEvent): void {
      if (!nodeDrag) return;

      const svg = options.svgRef();
      const point = eventToSvg(e, svg);
      const dx = point.x - nodeDrag.startSvg.x;
      const dy = point.y - nodeDrag.startSvg.y;
      const newX = nodeDrag.startPoint[0] + dx;
      const newY = nodeDrag.startPoint[1] + dy;

      const dragRef = nodeDrag;
      const el = options.store.state.elements.find((elem) => elem.id === dragRef.elementId);
      if (!el) return;

      if (el.type === "polygon") {
        const verts = [...(el.data as PolygonData).vertices];
        verts[nodeDrag.dataIndex] = [newX, newY];
        options.store.updateElementSilent(el.id, {
          data: { vertices: verts } as PolygonData,
        });
      } else if (el.type === "freehand") {
        const pts = [...(el.data as FreehandData).points];
        pts[nodeDrag.dataIndex] = [newX, newY];
        options.store.updateElementSilent(el.id, {
          data: { points: pts } as FreehandData,
        });
      } else if (el.type === "annotation") {
        const data = el.data as AnnotationData;
        if (nodeDrag.dataIndex === 0) {
          // Moving start point
          const endX = el.x + el.width;
          const endY = el.y + el.height;
          const x = Math.min(newX, endX);
          const y = Math.min(newY, endY);
          const width = Math.abs(endX - newX);
          const height = Math.abs(endY - newY);
          const arrowPath = `M ${newX} ${newY} L ${endX} ${endY}`;
          options.store.updateElementSilent(el.id, {
            x,
            y,
            width,
            height,
            data: { ...data, arrowPath } as AnnotationData,
          });
        } else {
          // Moving end point
          const origStartX = data.arrowPath ? parseFloat(data.arrowPath.split(" ")[1]) : el.x;
          const origStartY = data.arrowPath ? parseFloat(data.arrowPath.split(" ")[2]) : el.y;
          const x = Math.min(origStartX, newX);
          const y = Math.min(origStartY, newY);
          const width = Math.abs(newX - origStartX);
          const height = Math.abs(newY - origStartY);
          const arrowPath = `M ${origStartX} ${origStartY} L ${newX} ${newY}`;
          options.store.updateElementSilent(el.id, {
            x,
            y,
            width,
            height,
            data: { ...data, arrowPath } as AnnotationData,
          });
        }
        return;
      }

      // Recompute bounding box for polygon/freehand
      const updatedEl = options.store.state.elements.find((elem) => elem.id === dragRef.elementId);
      if (updatedEl) recomputeBounds(updatedEl);
    },

    onPointerUp(e: PointerEvent): void {
      if (nodeDrag) {
        (e.currentTarget as Element).releasePointerCapture(e.pointerId);
        options.store.batchCommit();
        nodeDrag = null;
        tool.cursor = "crosshair";
      }
    },
  };

  return tool;
}
