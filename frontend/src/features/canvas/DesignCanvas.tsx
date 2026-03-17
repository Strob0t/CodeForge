import { createMemo, createSignal, For, type JSX, onCleanup, onMount, Show } from "solid-js";

import type { CanvasStore } from "./canvasState";
import type {
  AnnotationData,
  CanvasElement,
  CanvasTool,
  FreehandData,
  ImageData,
  PolygonData,
  TextData,
} from "./canvasTypes";
import { getEditableNodes } from "./tools/NodeTool";
import { catmullRomToSvgPath } from "./tools/smoothing";

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

  // Compute original bounding-box origin from raw points
  let minX = Infinity;
  let minY = Infinity;
  for (const [px, py] of points) {
    if (px < minX) minX = px;
    if (py < minY) minY = py;
  }

  // Translation offset: element may have been moved after creation
  const dx = el.x - minX;
  const dy = el.y - minY;

  // Use Catmull-Rom smoothing for multi-point paths
  const d =
    points.length === 1
      ? `M${points[0][0]},${points[0][1]}L${points[0][0]},${points[0][1]}`
      : catmullRomToSvgPath(points);

  return (
    <g transform={`translate(${dx},${dy})`}>
      <path
        d={d}
        fill="none"
        stroke={el.style.stroke}
        stroke-width={el.style.strokeWidth}
        opacity={el.style.opacity}
        stroke-linecap="round"
        stroke-linejoin="round"
      />
    </g>
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

function renderPolygon(el: CanvasElement): JSX.Element {
  const data = el.data as PolygonData;
  const vertices = data.vertices;
  if (!vertices || vertices.length === 0) return <g />;

  // Compute original bounding-box origin for translation (same pattern as freehand)
  let minX = Infinity;
  let minY = Infinity;
  for (const [px, py] of vertices) {
    if (px < minX) minX = px;
    if (py < minY) minY = py;
  }
  const dx = el.x - minX;
  const dy = el.y - minY;

  const pointsStr = vertices.map(([x, y]) => `${x},${y}`).join(" ");

  return (
    <g transform={`translate(${dx},${dy})`}>
      <polygon
        points={pointsStr}
        fill={el.style.fill}
        stroke={el.style.stroke}
        stroke-width={el.style.strokeWidth}
        opacity={el.style.opacity}
        stroke-linejoin="round"
      />
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
    case "polygon":
      return renderPolygon(el);
  }
}

// ---------------------------------------------------------------------------
// Selection overlay — dashed blue stroke around selected elements
// ---------------------------------------------------------------------------

function SelectionOverlay(props: { element: CanvasElement }): JSX.Element {
  const padding = 4;
  const HANDLE_RENDER_SIZE = 6;
  const half = HANDLE_RENDER_SIZE / 2;

  const el = createMemo(() => props.element);
  const handles = createMemo(() => {
    const e = el();
    return [
      { x: e.x, y: e.y }, // nw
      { x: e.x + e.width / 2, y: e.y }, // n
      { x: e.x + e.width, y: e.y }, // ne
      { x: e.x + e.width, y: e.y + e.height / 2 }, // e
      { x: e.x + e.width, y: e.y + e.height }, // se
      { x: e.x + e.width / 2, y: e.y + e.height }, // s
      { x: e.x, y: e.y + e.height }, // sw
      { x: e.x, y: e.y + e.height / 2 }, // w
    ];
  });

  return (
    <g pointer-events="none">
      <rect
        x={el().x - padding}
        y={el().y - padding}
        width={el().width + padding * 2}
        height={el().height + padding * 2}
        fill="none"
        stroke="var(--cf-accent)"
        stroke-width={1.5}
        stroke-dasharray="6 3"
      />
      <For each={handles()}>
        {(h) => (
          <rect
            x={h.x - half}
            y={h.y - half}
            width={HANDLE_RENDER_SIZE}
            height={HANDLE_RENDER_SIZE}
            fill="white"
            stroke="var(--cf-accent)"
            stroke-width={1.5}
          />
        )}
      </For>
    </g>
  );
}

// ---------------------------------------------------------------------------
// InlineEditor — foreignObject overlay for editing text/annotation elements
// ---------------------------------------------------------------------------

function InlineEditor(props: {
  store: CanvasStore;
  svgRef: SVGSVGElement | undefined;
}): JSX.Element {
  const editingEl = () => {
    const id = props.store.state.editingId;
    if (!id) return undefined;
    return props.store.state.elements.find((e) => e.id === id);
  };

  function handleInput(e: InputEvent): void {
    const el = editingEl();
    if (!el) return;
    const value = (e.currentTarget as HTMLTextAreaElement).value;
    if (el.type === "text") {
      props.store.updateElementSilent(el.id, { data: { text: value } as TextData });
    } else if (el.type === "annotation") {
      const annData = el.data as AnnotationData;
      props.store.updateElementSilent(el.id, {
        data: { ...annData, text: value } as AnnotationData,
      });
    }
  }

  function handleKeyDown(e: KeyboardEvent): void {
    e.stopPropagation();
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      props.store.setEditingId(null);
    }
    if (e.key === "Escape") {
      e.preventDefault();
      props.store.setEditingId(null);
    }
  }

  function handleBlur(): void {
    props.store.setEditingId(null);
  }

  return (
    <Show when={editingEl()}>
      {(el) => {
        const text = () => {
          if (el().type === "text") return (el().data as TextData).text;
          if (el().type === "annotation") return (el().data as AnnotationData).text;
          return "";
        };

        return (
          <foreignObject
            x={el().x}
            y={el().type === "annotation" ? el().y - 24 : el().y}
            width={Math.max(el().width, 100)}
            height={Math.max(el().height, 30)}
          >
            <textarea
              ref={(ref) => {
                // Auto-focus and select on mount
                setTimeout(() => {
                  ref.focus();
                  ref.select();
                }, 0);
              }}
              value={text()}
              onInput={handleInput}
              onKeyDown={handleKeyDown}
              onBlur={handleBlur}
              style={{
                width: "100%",
                height: "100%",
                border: "2px solid var(--cf-accent, #3b82f6)",
                background: "white",
                color: "black",
                padding: "2px 4px",
                "font-size": `${el().style.fontSize ?? 16}px`,
                "font-family": el().style.fontFamily ?? "sans-serif",
                resize: "none",
                outline: "none",
              }}
            />
          </foreignObject>
        );
      }}
    </Show>
  );
}

// ---------------------------------------------------------------------------
// NodeOverlay — editable node circles for polygon/freehand/annotation elements
// ---------------------------------------------------------------------------

function NodeOverlay(props: { store: CanvasStore }): JSX.Element {
  const selectedEl = () => {
    const ids = props.store.state.selectedIds;
    if (ids.length !== 1) return undefined;
    return props.store.state.elements.find((e) => e.id === ids[0]);
  };

  return (
    <Show when={props.store.state.activeTool === "node" && selectedEl()}>
      {(el) => {
        const nodes = () => getEditableNodes(el());
        return (
          <>
            <For each={nodes()}>
              {([x, y]) => (
                <circle
                  cx={x}
                  cy={y}
                  r={4}
                  fill="white"
                  stroke="var(--cf-accent)"
                  stroke-width={1.5}
                  pointer-events="none"
                />
              )}
            </For>
          </>
        );
      }}
    </Show>
  );
}

// ---------------------------------------------------------------------------
// DesignCanvas — main SVG component
// ---------------------------------------------------------------------------

export interface DesignCanvasProps {
  store: CanvasStore;
  activeTool?: CanvasTool;
  /** Callback to expose the SVG element ref to the parent (for PNG export). */
  onSvgRef?: (ref: SVGSVGElement) => void;
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
    // Forward SVG ref to parent if callback provided
    if (svgRef && props.onSvgRef) {
      props.onSvgRef(svgRef);
    }

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

  function onDblClick(e: MouseEvent): void {
    props.activeTool?.onDblClick?.(e);
  }

  return (
    <div
      ref={containerRef}
      class="relative w-full h-full overflow-hidden bg-cf-bg-surface-alt"
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
        onDblClick={onDblClick}
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

        <InlineEditor store={props.store} svgRef={svgRef} />
        <NodeOverlay store={props.store} />
      </svg>
    </div>
  );
}
