import { createEffect, createSignal, For, type JSX, on, onCleanup, onMount, Show } from "solid-js";

import type { CanvasStore } from "./canvasState";
import { exportAscii } from "./export/exportAscii";
import type { CanvasJsonExport } from "./export/exportJson";
import { exportJson } from "./export/exportJson";
import { exportPng } from "./export/exportPng";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type TabId = "png" | "ascii" | "json";

interface Tab {
  id: TabId;
  label: string;
}

const TABS: Tab[] = [
  { id: "png", label: "PNG" },
  { id: "ascii", label: "ASCII" },
  { id: "json", label: "JSON" },
];

const DEBOUNCE_MS = 500;

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface CanvasExportPanelProps {
  store: CanvasStore;
  svgRef?: () => SVGSVGElement | undefined;
  width?: number;
  onResize?: (width: number) => void;
  onClose?: () => void;
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

const PANEL_MIN_WIDTH = 200;
const PANEL_MAX_WIDTH = 480;

export function CanvasExportPanel(props: CanvasExportPanelProps): JSX.Element {
  const [activeTab, setActiveTab] = createSignal<TabId>("png");
  const [pngDataUrl, setPngDataUrl] = createSignal<string>("");
  const [asciiPreview, setAsciiPreview] = createSignal<string>("");
  const [jsonPreview, setJsonPreview] = createSignal<string>("");
  const [copyFeedback, setCopyFeedback] = createSignal<string>("");

  // Default canvas dimensions when no SVG ref is available
  const canvasWidth = (): number => {
    const svg = props.svgRef?.();
    if (svg) {
      const box = svg.viewBox.baseVal;
      return box.width > 0 ? box.width : 800;
    }
    return 800;
  };

  const canvasHeight = (): number => {
    const svg = props.svgRef?.();
    if (svg) {
      const box = svg.viewBox.baseVal;
      return box.height > 0 ? box.height : 600;
    }
    return 600;
  };

  // Debounced update of all previews when elements change
  let debounceTimer: ReturnType<typeof setTimeout> | undefined;

  createEffect(
    on(
      () => JSON.stringify(props.store.state.elements),
      () => {
        if (debounceTimer !== undefined) {
          clearTimeout(debounceTimer);
        }
        debounceTimer = setTimeout(() => {
          updatePreviews();
        }, DEBOUNCE_MS);
      },
    ),
  );

  onCleanup(() => {
    if (debounceTimer !== undefined) {
      clearTimeout(debounceTimer);
    }
  });

  // Generate previews immediately on mount (bypass debounce for initial render)
  onMount(() => updatePreviews());

  function updatePreviews(): void {
    const elements = props.store.state.elements;
    const w = canvasWidth();
    const h = canvasHeight();

    // ASCII
    setAsciiPreview(exportAscii(elements, w, h));

    // JSON
    const jsonExport: CanvasJsonExport = exportJson(elements, w, h);
    setJsonPreview(JSON.stringify(jsonExport, null, 2));

    // PNG (async, only if SVG ref is available)
    const svg = props.svgRef?.();
    if (svg) {
      void (async () => {
        try {
          const dataUrl = await exportPng(svg);
          setPngDataUrl(dataUrl);
        } catch {
          setPngDataUrl("");
        }
      })();
    } else {
      setPngDataUrl("");
    }
  }

  function copyToClipboard(content: string): void {
    function showFeedback(msg: string): void {
      setCopyFeedback(msg);
      setTimeout(() => setCopyFeedback(""), 1500);
    }

    if (navigator.clipboard?.writeText) {
      void (async () => {
        try {
          await navigator.clipboard.writeText(content);
          showFeedback("Copied!");
        } catch {
          showFeedback(execCommandCopy(content) ? "Copied!" : "Copy failed");
        }
      })();
    } else if (execCommandCopy(content)) {
      showFeedback("Copied!");
    } else {
      showFeedback("Copy failed");
    }
  }

  function execCommandCopy(text: string): boolean {
    const textarea = document.createElement("textarea");
    textarea.value = text;
    textarea.style.position = "fixed";
    textarea.style.opacity = "0";
    document.body.appendChild(textarea);
    textarea.select();
    try {
      return document.execCommand("copy");
    } catch {
      return false;
    } finally {
      document.body.removeChild(textarea);
    }
  }

  function handleCopy(): void {
    const tab = activeTab();
    switch (tab) {
      case "png":
        copyToClipboard(pngDataUrl());
        break;
      case "ascii":
        copyToClipboard(asciiPreview());
        break;
      case "json":
        copyToClipboard(jsonPreview());
        break;
    }
  }

  // -------------------------------------------------------------------------
  // Drag-to-resize handlers (left edge)
  // -------------------------------------------------------------------------

  let dragStartX = 0;
  let dragStartWidth = 0;

  function onHandlePointerDown(e: PointerEvent): void {
    e.preventDefault();
    dragStartX = e.clientX;
    dragStartWidth = props.width ?? 256;
    (e.currentTarget as Element).setPointerCapture(e.pointerId);
  }

  function onHandlePointerMove(e: PointerEvent): void {
    if (!(e.currentTarget as Element).hasPointerCapture(e.pointerId)) return;
    // Dragging left edge: dragging left = wider (subtract delta)
    const newWidth = Math.max(
      PANEL_MIN_WIDTH,
      Math.min(PANEL_MAX_WIDTH, dragStartWidth - (e.clientX - dragStartX)),
    );
    props.onResize?.(newWidth);
  }

  function onHandlePointerUp(e: PointerEvent): void {
    (e.currentTarget as Element).releasePointerCapture(e.pointerId);
  }

  const panelWidth = (): number => props.width ?? 256;

  return (
    <div
      class="relative flex h-full shrink-0 border-l border-white/10 bg-cf-bg-surface"
      style={{ width: `${panelWidth()}px` }}
      data-testid="canvas-export-panel"
    >
      {/* Drag handle on left edge */}
      <div
        class="absolute left-0 top-0 bottom-0 w-1 cursor-col-resize hover:bg-cf-accent/30 active:bg-cf-accent/50"
        onPointerDown={onHandlePointerDown}
        onPointerMove={onHandlePointerMove}
        onPointerUp={onHandlePointerUp}
      />
      <div class="flex h-full flex-1 flex-col overflow-hidden pl-1">
        {/* Tab buttons */}
        <div class="flex border-b border-white/10">
          <For each={TABS}>
            {(tab) => (
              <button
                type="button"
                class={`flex-1 px-2 py-1.5 text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cf-focus-ring focus-visible:ring-offset-2 ${
                  activeTab() === tab.id
                    ? "border-b-2 border-cf-accent text-cf-accent"
                    : "text-cf-text-muted hover:text-cf-text-primary"
                }`}
                onClick={() => setActiveTab(tab.id)}
                data-testid={`tab-${tab.id}`}
              >
                {tab.label}
              </button>
            )}
          </For>
        </div>

        {/* Copy button + feedback */}
        <div class="flex items-center justify-between border-b border-white/10 px-2 py-1">
          <span class="text-xs text-cf-success-fg">{copyFeedback()}</span>
          <button
            type="button"
            class="rounded px-2 py-0.5 text-xs text-cf-text-secondary transition-colors hover:bg-white/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cf-focus-ring focus-visible:ring-offset-2"
            onClick={handleCopy}
            data-testid="copy-button"
            aria-label="Copy export to clipboard"
          >
            Copy
          </button>
        </div>

        {/* Preview content */}
        <div class="flex-1 overflow-auto p-2">
          <Show when={activeTab() === "png"}>
            <Show
              when={pngDataUrl()}
              fallback={
                <p class="text-xs text-cf-text-muted">
                  No preview available (PNG requires the canvas SVG element)
                </p>
              }
            >
              <img
                src={pngDataUrl()}
                alt="Canvas PNG preview"
                class="max-w-full rounded border border-white/10"
                data-testid="png-preview"
              />
            </Show>
          </Show>

          <Show when={activeTab() === "ascii"}>
            <pre
              class="whitespace-pre font-mono text-xs leading-tight text-cf-text-secondary"
              data-testid="ascii-preview"
            >
              {asciiPreview() || "(empty canvas)"}
            </pre>
          </Show>

          <Show when={activeTab() === "json"}>
            <pre
              class="whitespace-pre-wrap font-mono text-xs leading-tight text-cf-text-secondary"
              data-testid="json-preview"
            >
              <code>{jsonPreview() || "{}"}</code>
            </pre>
          </Show>
        </div>
      </div>
    </div>
  );
}
