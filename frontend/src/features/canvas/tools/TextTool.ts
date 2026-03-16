import type { CanvasStore } from "../canvasState";
import type { CanvasTool, TextData } from "../canvasTypes";
import { eventToSvg } from "./coords";

// ---------------------------------------------------------------------------
// TextTool — click-to-place text element
// ---------------------------------------------------------------------------

export interface TextToolOptions {
  store: CanvasStore;
  svgRef: () => SVGSVGElement | undefined;
}

const DEFAULT_FONT_SIZE = 16;
const DEFAULT_TEXT = "Text";

export function createTextTool(options: TextToolOptions): CanvasTool {
  const tool: CanvasTool = {
    cursor: "text",

    onPointerDown(e: PointerEvent): void {
      const svg = options.svgRef();
      const point = eventToSvg(e, svg);

      // Estimate a bounding box based on default text and font size.
      // Approximate width: ~characters * fontSize * 0.6, height: fontSize * 1.2
      const estimatedWidth = DEFAULT_TEXT.length * DEFAULT_FONT_SIZE * 0.6;
      const estimatedHeight = DEFAULT_FONT_SIZE * 1.2;

      options.store.addElement({
        type: "text",
        x: point.x,
        y: point.y,
        width: estimatedWidth,
        height: estimatedHeight,
        rotation: 0,
        style: {
          fill: "#000000",
          stroke: "none",
          strokeWidth: 0,
          opacity: 1,
          fontSize: DEFAULT_FONT_SIZE,
          fontFamily: "sans-serif",
        },
        data: { text: DEFAULT_TEXT } as TextData,
      });
    },

    onPointerMove(): void {
      // No drag behavior for text tool
    },

    onPointerUp(): void {
      // No drag behavior for text tool
    },
  };

  return tool;
}
