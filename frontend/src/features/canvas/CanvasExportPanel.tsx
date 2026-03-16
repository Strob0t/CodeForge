import { createEffect, createSignal, For, type JSX, on, onCleanup, Show } from "solid-js";

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
  svgRef?: SVGSVGElement;
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function CanvasExportPanel(props: CanvasExportPanelProps): JSX.Element {
  const [activeTab, setActiveTab] = createSignal<TabId>("png");
  const [pngDataUrl, setPngDataUrl] = createSignal<string>("");
  const [asciiPreview, setAsciiPreview] = createSignal<string>("");
  const [jsonPreview, setJsonPreview] = createSignal<string>("");
  const [copyFeedback, setCopyFeedback] = createSignal<string>("");

  // Default canvas dimensions when no SVG ref is available
  const canvasWidth = (): number => {
    const svg = props.svgRef;
    if (svg) {
      const box = svg.viewBox.baseVal;
      return box.width > 0 ? box.width : 800;
    }
    return 800;
  };

  const canvasHeight = (): number => {
    const svg = props.svgRef;
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
    const svg = props.svgRef;
    if (svg) {
      exportPng(svg)
        .then((dataUrl) => setPngDataUrl(dataUrl))
        .catch(() => setPngDataUrl(""));
    } else {
      setPngDataUrl("");
    }
  }

  async function copyToClipboard(content: string): Promise<void> {
    try {
      await navigator.clipboard.writeText(content);
      setCopyFeedback("Copied!");
      setTimeout(() => setCopyFeedback(""), 1500);
    } catch {
      setCopyFeedback("Copy failed");
      setTimeout(() => setCopyFeedback(""), 1500);
    }
  }

  function handleCopy(): void {
    const tab = activeTab();
    switch (tab) {
      case "png":
        void copyToClipboard(pngDataUrl());
        break;
      case "ascii":
        void copyToClipboard(asciiPreview());
        break;
      case "json":
        void copyToClipboard(jsonPreview());
        break;
    }
  }

  return (
    <div class="flex h-full flex-col" data-testid="canvas-export-panel">
      {/* Tab buttons */}
      <div class="flex border-b border-white/10">
        <For each={TABS}>
          {(tab) => (
            <button
              type="button"
              class={`flex-1 px-2 py-1.5 text-xs font-medium transition-colors ${
                activeTab() === tab.id
                  ? "border-b-2 border-blue-500 text-blue-400"
                  : "text-gray-400 hover:text-gray-200"
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
        <span class="text-xs text-green-400">{copyFeedback()}</span>
        <button
          type="button"
          class="rounded px-2 py-0.5 text-xs text-gray-300 transition-colors hover:bg-white/10"
          onClick={handleCopy}
          data-testid="copy-button"
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
              <p class="text-xs text-gray-500">
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
            class="whitespace-pre font-mono text-xs leading-tight text-gray-300"
            data-testid="ascii-preview"
          >
            {asciiPreview() || "(empty canvas)"}
          </pre>
        </Show>

        <Show when={activeTab() === "json"}>
          <pre
            class="whitespace-pre-wrap font-mono text-xs leading-tight text-gray-300"
            data-testid="json-preview"
          >
            <code>{jsonPreview() || "{}"}</code>
          </pre>
        </Show>
      </div>
    </div>
  );
}
