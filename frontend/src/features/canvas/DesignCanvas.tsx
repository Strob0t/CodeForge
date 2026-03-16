import { createMemo, createSignal, For, type JSX, onCleanup, onMount } from "solid-js";

import type { CanvasStore } from "./canvasState";
import type {
  AnnotationData,
  CanvasElement,
  CanvasTool,
  FreehandData,
  ImageData,
  TextData,
} from "./canvasTypes";

// ---------------------------------------------------------------------------
// Pure coordinate transform — exported for unit testing
// ---------------------------------------------------------------------------

/** Point in SVG user-space coordinates. */
export interface SvgPoint {
  x: number;
  y: number;
}

/**
 * Convert screen (client) coordinates to SVG user-space coordinates
 * using the inverse of the SVG screen CTM.
 *
 * For a non-rotated, non-skewed CTM:
 *   x_svg = (clientX - ctm.e) / ctm.a
 *   y_svg = (clientY - ctm.f) / ctm.d
 */
export function screenToSvgCoords(clientX: number, clientY: number, ctm: DOMMatrix): SvgPoint {
  return {
    x: (clientX - ctm.e) / ctm.a,
    y: (clientY - ctm.f) / ctm.d,
  };
}

// ---------------------------------------------------------------------------
// Zoom constants
// ---------------------------------------------------------------------------

const ZOOM_FACTOR = 1.1;
const ZOOM_MIN = 0.1;
const ZOOM_MAX = 5.0;

// ---------------------------------------------------------------------------
// Element renderers
// ---------------------------------------------------------------------------

function renderRect(el: CanvasElement): JSX.Element {
  return (
    <rect
      x={el.x}
      y={el.y}
      width={el.width}
      height={el.height}
      fill={el.style.fill}
      stroke={el.style.stroke}
      stroke-width={el.style.strokeWidth}
      opacity={el.style.opacity}
      transform={
        el.rotation
          ? `rotate(${el.rotation} ${el.x + el.width / 2} ${el.y + el.height / 2})`
          : undefined
      }
    />
  );
}

function renderEllipse(el: CanvasElement): JSX.Element {
  const cx = el.x + el.width / 2;
  const cy = el.y + el.height / 2;
  const rx = el.width / 2;
  const ry = el.height / 2;

  return (
    <ellipse
      cx={cx}
      cy={cy}
      rx={rx}
      ry={ry}
      fill={el.style.fill}
      stroke={el.style.stroke}
      stroke-width={el.style.strokeWidth}
      opacity={el.style.opacity}
      transform={el.rotation ? `rotate(${el.rotation} ${cx} ${cy})` : undefined}
    />
  );
}

function renderFreehand(el: CanvasElement): JSX.Element {
  const data = el.data as FreehandData;
  const points = data.points;
  if (!points || points.length === 0) return <g />;

  const d =
    points.length === 1
      ? `M${points[0][0]},${points[0][1]}L${points[0][0]},${points[0][1]}`
      : `M${points[0][0]},${points[0][1]}` +
        points
          .slice(1)
          .map((p) => `L${p[0]},${p[1]}`)
          .join("");

  return (
    <path
      d={d}
      fill="none"
      stroke={el.style.stroke}
      stroke-width={el.style.strokeWidth}
      opacity={el.style.opacity}
      stroke-linecap="round"
      stroke-linejoin="round"
    />
  );
}

function renderText(el: CanvasElement): JSX.Element {
  const data = el.data as TextData;

  return (
    <text
      x={el.x}
      y={el.y}
      fill={el.style.fill}
      stroke={el.style.stroke}
      stroke-width={el.style.strokeWidth}
      opacity={el.style.opacity}
      font-size={String(el.style.fontSize ?? 16)}
      font-family={el.style.fontFamily ?? "sans-serif"}
      dominant-baseline="hanging"
    >
      {data.text}
    </text>
  );
}

function renderImage(el: CanvasElement): JSX.Element {
  const data = el.data as ImageData;

  return (
    <image
      href={data.dataUrl}
      x={el.x}
      y={el.y}
      width={el.width}
      height={el.height}
      opacity={el.style.opacity}
      transform={
        el.rotation
          ? `rotate(${el.rotation} ${el.x + el.width / 2} ${el.y + el.height / 2})`
          : undefined
      }
    />
  );
}

function renderAnnotation(el: CanvasElement): JSX.Element {
  const data = el.data as AnnotationData;

  return (
    <g opacity={el.style.opacity}>
      {data.arrowPath ? (
        <path
          d={data.arrowPath}
          fill="none"
          stroke={el.style.stroke}
          stroke-width={el.style.strokeWidth}
          marker-end="url(#arrowhead)"
        />
      ) : (
        <line
          x1={el.x}
          y1={el.y}
          x2={el.x + el.width}
          y2={el.y + el.height}
          stroke={el.style.stroke}
          stroke-width={el.style.strokeWidth}
          marker-end="url(#arrowhead)"
        />
      )}
      <text
        x={el.x + el.width / 2}
        y={el.y - 8}
        fill={el.style.fill}
        font-size={String(el.style.fontSize ?? 12)}
        font-family={el.style.fontFamily ?? "sans-serif"}
        text-anchor="middle"
      >
        {data.text}
      </text>
    </g>
  );
}

function renderElement(el: CanvasElement): JSX.Element {
  switch (el.type) {
    case "rect":
      return renderRect(el);
    case "ellipse":
      return renderEllipse(el);
    case "freehand":
      return renderFreehand(el);
    case "text":
      return renderText(el);
    case "image":
      return renderImage(el);
    case "annotation":
      return renderAnnotation(el);
  }
}

// ---------------------------------------------------------------------------
// Selection overlay — dashed blue stroke around selected elements
// ---------------------------------------------------------------------------

function SelectionOverlay(props: { element: CanvasElement }): JSX.Element {
  const padding = 4;

  return (
    <rect
      x={props.element.x - padding}
      y={props.element.y - padding}
      width={props.element.width + padding * 2}
      height={props.element.height + padding * 2}
      fill="none"
      stroke="#3b82f6"
      stroke-width={1.5}
      stroke-dasharray="6 3"
      pointer-events="none"
    />
  );
}

// ---------------------------------------------------------------------------
// DesignCanvas — main SVG component
// ---------------------------------------------------------------------------

export interface DesignCanvasProps {
  store: CanvasStore;
  activeTool?: CanvasTool;
}

export function DesignCanvas(props: DesignCanvasProps): JSX.Element {
  let svgRef: SVGSVGElement | undefined;
  let containerRef: HTMLDivElement | undefined;

  const [containerWidth, setContainerWidth] = createSignal(800);
  const [containerHeight, setContainerHeight] = createSignal(600);
  const [isPanning, setIsPanning] = createSignal(false);
  const [isSpaceHeld, setIsSpaceHeld] = createSignal(false);
  const [panStart, setPanStart] = createSignal<{ x: number; y: number } | null>(null);

  // Observe container size via ResizeObserver
  onMount(() => {
    if (!containerRef) return;

    const observer = new ResizeObserver((entries) => {
      for (const entry of entries) {
        setContainerWidth(entry.contentRect.width);
        setContainerHeight(entry.contentRect.height);
      }
    });
    observer.observe(containerRef);

    onCleanup(() => observer.disconnect());
  });

  // Keyboard listeners for space-to-pan
  function onKeyDown(e: KeyboardEvent): void {
    if (e.code === "Space" && !e.repeat) {
      e.preventDefault();
      setIsSpaceHeld(true);
    }
  }

  function onKeyUp(e: KeyboardEvent): void {
    if (e.code === "Space") {
      setIsSpaceHeld(false);
      setIsPanning(false);
      setPanStart(null);
    }
  }

  onMount(() => {
    document.addEventListener("keydown", onKeyDown);
    document.addEventListener("keyup", onKeyUp);

    onCleanup(() => {
      document.removeEventListener("keydown", onKeyDown);
      document.removeEventListener("keyup", onKeyUp);
    });
  });

  // Computed viewBox
  const viewBox = createMemo(() => {
    const vp = props.store.state.viewport;
    const x = -vp.panX / vp.zoom;
    const y = -vp.panY / vp.zoom;
    const w = containerWidth() / vp.zoom;
    const h = containerHeight() / vp.zoom;
    return `${x} ${y} ${w} ${h}`;
  });

  // Sorted elements by zIndex for rendering
  const sortedElements = createMemo(() =>
    [...props.store.state.elements].sort((a, b) => a.zIndex - b.zIndex),
  );

  // Selected element set for fast lookup
  const selectedSet = createMemo(() => new Set(props.store.state.selectedIds));

  // Cursor: space-held = grab/grabbing, otherwise delegate to tool
  const cursor = createMemo(() => {
    if (isPanning()) return "grabbing";
    if (isSpaceHeld()) return "grab";
    return props.activeTool?.cursor ?? "default";
  });

  // --- Wheel handler: zoom toward pointer ---
  function onWheel(e: WheelEvent): void {
    e.preventDefault();

    const vp = props.store.state.viewport;
    const direction = e.deltaY < 0 ? 1 : -1;
    const factor = direction > 0 ? ZOOM_FACTOR : 1 / ZOOM_FACTOR;
    const newZoom = Math.max(ZOOM_MIN, Math.min(ZOOM_MAX, vp.zoom * factor));
    const zoomRatio = newZoom / vp.zoom;

    // Analytically compute new pan so the point under the pointer stays fixed.
    // Avoids a second getScreenCTM() call which would return stale DOM state.
    props.store.setViewport({
      zoom: newZoom,
      panX: e.clientX - (e.clientX - vp.panX) * zoomRatio,
      panY: e.clientY - (e.clientY - vp.panY) * zoomRatio,
    });
  }

  // --- Pointer handlers ---
  function onPointerDown(e: PointerEvent): void {
    if (isSpaceHeld()) {
      // Start panning
      setIsPanning(true);
      setPanStart({ x: e.clientX, y: e.clientY });
      (e.currentTarget as Element).setPointerCapture(e.pointerId);
      return;
    }

    props.activeTool?.onPointerDown(e);
  }

  function onPointerMove(e: PointerEvent): void {
    if (isPanning()) {
      const start = panStart();
      if (!start) return;

      const dx = e.clientX - start.x;
      const dy = e.clientY - start.y;
      const vp = props.store.state.viewport;

      props.store.setViewport({
        panX: vp.panX + dx,
        panY: vp.panY + dy,
      });

      setPanStart({ x: e.clientX, y: e.clientY });
      return;
    }

    props.activeTool?.onPointerMove(e);
  }

  function onPointerUp(e: PointerEvent): void {
    if (isPanning()) {
      setIsPanning(false);
      setPanStart(null);
      (e.currentTarget as Element).releasePointerCapture(e.pointerId);
      return;
    }

    props.activeTool?.onPointerUp(e);
  }

  return (
    <div
      ref={containerRef}
      class="relative w-full h-full overflow-hidden bg-gray-50"
      style={{ cursor: cursor() }}
    >
      <svg
        ref={svgRef}
        viewBox={viewBox()}
        width="100%"
        height="100%"
        xmlns="http://www.w3.org/2000/svg"
        onWheel={onWheel}
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
        onPointerUp={onPointerUp}
      >
        {/* Arrowhead marker for annotations */}
        <defs>
          <marker
            id="arrowhead"
            markerWidth="10"
            markerHeight="7"
            refX="10"
            refY="3.5"
            orient="auto"
          >
            <polygon points="0 0, 10 3.5, 0 7" fill="#000" />
          </marker>
        </defs>

        {/* Render elements sorted by zIndex */}
        <For each={sortedElements()}>
          {(el) => (
            <g data-element-id={el.id}>
              {renderElement(el)}
              {selectedSet().has(el.id) && <SelectionOverlay element={el} />}
            </g>
          )}
        </For>
      </svg>
    </div>
  );
}
