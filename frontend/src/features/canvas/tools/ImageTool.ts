import type { CanvasStore } from "../canvasState";
import type { CanvasTool, ImageData } from "../canvasTypes";
import { eventToSvg, type SvgPoint } from "./coords";

export interface ImageToolOptions {
  store: CanvasStore;
  svgRef: () => SVGSVGElement | undefined;
}

const MAX_FILE_SIZE_BYTES = 15 * 1024 * 1024;
const DEFAULT_IMAGE_SIZE = 200;
const MIN_DRAG_SIZE = 5;

interface DragState {
  start: SvgPoint;
  previewId: string | null;
}

export function createImageTool(options: ImageToolOptions): CanvasTool {
  let drag: DragState | null = null;

  function openFileDialog(x: number, y: number, width: number, height: number): void {
    const input = document.createElement("input");
    input.type = "file";
    input.accept = "image/*";
    input.style.display = "none";

    input.addEventListener("change", () => {
      const file = input.files?.[0];
      if (!file) {
        input.remove();
        return;
      }
      if (file.size > MAX_FILE_SIZE_BYTES) {
        input.remove();
        return;
      }

      const reader = new FileReader();
      reader.onload = () => {
        const dataUrl = reader.result as string;
        options.store.addElement({
          type: "image",
          x,
          y,
          width,
          height,
          rotation: 0,
          style: { fill: "none", stroke: "none", strokeWidth: 0, opacity: 1 },
          data: { dataUrl, originalName: file.name } as ImageData,
        });
        input.remove();
      };
      reader.onerror = () => {
        input.remove();
      };
      reader.readAsDataURL(file);
    });

    document.body.appendChild(input);
    input.click();
  }

  const tool: CanvasTool = {
    cursor: "copy",

    onPointerDown(e: PointerEvent): void {
      const svg = options.svgRef();
      const start = eventToSvg(e, svg);

      // Create a dashed preview rect
      const id = options.store.addElement({
        type: "rect",
        x: start.x,
        y: start.y,
        width: 0,
        height: 0,
        rotation: 0,
        style: { fill: "none", stroke: "#6b7280", strokeWidth: 1, opacity: 0.5 },
        data: {},
      });

      options.store.batchStart();
      drag = { start, previewId: id };
      (e.currentTarget as Element).setPointerCapture(e.pointerId);
    },

    onPointerMove(e: PointerEvent): void {
      if (!drag || !drag.previewId) return;

      const svg = options.svgRef();
      const current = eventToSvg(e, svg);

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
      const end = eventToSvg(e, svg);

      const width = Math.abs(end.x - drag.start.x);
      const height = Math.abs(end.y - drag.start.y);
      const x = Math.min(drag.start.x, end.x);
      const y = Math.min(drag.start.y, end.y);

      // Remove the preview rect
      options.store.removeElement(drag.previewId);
      options.store.batchCommit();

      if (width >= MIN_DRAG_SIZE && height >= MIN_DRAG_SIZE) {
        // Dragged an area — open file dialog with dragged dimensions
        openFileDialog(x, y, width, height);
      } else {
        // Simple click — open file dialog with default size
        openFileDialog(drag.start.x, drag.start.y, DEFAULT_IMAGE_SIZE, DEFAULT_IMAGE_SIZE);
      }

      drag = null;
    },
  };

  return tool;
}
