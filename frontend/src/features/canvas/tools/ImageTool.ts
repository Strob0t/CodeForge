import type { CanvasStore } from "../canvasState";
import type { CanvasTool, ImageData } from "../canvasTypes";
import { eventToSvg } from "./coords";

// ---------------------------------------------------------------------------
// ImageTool — click to upload and place an image on the canvas
// ---------------------------------------------------------------------------

export interface ImageToolOptions {
  store: CanvasStore;
  svgRef: () => SVGSVGElement | undefined;
}

/** Maximum image file size: 5 MB */
const MAX_FILE_SIZE_BYTES = 5 * 1024 * 1024;

/** Default dimensions for placed images */
const DEFAULT_IMAGE_SIZE = 200;

export function createImageTool(options: ImageToolOptions): CanvasTool {
  const tool: CanvasTool = {
    cursor: "copy",

    onPointerDown(e: PointerEvent): void {
      const svg = options.svgRef();
      const point = eventToSvg(e, svg);

      // Create a hidden file input, trigger it, handle selection
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

        // Enforce 5 MB limit
        if (file.size > MAX_FILE_SIZE_BYTES) {
          input.remove();
          return;
        }

        const reader = new FileReader();
        reader.onload = () => {
          const dataUrl = reader.result as string;

          options.store.addElement({
            type: "image",
            x: point.x,
            y: point.y,
            width: DEFAULT_IMAGE_SIZE,
            height: DEFAULT_IMAGE_SIZE,
            rotation: 0,
            style: {
              fill: "none",
              stroke: "none",
              strokeWidth: 0,
              opacity: 1,
            },
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
    },

    onPointerMove(): void {
      // No drag behavior for image tool
    },

    onPointerUp(): void {
      // No drag behavior for image tool
    },
  };

  return tool;
}
