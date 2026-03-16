import type {
  AnnotationData,
  CanvasElement,
  ElementData,
  ElementStyle,
  ImageData,
} from "../canvasTypes";

// ---------------------------------------------------------------------------
// Export types
// ---------------------------------------------------------------------------

export interface ExportedElement {
  id: string;
  type: CanvasElement["type"];
  x: number;
  y: number;
  width: number;
  height: number;
  rotation: number;
  zIndex: number;
  style: ElementStyle;
  data: ElementData;
}

export interface ExportedAnnotation {
  id: string;
  x: number;
  y: number;
  width: number;
  height: number;
  text: string;
  targetElementId?: string;
}

export interface CanvasJsonExport {
  canvas: { width: number; height: number };
  elements: ExportedElement[];
  annotations: ExportedAnnotation[];
}

// ---------------------------------------------------------------------------
// Implementation
// ---------------------------------------------------------------------------

/**
 * Export canvas elements to a structured JSON description suitable for
 * LLM consumption. Images have their dataUrl stripped (too large).
 * Annotations are separated into their own array for clarity.
 * Elements are sorted by zIndex ascending.
 */
export function exportJson(
  elements: CanvasElement[],
  canvasWidth: number,
  canvasHeight: number,
): CanvasJsonExport {
  const sorted = [...elements].sort((a, b) => a.zIndex - b.zIndex);

  const exportedElements: ExportedElement[] = [];
  const exportedAnnotations: ExportedAnnotation[] = [];

  for (const el of sorted) {
    if (el.type === "annotation") {
      const annData = el.data as AnnotationData;
      exportedAnnotations.push({
        id: el.id,
        x: el.x,
        y: el.y,
        width: el.width,
        height: el.height,
        text: annData.text,
        targetElementId: annData.targetElementId,
      });
    } else {
      exportedElements.push({
        id: el.id,
        type: el.type,
        x: el.x,
        y: el.y,
        width: el.width,
        height: el.height,
        rotation: el.rotation,
        zIndex: el.zIndex,
        style: { ...el.style },
        data: el.type === "image" ? stripImageDataUrl(el.data as ImageData) : { ...el.data },
      });
    }
  }

  return {
    canvas: { width: canvasWidth, height: canvasHeight },
    elements: exportedElements,
    annotations: exportedAnnotations,
  };
}

/** Strip the large dataUrl from image data, keep only the file name. */
function stripImageDataUrl(data: ImageData): ElementData {
  return { originalName: data.originalName };
}
