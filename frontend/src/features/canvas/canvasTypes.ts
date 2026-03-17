// Canvas element style properties
export interface ElementStyle {
  fill: string;
  stroke: string;
  strokeWidth: number;
  opacity: number;
  fontSize?: number;
  fontFamily?: string;
}

// Per-type data payloads (discriminated by CanvasElement.type)
// Reserved for future rect-specific properties (e.g. cornerRadius)
export type RectData = Record<string, never>;

// Reserved for future ellipse-specific properties
export type EllipseData = Record<string, never>;

export interface FreehandData {
  points: [number, number][];
}

export interface TextData {
  text: string;
}

export interface ImageData {
  dataUrl: string;
  originalName: string;
}

export interface AnnotationData {
  text: string;
  targetElementId?: string;
  arrowPath?: string;
}

export interface PolygonData {
  vertices: [number, number][];
}

// Union of all element data types
export type ElementData =
  | RectData
  | EllipseData
  | FreehandData
  | TextData
  | ImageData
  | AnnotationData
  | PolygonData;

// Element type discriminator
export type ElementType =
  | "rect"
  | "ellipse"
  | "freehand"
  | "text"
  | "image"
  | "annotation"
  | "polygon";

// A single element on the canvas
export interface CanvasElement {
  id: string;
  type: ElementType;
  x: number;
  y: number;
  width: number;
  height: number;
  rotation: number;
  zIndex: number;
  style: ElementStyle;
  data: ElementData;
}

// Available tools
export type ToolType =
  | "select"
  | "rect"
  | "ellipse"
  | "freehand"
  | "text"
  | "annotate"
  | "image"
  | "polygon"
  | "node";

// Tool interface for pointer-event-driven tools
export interface CanvasTool {
  onPointerDown: (event: PointerEvent) => void;
  onPointerMove: (event: PointerEvent) => void;
  onPointerUp: (event: PointerEvent) => void;
  onDblClick?: (event: MouseEvent) => void;
  cursor: string;
}

// Viewport state (pan + zoom)
export interface Viewport {
  panX: number;
  panY: number;
  zoom: number;
}

// Context passed to tools and child components
export interface CanvasContext {
  store: CanvasStoreState;
  svgRef: SVGSVGElement | undefined;
  screenToSvg: (clientX: number, clientY: number) => { x: number; y: number };
}

// Export outputs
export interface CanvasExports {
  png: string;
  ascii: string;
  json: object;
}

// Internal store shape
export interface CanvasStoreState {
  elements: CanvasElement[];
  selectedIds: string[];
  activeTool: ToolType;
  viewport: Viewport;
  undoStack: CanvasElement[][];
  redoStack: CanvasElement[][];
  editingId: string | null;
}
