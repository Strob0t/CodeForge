import { createResource, createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type { RetrievalIndexStatus, RetrievalSearchHit, SubAgentSearchResult } from "~/api/types";

interface RetrievalPanelProps {
  projectId: string;
  onStatusUpdate?: (status: string) => void;
}

function formatNumber(n: number): string {
  if (n >= 1000) {
    return `${(n / 1000).toFixed(1)}k`;
  }
  return n.toString();
}

export default function RetrievalPanel(props: RetrievalPanelProps) {
  const [indexStatus, { refetch }] = createResource<RetrievalIndexStatus | null>(
    () => props.projectId,
    async (id) => {
      try {
        return await api.retrieval.indexStatus(id);
      } catch {
        return null;
      }
    },
  );

  const [expanded, setExpanded] = createSignal<Record<number, boolean>>({});
  const [building, setBuilding] = createSignal(false);
  const [searching, setSearching] = createSignal(false);
  const [error, setError] = createSignal("");
  const [query, setQuery] = createSignal("");
  const [searchResults, setSearchResults] = createSignal<RetrievalSearchHit[]>([]);
  const [useAgent, setUseAgent] = createSignal(false);
  const [expandedQueries, setExpandedQueries] = createSignal<string[]>([]);
  const [totalCandidates, setTotalCandidates] = createSignal(0);

  const handleBuildIndex = async () => {
    setBuilding(true);
    setError("");
    try {
      await api.retrieval.buildIndex(props.projectId);
      props.onStatusUpdate?.("building");
      setTimeout(() => refetch(), 2000);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to build index");
    } finally {
      setBuilding(false);
    }
  };

  const handleSearch = async (e: Event) => {
    e.preventDefault();
    const q = query().trim();
    if (!q) return;
    setSearching(true);
    setError("");
    setSearchResults([]);
    setExpandedQueries([]);
    setTotalCandidates(0);
    try {
      if (useAgent()) {
        const result: SubAgentSearchResult = await api.retrieval.agentSearch(props.projectId, {
          query: q,
        });
        if (result.error) {
          setError(result.error);
        } else {
          setSearchResults(result.results);
          setExpandedQueries(result.expanded_queries);
          setTotalCandidates(result.total_candidates);
        }
      } else {
        const result = await api.retrieval.search(props.projectId, { query: q });
        if (result.error) {
          setError(result.error);
        } else {
          setSearchResults(result.results);
        }
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "Search failed");
    } finally {
      setSearching(false);
    }
  };

  const toggleExpanded = (idx: number) => {
    setExpanded((prev) => ({ ...prev, [idx]: !prev[idx] }));
  };

  const statusColor = (status: string): string => {
    switch (status) {
      case "ready":
        return "bg-green-100 text-green-700";
      case "building":
        return "bg-yellow-100 text-yellow-700";
      case "error":
        return "bg-red-100 text-red-700";
      default:
        return "bg-gray-100 text-gray-700";
    }
  };

  return (
    <div class="rounded-lg border border-gray-200 bg-white p-4">
      <div class="mb-3 flex items-center justify-between">
        <h3 class="text-lg font-semibold">Hybrid Retrieval</h3>
        <button
          class="rounded bg-indigo-600 px-3 py-1.5 text-sm text-white hover:bg-indigo-700 disabled:opacity-50"
          onClick={handleBuildIndex}
          disabled={building()}
        >
          {building() ? "Building..." : indexStatus() ? "Rebuild Index" : "Build Index"}
        </button>
      </div>

      <Show when={error()}>
        <div class="mb-3 rounded bg-red-50 p-2 text-sm text-red-600">{error()}</div>
      </Show>

      <Show when={!indexStatus.loading} fallback={<p class="text-sm text-gray-400">Loading...</p>}>
        <Show
          when={indexStatus()}
          fallback={
            <p class="text-sm text-gray-500">
              No retrieval index built yet. Click "Build Index" to create one.
            </p>
          }
        >
          {(status) => (
            <>
              {/* Stats */}
              <div class="mb-3 grid grid-cols-4 gap-3">
                <div class="rounded border border-gray-100 bg-gray-50 p-2 text-center">
                  <div class="text-lg font-semibold text-gray-800">
                    {formatNumber(status().file_count)}
                  </div>
                  <div class="text-xs text-gray-500">Files</div>
                </div>
                <div class="rounded border border-gray-100 bg-gray-50 p-2 text-center">
                  <div class="text-lg font-semibold text-gray-800">
                    {formatNumber(status().chunk_count)}
                  </div>
                  <div class="text-xs text-gray-500">Chunks</div>
                </div>
                <div class="rounded border border-gray-100 bg-gray-50 p-2 text-center">
                  <div class="text-xs font-medium text-gray-800">
                    {status().embedding_model || "—"}
                  </div>
                  <div class="text-xs text-gray-500">Model</div>
                </div>
                <div class="rounded border border-gray-100 bg-gray-50 p-2 text-center">
                  <span
                    class={`inline-block rounded px-2 py-0.5 text-xs font-medium ${statusColor(status().status)}`}
                  >
                    {status().status}
                  </span>
                  <div class="text-xs text-gray-500">Status</div>
                </div>
              </div>

              <Show when={status().error}>
                <div class="mb-3 rounded bg-red-50 p-2 text-sm text-red-600">
                  Index error: {status().error}
                </div>
              </Show>

              {/* Search */}
              <form class="mb-3 flex gap-2" onSubmit={handleSearch}>
                <input
                  type="text"
                  class="flex-1 rounded border border-gray-300 px-3 py-1.5 text-sm focus:border-indigo-500 focus:outline-none focus:ring-1 focus:ring-indigo-500"
                  placeholder={
                    useAgent() ? "Describe what you're looking for..." : "Search code..."
                  }
                  value={query()}
                  onInput={(e) => setQuery(e.currentTarget.value)}
                />
                <button
                  type="button"
                  class={`rounded px-3 py-1.5 text-sm font-medium ${
                    useAgent()
                      ? "bg-purple-100 text-purple-700 hover:bg-purple-200"
                      : "bg-gray-100 text-gray-600 hover:bg-gray-200"
                  }`}
                  onClick={() => {
                    setUseAgent((v) => !v);
                    setExpandedQueries([]);
                    setTotalCandidates(0);
                  }}
                  title={
                    useAgent()
                      ? "Agent search: LLM expands queries and re-ranks"
                      : "Standard hybrid search"
                  }
                >
                  {useAgent() ? "Agent" : "Standard"}
                </button>
                <button
                  type="submit"
                  class="rounded bg-indigo-600 px-3 py-1.5 text-sm text-white hover:bg-indigo-700 disabled:opacity-50"
                  disabled={searching() || status().status !== "ready" || !query().trim()}
                >
                  {searching() ? "Searching..." : "Search"}
                </button>
              </form>

              {/* Expanded queries from agent search */}
              <Show when={expandedQueries().length > 0}>
                <div class="mb-3">
                  <div class="mb-1 flex items-center gap-2 text-xs text-gray-500">
                    <span>Expanded queries</span>
                    <Show when={totalCandidates() > 0}>
                      <span class="rounded bg-gray-100 px-1.5 py-0.5 text-xs text-gray-500">
                        {totalCandidates()} candidates
                      </span>
                    </Show>
                  </div>
                  <div class="flex flex-wrap gap-1">
                    <For each={expandedQueries()}>
                      {(eq) => (
                        <span class="rounded bg-purple-50 px-2 py-0.5 text-xs text-purple-700">
                          {eq}
                        </span>
                      )}
                    </For>
                  </div>
                </div>
              </Show>
            </>
          )}
        </Show>
      </Show>

      {/* Search Results */}
      <Show when={searchResults().length > 0}>
        <div class="mt-3">
          <h4 class="mb-2 text-sm font-medium text-gray-500">Results ({searchResults().length})</h4>
          <div class="space-y-2">
            <For each={searchResults()}>
              {(hit, idx) => (
                <div class="rounded border border-gray-200 bg-gray-50 p-3">
                  <div class="flex items-center justify-between">
                    <button
                      class="flex items-center gap-2 text-sm font-medium text-gray-800 hover:text-indigo-700"
                      onClick={() => toggleExpanded(idx())}
                    >
                      <span class="font-mono text-xs">{expanded()[idx()] ? "v" : ">"}</span>
                      <span class="font-mono">
                        {hit.filepath}:{hit.start_line}-{hit.end_line}
                      </span>
                    </button>
                    <div class="flex items-center gap-2">
                      <Show when={hit.symbol_name}>
                        <span class="rounded bg-indigo-50 px-2 py-0.5 text-xs text-indigo-700">
                          {hit.symbol_name}
                        </span>
                      </Show>
                      <span class="rounded bg-gray-200 px-2 py-0.5 text-xs text-gray-600">
                        {hit.language}
                      </span>
                      <span class="text-xs text-gray-400" title="Relevance score">
                        {hit.score.toFixed(3)}
                      </span>
                    </div>
                  </div>
                  <Show when={expanded()[idx()]}>
                    <pre class="mt-2 max-h-64 overflow-auto rounded border border-gray-200 bg-white p-3 text-xs leading-relaxed text-gray-700">
                      {hit.content}
                    </pre>
                  </Show>
                </div>
              )}
            </For>
          </div>
        </div>
      </Show>
    </div>
  );
}

export { RetrievalPanel };
