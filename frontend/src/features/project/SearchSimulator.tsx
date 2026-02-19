import { createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type { GraphSearchHit, RetrievalSearchHit } from "~/api/types";
import { useI18n } from "~/i18n";

interface SearchSimulatorProps {
  projectId: string;
}

/** Rough token estimate: ~4 chars per token */
function estimateTokens(text: string): number {
  return Math.ceil(text.length / 4);
}

export default function SearchSimulator(props: SearchSimulatorProps) {
  const { t, fmt } = useI18n();

  // Search parameters
  const [query, setQuery] = createSignal("");
  const [topK, setTopK] = createSignal(10);
  const [bm25Weight, setBm25Weight] = createSignal(0.5);
  const [semanticWeight, setSemanticWeight] = createSignal(0.5);
  const [tokenBudget, setTokenBudget] = createSignal(4000);
  const [useAgent, setUseAgent] = createSignal(false);
  const [includeGraph, setIncludeGraph] = createSignal(false);

  // Results
  const [searching, setSearching] = createSignal(false);
  const [hybridResults, setHybridResults] = createSignal<RetrievalSearchHit[]>([]);
  const [graphResults, setGraphResults] = createSignal<GraphSearchHit[]>([]);
  const [expandedQueries, setExpandedQueries] = createSignal<string[]>([]);
  const [error, setError] = createSignal("");
  const [expanded, setExpanded] = createSignal<Record<number, boolean>>({});

  const totalTokens = () =>
    hybridResults().reduce((sum, hit) => sum + estimateTokens(hit.content), 0);

  const resultsWithBudget = () => {
    let running = 0;
    return hybridResults().map((hit) => {
      const tokens = estimateTokens(hit.content);
      running += tokens;
      return { hit, tokens, cumulative: running, withinBudget: running <= tokenBudget() };
    });
  };

  const handleSearch = async (e: Event) => {
    e.preventDefault();
    const q = query().trim();
    if (!q) return;

    setSearching(true);
    setError("");
    setHybridResults([]);
    setGraphResults([]);
    setExpandedQueries([]);
    setExpanded({});

    try {
      if (useAgent()) {
        const result = await api.retrieval.agentSearch(props.projectId, {
          query: q,
          top_k: topK(),
        });
        if (result.error) {
          setError(result.error);
        } else {
          setHybridResults(result.results);
          setExpandedQueries(result.expanded_queries);
        }
      } else {
        const result = await api.retrieval.search(props.projectId, {
          query: q,
          top_k: topK(),
          bm25_weight: bm25Weight(),
          semantic_weight: semanticWeight(),
        });
        if (result.error) {
          setError(result.error);
        } else {
          setHybridResults(result.results);
        }
      }

      if (includeGraph()) {
        const keywords = q.split(/\s+/).filter((w) => w.length > 2);
        if (keywords.length > 0) {
          try {
            const graphResult = await api.graph.search(props.projectId, {
              seed_symbols: keywords.slice(0, 5),
              top_k: 5,
            });
            if (!graphResult.error) {
              setGraphResults(graphResult.results);
            }
          } catch {
            // Graph search is optional — ignore errors
          }
        }
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : t("simulator.error"));
    } finally {
      setSearching(false);
    }
  };

  const toggleExpanded = (idx: number) => {
    setExpanded((prev) => ({ ...prev, [idx]: !prev[idx] }));
  };

  return (
    <div class="rounded-lg border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-gray-800">
      <h3 class="mb-1 text-lg font-semibold">{t("simulator.title")}</h3>
      <p class="mb-4 text-xs text-gray-500 dark:text-gray-400">{t("simulator.description")}</p>

      {/* Search Form */}
      <form class="mb-4 space-y-3" onSubmit={handleSearch}>
        {/* Query input */}
        <div>
          <label
            for="sim-query"
            class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400"
          >
            {t("simulator.query")}
          </label>
          <input
            id="sim-query"
            type="text"
            class="w-full rounded border border-gray-300 px-3 py-1.5 text-sm focus:border-indigo-500 focus:outline-none focus:ring-1 focus:ring-indigo-500 dark:border-gray-600 dark:bg-gray-700 dark:text-gray-100"
            placeholder={t("simulator.queryPlaceholder")}
            value={query()}
            onInput={(e) => setQuery(e.currentTarget.value)}
            aria-label={t("simulator.query")}
          />
        </div>

        {/* Parameters row */}
        <div class="grid grid-cols-2 gap-3 lg:grid-cols-4">
          {/* Top K */}
          <div>
            <label
              for="sim-topk"
              class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400"
            >
              {t("simulator.topK")}
            </label>
            <input
              id="sim-topk"
              type="number"
              min="1"
              max="50"
              class="w-full rounded border border-gray-300 px-2 py-1 text-sm dark:border-gray-600 dark:bg-gray-700 dark:text-gray-100"
              value={topK()}
              onInput={(e) => setTopK(parseInt(e.currentTarget.value) || 10)}
            />
          </div>

          {/* Token Budget */}
          <div>
            <label
              for="sim-budget"
              class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400"
            >
              {t("simulator.tokenBudget")}
            </label>
            <input
              id="sim-budget"
              type="number"
              min="100"
              max="128000"
              step="500"
              class="w-full rounded border border-gray-300 px-2 py-1 text-sm dark:border-gray-600 dark:bg-gray-700 dark:text-gray-100"
              value={tokenBudget()}
              onInput={(e) => setTokenBudget(parseInt(e.currentTarget.value) || 4000)}
            />
          </div>

          {/* BM25 Weight */}
          <div>
            <label
              for="sim-bm25"
              class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400"
            >
              {t("simulator.bm25Weight")}: {bm25Weight().toFixed(2)}
            </label>
            <input
              id="sim-bm25"
              type="range"
              min="0"
              max="1"
              step="0.05"
              class="w-full"
              value={bm25Weight()}
              onInput={(e) => {
                const v = parseFloat(e.currentTarget.value);
                setBm25Weight(v);
                setSemanticWeight(+(1 - v).toFixed(2));
              }}
              disabled={useAgent()}
            />
          </div>

          {/* Semantic Weight */}
          <div>
            <label
              for="sim-semantic"
              class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400"
            >
              {t("simulator.semanticWeight")}: {semanticWeight().toFixed(2)}
            </label>
            <input
              id="sim-semantic"
              type="range"
              min="0"
              max="1"
              step="0.05"
              class="w-full"
              value={semanticWeight()}
              onInput={(e) => {
                const v = parseFloat(e.currentTarget.value);
                setSemanticWeight(v);
                setBm25Weight(+(1 - v).toFixed(2));
              }}
              disabled={useAgent()}
            />
          </div>
        </div>

        {/* Toggles row */}
        <div class="flex items-center gap-4">
          <label class="flex items-center gap-1.5 text-sm text-gray-700 dark:text-gray-300">
            <input
              type="checkbox"
              checked={useAgent()}
              onChange={(e) => setUseAgent(e.currentTarget.checked)}
            />
            {t("simulator.agentMode")}
          </label>
          <label class="flex items-center gap-1.5 text-sm text-gray-700 dark:text-gray-300">
            <input
              type="checkbox"
              checked={includeGraph()}
              onChange={(e) => setIncludeGraph(e.currentTarget.checked)}
            />
            {t("simulator.includeGraph")}
          </label>
          <div class="flex-1" />
          <button
            type="submit"
            class="rounded bg-indigo-600 px-4 py-1.5 text-sm text-white hover:bg-indigo-700 disabled:opacity-50"
            disabled={searching() || !query().trim()}
          >
            {searching() ? t("simulator.searching") : t("simulator.search")}
          </button>
        </div>
      </form>

      {/* Error */}
      <Show when={error()}>
        <div class="mb-3 rounded bg-red-50 p-2 text-sm text-red-600 dark:bg-red-900/30 dark:text-red-400">
          {error()}
        </div>
      </Show>

      {/* Expanded queries (agent mode) */}
      <Show when={expandedQueries().length > 0}>
        <div class="mb-3">
          <span class="mr-2 text-xs text-gray-500 dark:text-gray-400">
            {t("simulator.expandedQueries")}:
          </span>
          <div class="inline-flex flex-wrap gap-1">
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

      {/* Token budget summary */}
      <Show when={hybridResults().length > 0}>
        <div class="mb-3 rounded border border-gray-100 bg-gray-50 p-3 dark:border-gray-600 dark:bg-gray-700">
          <div class="mb-2 flex items-center justify-between text-xs">
            <span class="font-medium text-gray-700 dark:text-gray-300">
              {t("simulator.budgetUsage")}
            </span>
            <span class="text-gray-500 dark:text-gray-400">
              {fmt.compact(totalTokens())} / {fmt.compact(tokenBudget())} {t("simulator.tokens")}
            </span>
          </div>
          <div class="h-2 overflow-hidden rounded-full bg-gray-200 dark:bg-gray-600">
            <div
              class={`h-full rounded-full transition-all ${
                totalTokens() > tokenBudget() ? "bg-red-500" : "bg-green-500"
              }`}
              style={{ width: `${Math.min(100, (totalTokens() / tokenBudget()) * 100)}%` }}
            />
          </div>
          <div class="mt-1 flex justify-between text-xs text-gray-400">
            <span>
              {resultsWithBudget().filter((r) => r.withinBudget).length} / {hybridResults().length}{" "}
              {t("simulator.fitsInBudget")}
            </span>
            <span>
              {totalTokens() > tokenBudget()
                ? t("simulator.overBudget")
                : t("simulator.withinBudget")}
            </span>
          </div>
        </div>
      </Show>

      {/* Results */}
      <Show when={hybridResults().length > 0}>
        <div class="space-y-2">
          <For each={resultsWithBudget()}>
            {(item, idx) => (
              <div
                class={`rounded border p-3 ${
                  item.withinBudget
                    ? "border-green-200 bg-green-50/50 dark:border-green-800 dark:bg-green-900/10"
                    : "border-red-200 bg-red-50/50 dark:border-red-800 dark:bg-red-900/10"
                }`}
              >
                <div class="flex items-center justify-between">
                  <button
                    type="button"
                    class="flex items-center gap-2 text-sm font-medium text-gray-800 hover:text-indigo-700 dark:text-gray-200 dark:hover:text-indigo-400"
                    onClick={() => toggleExpanded(idx())}
                    aria-expanded={!!expanded()[idx()]}
                    aria-label={t("simulator.toggleResult", { path: item.hit.filepath })}
                  >
                    <span class="font-mono text-xs text-gray-400" aria-hidden="true">
                      {expanded()[idx()] ? "\u25BC" : "\u25B6"}
                    </span>
                    <span class="font-mono text-xs">
                      {item.hit.filepath}:{item.hit.start_line}-{item.hit.end_line}
                    </span>
                  </button>
                  <div class="flex items-center gap-2 text-xs">
                    <Show when={item.hit.symbol_name}>
                      <span class="rounded bg-indigo-50 px-1.5 py-0.5 text-indigo-700 dark:bg-indigo-900/30 dark:text-indigo-400">
                        {item.hit.symbol_name}
                      </span>
                    </Show>
                    <span class="rounded bg-gray-200 px-1.5 py-0.5 text-gray-600 dark:bg-gray-600 dark:text-gray-300">
                      {item.hit.language}
                    </span>
                    <span title={t("simulator.bm25Rank")} class="text-gray-400">
                      B:{item.hit.bm25_rank}
                    </span>
                    <span title={t("simulator.semanticRank")} class="text-gray-400">
                      S:{item.hit.semantic_rank}
                    </span>
                    <span
                      class="font-medium text-gray-600 dark:text-gray-300"
                      title={t("simulator.combinedScore")}
                    >
                      {fmt.score(item.hit.score)}
                    </span>
                    <span
                      class={`rounded px-1.5 py-0.5 ${
                        item.withinBudget
                          ? "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400"
                          : "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400"
                      }`}
                    >
                      {item.tokens}t
                    </span>
                  </div>
                </div>
                <Show when={expanded()[idx()]}>
                  <pre class="mt-2 max-h-48 overflow-auto rounded border border-gray-200 bg-white p-2 text-xs leading-relaxed text-gray-700 dark:border-gray-600 dark:bg-gray-800 dark:text-gray-300">
                    {item.hit.content}
                  </pre>
                </Show>
              </div>
            )}
          </For>
        </div>
      </Show>

      {/* Graph results */}
      <Show when={graphResults().length > 0}>
        <div class="mt-4 border-t border-gray-200 pt-4 dark:border-gray-700">
          <h4 class="mb-2 text-sm font-medium text-gray-600 dark:text-gray-400">
            {t("simulator.graphResults")} ({graphResults().length})
          </h4>
          <div class="space-y-1">
            <For each={graphResults()}>
              {(hit) => (
                <div class="flex items-center gap-2 rounded bg-violet-50 p-2 text-xs dark:bg-violet-900/10">
                  <span class="rounded bg-violet-100 px-1.5 py-0.5 font-medium text-violet-700 dark:bg-violet-900/30 dark:text-violet-400">
                    {hit.kind}
                  </span>
                  <span class="font-mono text-gray-700 dark:text-gray-300">{hit.symbol_name}</span>
                  <span class="text-gray-400">
                    {hit.filepath}:{hit.start_line}
                  </span>
                  <Show when={hit.edge_path.length > 0}>
                    <span class="text-gray-400" title={t("simulator.graphPath")}>
                      {hit.edge_path.join(" \u2192 ")}
                    </span>
                  </Show>
                  <span class="ml-auto text-gray-400">{fmt.score(hit.score)}</span>
                </div>
              )}
            </For>
          </div>
        </div>
      </Show>
    </div>
  );
}

export { SearchSimulator };
