import { createResource, createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type {
  GraphStatus,
  RetrievalIndexStatus,
  RetrievalSearchHit,
  SubAgentSearchResult,
} from "~/api/types";
import { useI18n } from "~/i18n";

interface RetrievalPanelProps {
  projectId: string;
  onStatusUpdate?: (status: string) => void;
}

export default function RetrievalPanel(props: RetrievalPanelProps) {
  const { t, fmt } = useI18n();
  const [indexStatus, { refetch }] = createResource(
    () => props.projectId || undefined,
    async (id: string): Promise<RetrievalIndexStatus | null> => {
      try {
        return await api.retrieval.indexStatus(id);
      } catch {
        return null;
      }
    },
  );

  const [graphStatus, { refetch: refetchGraph }] = createResource(
    () => props.projectId || undefined,
    async (id: string): Promise<GraphStatus | null> => {
      try {
        return await api.graph.status(id);
      } catch {
        return null;
      }
    },
  );

  const [expanded, setExpanded] = createSignal<Record<number, boolean>>({});
  const [building, setBuilding] = createSignal(false);
  const [buildingGraph, setBuildingGraph] = createSignal(false);
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
      setError(e instanceof Error ? e.message : t("retrieval.toast.buildFailed"));
    } finally {
      setBuilding(false);
    }
  };

  const handleBuildGraph = async () => {
    setBuildingGraph(true);
    setError("");
    try {
      await api.graph.build(props.projectId);
      props.onStatusUpdate?.("building");
      setTimeout(() => refetchGraph(), 2000);
    } catch (e) {
      setError(e instanceof Error ? e.message : t("retrieval.toast.graphFailed"));
    } finally {
      setBuildingGraph(false);
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
      setError(err instanceof Error ? err.message : t("retrieval.toast.searchFailed"));
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
        return "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400";
      case "building":
        return "bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400";
      case "error":
        return "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400";
      default:
        return "bg-gray-100 text-gray-700 dark:bg-gray-700 dark:text-gray-300";
    }
  };

  return (
    <div class="rounded-lg border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-gray-800">
      <div class="mb-3 flex items-center justify-between">
        <h3 class="text-lg font-semibold">{t("retrieval.title")}</h3>
        <button
          class="rounded bg-indigo-600 px-3 py-1.5 text-sm text-white hover:bg-indigo-700 disabled:opacity-50"
          onClick={handleBuildIndex}
          disabled={building()}
        >
          {building()
            ? t("retrieval.building")
            : indexStatus()
              ? t("retrieval.rebuildIndex")
              : t("retrieval.buildIndex")}
        </button>
      </div>

      <Show when={error()}>
        <div class="mb-3 rounded bg-red-50 p-2 text-sm text-red-600 dark:bg-red-900/30 dark:text-red-400">
          {error()}
        </div>
      </Show>

      <Show
        when={!indexStatus.loading}
        fallback={<p class="text-sm text-gray-400 dark:text-gray-500">{t("common.loading")}</p>}
      >
        <Show
          when={indexStatus()}
          fallback={<p class="text-sm text-gray-500 dark:text-gray-400">{t("retrieval.empty")}</p>}
        >
          {(status) => (
            <>
              {/* Stats */}
              <div class="mb-3 grid grid-cols-4 gap-3">
                <div class="rounded border border-gray-100 bg-gray-50 p-2 text-center dark:border-gray-600 dark:bg-gray-700">
                  <div class="text-lg font-semibold text-gray-800 dark:text-gray-200">
                    {fmt.compact(status().file_count)}
                  </div>
                  <div class="text-xs text-gray-500 dark:text-gray-400">{t("retrieval.files")}</div>
                </div>
                <div class="rounded border border-gray-100 bg-gray-50 p-2 text-center dark:border-gray-600 dark:bg-gray-700">
                  <div class="text-lg font-semibold text-gray-800 dark:text-gray-200">
                    {fmt.compact(status().chunk_count)}
                  </div>
                  <div class="text-xs text-gray-500 dark:text-gray-400">
                    {t("retrieval.chunks")}
                  </div>
                </div>
                <div class="rounded border border-gray-100 bg-gray-50 p-2 text-center dark:border-gray-600 dark:bg-gray-700">
                  <div class="text-xs font-medium text-gray-800 dark:text-gray-200">
                    {status().embedding_model || "\u2014"}
                  </div>
                  <div class="text-xs text-gray-500 dark:text-gray-400">{t("retrieval.model")}</div>
                </div>
                <div class="rounded border border-gray-100 bg-gray-50 p-2 text-center dark:border-gray-600 dark:bg-gray-700">
                  <span
                    class={`inline-block rounded px-2 py-0.5 text-xs font-medium ${statusColor(status().status)}`}
                  >
                    {status().status}
                  </span>
                  <div class="text-xs text-gray-500 dark:text-gray-400">{t("common.status")}</div>
                </div>
              </div>

              <Show when={status().error}>
                <div class="mb-3 rounded bg-red-50 p-2 text-sm text-red-600 dark:bg-red-900/30 dark:text-red-400">
                  {t("retrieval.indexError")} {status().error}
                </div>
              </Show>

              {/* Search */}
              <form class="mb-3 flex gap-2" onSubmit={handleSearch}>
                <input
                  type="text"
                  class="flex-1 rounded border border-gray-300 px-3 py-1.5 text-sm focus:border-indigo-500 focus:outline-none focus:ring-1 focus:ring-indigo-500 dark:border-gray-600 dark:bg-gray-700 dark:text-gray-100"
                  placeholder={
                    useAgent()
                      ? t("retrieval.agentSearchPlaceholder")
                      : t("retrieval.searchPlaceholder")
                  }
                  value={query()}
                  onInput={(e) => setQuery(e.currentTarget.value)}
                  aria-label={t("retrieval.searchAria")}
                />
                <button
                  type="button"
                  class={`rounded px-3 py-1.5 text-sm font-medium ${
                    useAgent()
                      ? "bg-purple-100 text-purple-700 hover:bg-purple-200 dark:bg-purple-900/30 dark:text-purple-400 dark:hover:bg-purple-900/50"
                      : "bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-gray-700 dark:text-gray-400 dark:hover:bg-gray-600"
                  }`}
                  onClick={() => {
                    setUseAgent((v) => !v);
                    setExpandedQueries([]);
                    setTotalCandidates(0);
                  }}
                  title={useAgent() ? t("retrieval.agentTitle") : t("retrieval.standardTitle")}
                  aria-label={
                    useAgent() ? t("retrieval.agentSwitchAria") : t("retrieval.standardSwitchAria")
                  }
                  aria-pressed={useAgent()}
                >
                  {useAgent() ? t("retrieval.agentToggle") : t("retrieval.standardToggle")}
                </button>
                <button
                  type="submit"
                  class="rounded bg-indigo-600 px-3 py-1.5 text-sm text-white hover:bg-indigo-700 disabled:opacity-50"
                  disabled={searching() || status().status !== "ready" || !query().trim()}
                >
                  {searching() ? t("retrieval.searching") : t("retrieval.search")}
                </button>
              </form>

              {/* Expanded queries from agent search */}
              <Show when={expandedQueries().length > 0}>
                <div class="mb-3">
                  <div class="mb-1 flex items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
                    <span>{t("retrieval.expandedQueries")}</span>
                    <Show when={totalCandidates() > 0}>
                      <span class="rounded bg-gray-100 px-1.5 py-0.5 text-xs text-gray-500 dark:bg-gray-700 dark:text-gray-400">
                        {t("retrieval.candidates", { n: totalCandidates() })}
                      </span>
                    </Show>
                  </div>
                  <div class="flex flex-wrap gap-1">
                    <For each={expandedQueries()}>
                      {(eq) => (
                        <span class="rounded bg-purple-50 px-2 py-0.5 text-xs text-purple-700 dark:bg-purple-900/30 dark:text-purple-400">
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
          <h4 class="mb-2 text-sm font-medium text-gray-500 dark:text-gray-400">
            {t("retrieval.results", { n: searchResults().length })}
          </h4>
          <div class="space-y-2">
            <For each={searchResults()}>
              {(hit, idx) => (
                <div class="rounded border border-gray-200 bg-gray-50 p-3 dark:border-gray-600 dark:bg-gray-700">
                  <div class="flex items-center justify-between">
                    <button
                      type="button"
                      class="flex items-center gap-2 text-sm font-medium text-gray-800 hover:text-indigo-700 dark:text-gray-200 dark:hover:text-indigo-400"
                      onClick={() => toggleExpanded(idx())}
                      aria-expanded={!!expanded()[idx()]}
                      aria-label={`${expanded()[idx()] ? "Collapse" : "Expand"} result: ${hit.filepath}`}
                    >
                      <span class="font-mono text-xs" aria-hidden="true">
                        {expanded()[idx()] ? "v" : ">"}
                      </span>
                      <span class="font-mono">
                        {hit.filepath}:{hit.start_line}-{hit.end_line}
                      </span>
                    </button>
                    <div class="flex items-center gap-2">
                      <Show when={hit.symbol_name}>
                        <span class="rounded bg-indigo-50 px-2 py-0.5 text-xs text-indigo-700 dark:bg-indigo-900/30 dark:text-indigo-400">
                          {hit.symbol_name}
                        </span>
                      </Show>
                      <span class="rounded bg-gray-200 px-2 py-0.5 text-xs text-gray-600 dark:bg-gray-600 dark:text-gray-300">
                        {hit.language}
                      </span>
                      <span
                        class="text-xs text-gray-400 dark:text-gray-500"
                        title="Relevance score"
                      >
                        {fmt.score(hit.score)}
                      </span>
                    </div>
                  </div>
                  <Show when={expanded()[idx()]}>
                    <pre class="mt-2 max-h-64 overflow-auto rounded border border-gray-200 bg-white p-3 text-xs leading-relaxed text-gray-700 dark:border-gray-600 dark:bg-gray-800 dark:text-gray-300">
                      {hit.content}
                    </pre>
                  </Show>
                </div>
              )}
            </For>
          </div>
        </div>
      </Show>

      {/* Graph Status Section */}
      <div class="mt-4 border-t border-gray-200 pt-4 dark:border-gray-700">
        <div class="mb-3 flex items-center justify-between">
          <h3 class="text-lg font-semibold">{t("retrieval.graph.title")}</h3>
          <button
            class="rounded bg-violet-600 px-3 py-1.5 text-sm text-white hover:bg-violet-700 disabled:opacity-50"
            onClick={handleBuildGraph}
            disabled={buildingGraph() || graphStatus()?.status === "building"}
          >
            {buildingGraph() || graphStatus()?.status === "building"
              ? t("retrieval.graph.rebuilding")
              : graphStatus()?.status === "ready"
                ? t("retrieval.graph.rebuild")
                : t("retrieval.graph.build")}
          </button>
        </div>

        <Show
          when={!graphStatus.loading}
          fallback={<p class="text-sm text-gray-400 dark:text-gray-500">{t("common.loading")}</p>}
        >
          <Show
            when={graphStatus()}
            fallback={
              <p class="text-sm text-gray-500 dark:text-gray-400">{t("retrieval.graph.empty")}</p>
            }
          >
            {(gs) => (
              <>
                <div class="mb-3 grid grid-cols-3 gap-3">
                  <div class="rounded border border-gray-100 bg-gray-50 p-2 text-center dark:border-gray-600 dark:bg-gray-700">
                    <div class="text-lg font-semibold text-gray-800 dark:text-gray-200">
                      {fmt.compact(gs().node_count)}
                    </div>
                    <div class="text-xs text-gray-500 dark:text-gray-400">
                      {t("retrieval.graph.nodes")}
                    </div>
                  </div>
                  <div class="rounded border border-gray-100 bg-gray-50 p-2 text-center dark:border-gray-600 dark:bg-gray-700">
                    <div class="text-lg font-semibold text-gray-800 dark:text-gray-200">
                      {fmt.compact(gs().edge_count)}
                    </div>
                    <div class="text-xs text-gray-500 dark:text-gray-400">
                      {t("retrieval.graph.edges")}
                    </div>
                  </div>
                  <div class="rounded border border-gray-100 bg-gray-50 p-2 text-center dark:border-gray-600 dark:bg-gray-700">
                    <span
                      class={`inline-block rounded px-2 py-0.5 text-xs font-medium ${statusColor(gs().status)}`}
                    >
                      {gs().status}
                    </span>
                    <div class="text-xs text-gray-500 dark:text-gray-400">{t("common.status")}</div>
                  </div>
                </div>

                <Show when={gs().languages.length > 0}>
                  <div class="mb-3">
                    <span class="mr-2 text-xs text-gray-500 dark:text-gray-400">
                      {t("retrieval.graph.languages")}
                    </span>
                    <div class="inline-flex flex-wrap gap-1">
                      <For each={gs().languages}>
                        {(lang) => (
                          <span class="rounded bg-violet-50 px-2 py-0.5 text-xs text-violet-700 dark:bg-violet-900/30 dark:text-violet-400">
                            {lang}
                          </span>
                        )}
                      </For>
                    </div>
                  </div>
                </Show>

                <Show when={gs().built_at}>
                  <div class="mb-3 text-xs text-gray-400 dark:text-gray-500">
                    {t("retrieval.graph.built", {
                      date: fmt.dateTime(gs().built_at ?? ""),
                    })}
                  </div>
                </Show>

                <Show when={gs().error}>
                  <div class="rounded bg-red-50 p-2 text-sm text-red-600 dark:bg-red-900/30 dark:text-red-400">
                    {t("retrieval.graph.error")} {gs().error}
                  </div>
                </Show>
              </>
            )}
          </Show>
        </Show>
      </div>
    </div>
  );
}

export { RetrievalPanel };
