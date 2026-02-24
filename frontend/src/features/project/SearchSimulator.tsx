import { createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type { GraphSearchHit, RetrievalSearchHit } from "~/api/types";
import { useI18n } from "~/i18n";
import { Alert, Badge, Button, Card, Checkbox, FormField, Input } from "~/ui";

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
    <Card>
      <Card.Header>
        <h3 class="text-lg font-semibold">{t("simulator.title")}</h3>
        <p class="text-xs text-cf-text-tertiary">{t("simulator.description")}</p>
      </Card.Header>

      <Card.Body>
        {/* Search Form */}
        <form class="mb-4 space-y-3" onSubmit={handleSearch}>
          {/* Query input */}
          <FormField id="sim-query" label={t("simulator.query")}>
            <Input
              id="sim-query"
              type="text"
              placeholder={t("simulator.queryPlaceholder")}
              value={query()}
              onInput={(e) => setQuery(e.currentTarget.value)}
              aria-label={t("simulator.query")}
            />
          </FormField>

          {/* Parameters row */}
          <div class="grid grid-cols-2 gap-3 lg:grid-cols-4">
            <FormField id="sim-topk" label={t("simulator.topK")}>
              <Input
                id="sim-topk"
                type="number"
                min="1"
                max="50"
                value={topK()}
                onInput={(e) => setTopK(parseInt(e.currentTarget.value) || 10)}
              />
            </FormField>

            <FormField id="sim-budget" label={t("simulator.tokenBudget")}>
              <Input
                id="sim-budget"
                type="number"
                min="100"
                max="128000"
                step="500"
                value={tokenBudget()}
                onInput={(e) => setTokenBudget(parseInt(e.currentTarget.value) || 4000)}
              />
            </FormField>

            {/* BM25 Weight */}
            <div>
              <label for="sim-bm25" class="mb-1 block text-xs font-medium text-cf-text-secondary">
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
                class="mb-1 block text-xs font-medium text-cf-text-secondary"
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
            <label class="flex items-center gap-1.5 text-sm text-cf-text-secondary">
              <Checkbox checked={useAgent()} onChange={(checked) => setUseAgent(checked)} />
              {t("simulator.agentMode")}
            </label>
            <label class="flex items-center gap-1.5 text-sm text-cf-text-secondary">
              <Checkbox checked={includeGraph()} onChange={(checked) => setIncludeGraph(checked)} />
              {t("simulator.includeGraph")}
            </label>
            <div class="flex-1" />
            <Button
              type="submit"
              variant="primary"
              size="sm"
              disabled={searching() || !query().trim()}
              loading={searching()}
            >
              {searching() ? t("simulator.searching") : t("simulator.search")}
            </Button>
          </div>
        </form>

        {/* Error */}
        <Show when={error()}>
          <div class="mb-3">
            <Alert variant="error">{error()}</Alert>
          </div>
        </Show>

        {/* Expanded queries (agent mode) */}
        <Show when={expandedQueries().length > 0}>
          <div class="mb-3">
            <span class="mr-2 text-xs text-cf-text-tertiary">
              {t("simulator.expandedQueries")}:
            </span>
            <div class="inline-flex flex-wrap gap-1">
              <For each={expandedQueries()}>{(eq) => <Badge variant="info">{eq}</Badge>}</For>
            </div>
          </div>
        </Show>

        {/* Token budget summary */}
        <Show when={hybridResults().length > 0}>
          <div class="mb-3 rounded-cf-sm border border-cf-border-subtle bg-cf-bg-surface-alt p-3">
            <div class="mb-2 flex items-center justify-between text-xs">
              <span class="font-medium text-cf-text-secondary">{t("simulator.budgetUsage")}</span>
              <span class="text-cf-text-tertiary">
                {fmt.compact(totalTokens())} / {fmt.compact(tokenBudget())} {t("simulator.tokens")}
              </span>
            </div>
            <div class="h-2 overflow-hidden rounded-full bg-cf-bg-inset">
              <div
                class={`h-full rounded-full transition-all ${
                  totalTokens() > tokenBudget() ? "bg-cf-danger" : "bg-cf-success"
                }`}
                style={{ width: `${Math.min(100, (totalTokens() / tokenBudget()) * 100)}%` }}
              />
            </div>
            <div class="mt-1 flex justify-between text-xs text-cf-text-muted">
              <span>
                {resultsWithBudget().filter((r) => r.withinBudget).length} /{" "}
                {hybridResults().length} {t("simulator.fitsInBudget")}
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
                  class={`rounded-cf-sm border p-3 ${
                    item.withinBudget
                      ? "border-cf-success-border bg-cf-success-bg/50"
                      : "border-cf-danger-border bg-cf-danger-bg/50"
                  }`}
                >
                  <div class="flex items-center justify-between">
                    <button
                      type="button"
                      class="flex items-center gap-2 text-sm font-medium text-cf-text-primary hover:text-cf-accent"
                      onClick={() => toggleExpanded(idx())}
                      aria-expanded={!!expanded()[idx()]}
                      aria-label={t("simulator.toggleResult", { path: item.hit.filepath })}
                    >
                      <span class="font-mono text-xs text-cf-text-muted" aria-hidden="true">
                        {expanded()[idx()] ? "\u25BC" : "\u25B6"}
                      </span>
                      <span class="font-mono text-xs">
                        {item.hit.filepath}:{item.hit.start_line}-{item.hit.end_line}
                      </span>
                    </button>
                    <div class="flex items-center gap-2 text-xs">
                      <Show when={item.hit.symbol_name}>
                        <Badge variant="primary">{item.hit.symbol_name}</Badge>
                      </Show>
                      <Badge variant="default">{item.hit.language}</Badge>
                      <span title={t("simulator.bm25Rank")} class="text-cf-text-muted">
                        B:{item.hit.bm25_rank}
                      </span>
                      <span title={t("simulator.semanticRank")} class="text-cf-text-muted">
                        S:{item.hit.semantic_rank}
                      </span>
                      <span
                        class="font-medium text-cf-text-secondary"
                        title={t("simulator.combinedScore")}
                      >
                        {fmt.score(item.hit.score)}
                      </span>
                      <Badge variant={item.withinBudget ? "success" : "danger"}>
                        {item.tokens}t
                      </Badge>
                    </div>
                  </div>
                  <Show when={expanded()[idx()]}>
                    <pre class="mt-2 max-h-48 overflow-auto rounded-cf-sm border border-cf-border bg-cf-bg-surface p-2 text-xs leading-relaxed text-cf-text-secondary">
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
          <div class="mt-4 border-t border-cf-border pt-4">
            <h4 class="mb-2 text-sm font-medium text-cf-text-secondary">
              {t("simulator.graphResults")} ({graphResults().length})
            </h4>
            <div class="space-y-1">
              <For each={graphResults()}>
                {(hit) => (
                  <div class="flex items-center gap-2 rounded-cf-sm bg-violet-50 p-2 text-xs dark:bg-violet-900/10">
                    <Badge variant="info">{hit.kind}</Badge>
                    <span class="font-mono text-cf-text-primary">{hit.symbol_name}</span>
                    <span class="text-cf-text-muted">
                      {hit.filepath}:{hit.start_line}
                    </span>
                    <Show when={hit.edge_path.length > 0}>
                      <span class="text-cf-text-muted" title={t("simulator.graphPath")}>
                        {hit.edge_path.join(" \u2192 ")}
                      </span>
                    </Show>
                    <span class="ml-auto text-cf-text-muted">{fmt.score(hit.score)}</span>
                  </div>
                )}
              </For>
            </div>
          </div>
        </Show>
      </Card.Body>
    </Card>
  );
}

export { SearchSimulator };
