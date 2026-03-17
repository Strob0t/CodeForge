import type {
  AnnotationData,
  CanvasElement,
  FreehandData,
  ImageData,
  PolygonData,
  TextData,
} from "../canvasTypes";

// ---------------------------------------------------------------------------
// Constants — pixel-to-char mapping
// ---------------------------------------------------------------------------

/** One character cell = 8px wide. */
const CHAR_W = 8;
/** One character cell = 16px tall. */
const CHAR_H = 16;

// ---------------------------------------------------------------------------
// Grid helpers
// ---------------------------------------------------------------------------

/** Create a 2D grid of spaces with the given dimensions. */
function createGrid(cols: number, rows: number): string[][] {
  const grid: string[][] = [];
  for (let r = 0; r < rows; r++) {
    const row: string[] = [];
    for (let c = 0; c < cols; c++) {
      row.push(" ");
    }
    grid.push(row);
  }
  return grid;
}

/** Safely set a character in the grid (bounds-checked). */
function setChar(grid: string[][], col: number, row: number, ch: string): void {
  if (row >= 0 && row < grid.length && col >= 0 && col < grid[0].length) {
    grid[row][col] = ch;
  }
}

/** Convert pixel coordinate to grid column. */
function pxToCol(px: number): number {
  return Math.floor(px / CHAR_W);
}

/** Convert pixel coordinate to grid row. */
function pxToRow(px: number): number {
  return Math.floor(px / CHAR_H);
}

// ---------------------------------------------------------------------------
// Per-type rasterizers
// ---------------------------------------------------------------------------

function rasterizeRect(grid: string[][], el: CanvasElement): void {
  const col0 = pxToCol(el.x);
  const row0 = pxToRow(el.y);
  const cols = pxToCol(el.width);
  const rows = pxToRow(el.height);
  if (cols < 1 || rows < 1) return;

  const lastCol = col0 + cols - 1;
  const lastRow = row0 + rows - 1;

  for (let c = col0; c <= lastCol; c++) {
    for (let r = row0; r <= lastRow; r++) {
      if ((r === row0 || r === lastRow) && (c === col0 || c === lastCol)) {
        setChar(grid, c, r, "+");
      } else if (r === row0 || r === lastRow) {
        setChar(grid, c, r, "-");
      } else if (c === col0 || c === lastCol) {
        setChar(grid, c, r, "|");
      }
    }
  }
}

function rasterizeEllipse(grid: string[][], el: CanvasElement): void {
  const col0 = pxToCol(el.x);
  const row0 = pxToRow(el.y);
  const cols = pxToCol(el.width);
  const rows = pxToRow(el.height);
  if (cols < 1 || rows < 1) return;

  const lastCol = col0 + cols - 1;
  const lastRow = row0 + rows - 1;

  for (let c = col0; c <= lastCol; c++) {
    for (let r = row0; r <= lastRow; r++) {
      if ((r === row0 || r === lastRow) && (c === col0 || c === lastCol)) {
        setChar(grid, c, r, "(");
      } else if (r === row0 || r === lastRow) {
        setChar(grid, c, r, "-");
      } else if (c === col0 || c === lastCol) {
        setChar(grid, c, r, "|");
      }
    }
  }
  // Close with ) on right side of top and bottom
  setChar(grid, col0 + cols - 1, row0, ")");
  setChar(grid, col0 + cols - 1, lastRow, ")");
}

function rasterizeText(grid: string[][], el: CanvasElement): void {
  const data = el.data as TextData;
  const text = data.text;
  if (!text) return;

  const col0 = pxToCol(el.x);
  const row0 = pxToRow(el.y);

  for (let i = 0; i < text.length; i++) {
    setChar(grid, col0 + i, row0, text[i]);
  }
}

function rasterizeFreehand(grid: string[][], el: CanvasElement): void {
  const data = el.data as FreehandData;
  if (!data.points || data.points.length === 0) return;

  for (const [px, py] of data.points) {
    const col = pxToCol(px);
    const row = pxToRow(py);
    setChar(grid, col, row, "*");
  }
}

function rasterizeImage(grid: string[][], el: CanvasElement): void {
  const data = el.data as ImageData;
  const label = `[img: ${data.originalName}]`;

  const col0 = pxToCol(el.x);
  const row0 = pxToRow(el.y);
  const cols = pxToCol(el.width);
  const rows = pxToRow(el.height);
  if (cols < 1 || rows < 1) return;

  // Center the label in the middle row
  const midRow = row0 + Math.floor(rows / 2);
  const startCol = col0 + Math.max(0, Math.floor((cols - label.length) / 2));

  for (let i = 0; i < label.length; i++) {
    setChar(grid, startCol + i, midRow, label[i]);
  }
}

function rasterizeLine(
  grid: string[][],
  x0: number,
  y0: number,
  x1: number,
  y1: number,
  ch: string,
): void {
  const dx = Math.abs(x1 - x0);
  const dy = Math.abs(y1 - y0);
  const sx = x0 < x1 ? 1 : -1;
  const sy = y0 < y1 ? 1 : -1;
  let err = dx - dy;
  let cx = x0,
    cy = y0;
  while (true) {
    setChar(grid, cx, cy, ch);
    if (cx === x1 && cy === y1) break;
    const e2 = 2 * err;
    if (e2 > -dy) {
      err -= dy;
      cx += sx;
    }
    if (e2 < dx) {
      err += dx;
      cy += sy;
    }
  }
}

function rasterizePolygon(grid: string[][], el: CanvasElement): void {
  const data = el.data as PolygonData;
  if (!data.vertices || data.vertices.length < 2) return;
  const verts = data.vertices;
  for (let i = 0; i < verts.length; i++) {
    const [ax, ay] = verts[i];
    const [bx, by] = verts[(i + 1) % verts.length];
    rasterizeLine(grid, pxToCol(ax), pxToRow(ay), pxToCol(bx), pxToRow(by), "*");
  }
}

function rasterizeAnnotation(grid: string[][], el: CanvasElement): void {
  const data = el.data as AnnotationData;
  const col0 = pxToCol(el.x);
  const row0 = pxToRow(el.y);
  const cols = pxToCol(el.width);

  // Draw arrow: ----> across the width
  if (cols >= 3) {
    for (let c = col0; c < col0 + cols - 2; c++) {
      setChar(grid, c, row0, "-");
    }
    setChar(grid, col0 + cols - 2, row0, "-");
    setChar(grid, col0 + cols - 1, row0, ">");
  } else if (cols >= 1) {
    setChar(grid, col0, row0, ">");
  }

  // Place text label after the arrow
  const text = data.text;
  if (text) {
    const labelCol = col0 + cols + 1;
    for (let i = 0; i < text.length; i++) {
      setChar(grid, labelCol + i, row0, text[i]);
    }
  }
}

// ---------------------------------------------------------------------------
// Main export function
// ---------------------------------------------------------------------------

/**
 * Render canvas elements to an ASCII art string.
 *
 * Grid mapping: 1 char = 8px wide, 16px tall.
 * Elements are rendered in zIndex order (lower first), so higher-zIndex
 * elements overwrite lower ones at the same position.
 *
 * @returns multi-line string with rows joined by newline, trailing spaces trimmed.
 */
export function exportAscii(
  elements: CanvasElement[],
  canvasWidth: number,
  canvasHeight: number,
): string {
  const cols = pxToCol(canvasWidth);
  const rows = pxToRow(canvasHeight);

  if (cols <= 0 || rows <= 0) return "";
  if (elements.length === 0) return "";

  const grid = createGrid(cols, rows);

  // Sort by zIndex ascending — later elements overwrite earlier ones
  const sorted = [...elements].sort((a, b) => a.zIndex - b.zIndex);

  for (const el of sorted) {
    switch (el.type) {
      case "rect":
        rasterizeRect(grid, el);
        break;
      case "ellipse":
        rasterizeEllipse(grid, el);
        break;
      case "text":
        rasterizeText(grid, el);
        break;
      case "freehand":
        rasterizeFreehand(grid, el);
        break;
      case "image":
        rasterizeImage(grid, el);
        break;
      case "annotation":
        rasterizeAnnotation(grid, el);
        break;
      case "polygon":
        rasterizePolygon(grid, el);
        break;
    }
  }

  // Convert grid to string, trimming trailing spaces from each row
  const lines = grid.map((row) => row.join("").trimEnd());

  // Remove trailing empty lines
  while (lines.length > 0 && lines[lines.length - 1] === "") {
    lines.pop();
  }

  return lines.join("\n");
}
