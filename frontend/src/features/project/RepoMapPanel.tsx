import { createResource, createSignal, For, Show } from "solid-js";
import { api } from "~/api/client";
import type { RepoMap } from "~/api/types";

interface RepoMapPanelProps {
  projectId: string;
  onStatusUpdate?: (status: string) => void;
}

function formatNumber(n: number): string {
  if (n >= 1000) {
    return `${(n / 1000).toFixed(1)}k`;
  }
  return n.toString();
}

export default function RepoMapPanel(props: RepoMapPanelProps) {
  const [repoMap, { refetch }] = createResource<RepoMap | null>(
    () => props.projectId,
    async (id) => {
      try {
        return await api.repomap.get(id);
      } catch {
        return null;
      }
    },
  );

  const [expanded, setExpanded] = createSignal(false);
  const [generating, setGenerating] = createSignal(false);
  const [error, setError] = createSignal("");

  const handleGenerate = async () => {
    setGenerating(true);
    setError("");
    try {
      await api.repomap.generate(props.projectId);
      props.onStatusUpdate?.("generating");
      // Refetch after a short delay to pick up the result
      setTimeout(() => refetch(), 2000);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to generate repo map");
    } finally {
      setGenerating(false);
    }
  };

  return (
    <div class="rounded-lg border border-gray-200 bg-white p-4">
      <div class="mb-3 flex items-center justify-between">
        <h3 class="text-lg font-semibold">Repo Map</h3>
        <button
          class="rounded bg-teal-600 px-3 py-1.5 text-sm text-white hover:bg-teal-700 disabled:opacity-50"
          onClick={handleGenerate}
          disabled={generating()}
        >
          {generating() ? "Generating..." : repoMap() ? "Regenerate" : "Generate"}
        </button>
      </div>

      <Show when={error()}>
        <div class="mb-3 rounded bg-red-50 p-2 text-sm text-red-600">{error()}</div>
      </Show>

      <Show when={!repoMap.loading} fallback={<p class="text-sm text-gray-400">Loading...</p>}>
        <Show
          when={repoMap()}
          fallback={
            <p class="text-sm text-gray-500">
              No repo map generated yet. Click "Generate" to create one.
            </p>
          }
        >
          {(rm) => (
            <>
              {/* Stats */}
              <div class="mb-3 grid grid-cols-3 gap-3">
                <div class="rounded border border-gray-100 bg-gray-50 p-2 text-center">
                  <div class="text-lg font-semibold text-gray-800">
                    {formatNumber(rm().file_count)}
                  </div>
                  <div class="text-xs text-gray-500">Files</div>
                </div>
                <div class="rounded border border-gray-100 bg-gray-50 p-2 text-center">
                  <div class="text-lg font-semibold text-gray-800">
                    {formatNumber(rm().symbol_count)}
                  </div>
                  <div class="text-xs text-gray-500">Symbols</div>
                </div>
                <div class="rounded border border-gray-100 bg-gray-50 p-2 text-center">
                  <div class="text-lg font-semibold text-gray-800">
                    {formatNumber(rm().token_count)}
                  </div>
                  <div class="text-xs text-gray-500">Tokens</div>
                </div>
              </div>

              {/* Languages */}
              <Show when={rm().languages.length > 0}>
                <div class="mb-3">
                  <span class="mr-2 text-xs text-gray-500">Languages:</span>
                  <div class="inline-flex flex-wrap gap-1">
                    <For each={rm().languages}>
                      {(lang) => (
                        <span class="rounded bg-teal-50 px-2 py-0.5 text-xs text-teal-700">
                          {lang}
                        </span>
                      )}
                    </For>
                  </div>
                </div>
              </Show>

              {/* Version and timestamp */}
              <div class="mb-3 text-xs text-gray-400">
                Version {rm().version} — updated {new Date(rm().updated_at).toLocaleString()}
              </div>

              {/* Collapsible map text */}
              <div>
                <button
                  class="flex items-center gap-1 text-sm text-gray-600 hover:text-gray-800"
                  onClick={() => setExpanded((v) => !v)}
                >
                  <span class="font-mono text-xs">{expanded() ? "v" : ">"}</span>
                  {expanded() ? "Hide map" : "Show map"}
                </button>
                <Show when={expanded()}>
                  <pre class="mt-2 max-h-96 overflow-auto rounded border border-gray-200 bg-gray-50 p-3 text-xs leading-relaxed text-gray-700">
                    {rm().map_text}
                  </pre>
                </Show>
              </div>
            </>
          )}
        </Show>
      </Show>
    </div>
  );
}

export { RepoMapPanel };
