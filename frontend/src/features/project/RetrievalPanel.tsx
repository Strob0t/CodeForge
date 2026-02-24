import { createResource, createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type {
  GraphStatus,
  RetrievalIndexStatus,
  RetrievalSearchHit,
  SubAgentSearchResult,
} from "~/api/types";
import { useI18n } from "~/i18n";
import { Alert, Badge, Button, Card, Input } from "~/ui";

interface RetrievalPanelProps {
  projectId: string;
  onStatusUpdate?: (status: string) => void;
}

function statusBadgeVariant(status: string): "success" | "warning" | "danger" | "default" {
  switch (status) {
    case "ready":
      return "success";
    case "building":
      return "warning";
    case "error":
      return "danger";
    default:
      return "default";
  }
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

  return (
    <Card>
      <Card.Header>
        <div class="flex items-center justify-between">
          <h3 class="text-lg font-semibold">{t("retrieval.title")}</h3>
          <Button
            variant="primary"
            size="sm"
            onClick={handleBuildIndex}
            disabled={building()}
            loading={building()}
          >
            {building()
              ? t("retrieval.building")
              : indexStatus()
                ? t("retrieval.rebuildIndex")
                : t("retrieval.buildIndex")}
          </Button>
        </div>
      </Card.Header>

      <Card.Body>
        <Show when={error()}>
          <div class="mb-3">
            <Alert variant="error">{error()}</Alert>
          </div>
        </Show>

        <Show
          when={!indexStatus.loading}
          fallback={<p class="text-sm text-cf-text-muted">{t("common.loading")}</p>}
        >
          <Show
            when={indexStatus()}
            fallback={<p class="text-sm text-cf-text-tertiary">{t("retrieval.empty")}</p>}
          >
            {(status) => (
              <>
                {/* Stats */}
                <div class="mb-3 grid grid-cols-4 gap-3">
                  <div class="rounded-cf-sm border border-cf-border-subtle bg-cf-bg-surface-alt p-2 text-center">
                    <div class="text-lg font-semibold text-cf-text-primary">
                      {fmt.compact(status().file_count)}
                    </div>
                    <div class="text-xs text-cf-text-tertiary">{t("retrieval.files")}</div>
                  </div>
                  <div class="rounded-cf-sm border border-cf-border-subtle bg-cf-bg-surface-alt p-2 text-center">
                    <div class="text-lg font-semibold text-cf-text-primary">
                      {fmt.compact(status().chunk_count)}
                    </div>
                    <div class="text-xs text-cf-text-tertiary">{t("retrieval.chunks")}</div>
                  </div>
                  <div class="rounded-cf-sm border border-cf-border-subtle bg-cf-bg-surface-alt p-2 text-center">
                    <div class="text-xs font-medium text-cf-text-primary">
                      {status().embedding_model || "\u2014"}
                    </div>
                    <div class="text-xs text-cf-text-tertiary">{t("retrieval.model")}</div>
                  </div>
                  <div class="rounded-cf-sm border border-cf-border-subtle bg-cf-bg-surface-alt p-2 text-center">
                    <Badge variant={statusBadgeVariant(status().status)} pill>
                      {status().status}
                    </Badge>
                    <div class="text-xs text-cf-text-tertiary">{t("common.status")}</div>
                  </div>
                </div>

                <Show when={status().error}>
                  <div class="mb-3">
                    <Alert variant="error">
                      {t("retrieval.indexError")} {status().error}
                    </Alert>
                  </div>
                </Show>

                {/* Search */}
                <form class="mb-3 flex gap-2" onSubmit={handleSearch}>
                  <Input
                    type="text"
                    class="flex-1"
                    placeholder={
                      useAgent()
                        ? t("retrieval.agentSearchPlaceholder")
                        : t("retrieval.searchPlaceholder")
                    }
                    value={query()}
                    onInput={(e) => setQuery(e.currentTarget.value)}
                    aria-label={t("retrieval.searchAria")}
                  />
                  <Button
                    variant={useAgent() ? "secondary" : "ghost"}
                    size="sm"
                    onClick={() => {
                      setUseAgent((v) => !v);
                      setExpandedQueries([]);
                      setTotalCandidates(0);
                    }}
                    title={useAgent() ? t("retrieval.agentTitle") : t("retrieval.standardTitle")}
                    aria-label={
                      useAgent()
                        ? t("retrieval.agentSwitchAria")
                        : t("retrieval.standardSwitchAria")
                    }
                    aria-pressed={useAgent()}
                  >
                    {useAgent() ? t("retrieval.agentToggle") : t("retrieval.standardToggle")}
                  </Button>
                  <Button
                    type="submit"
                    variant="primary"
                    size="sm"
                    disabled={searching() || status().status !== "ready" || !query().trim()}
                    loading={searching()}
                  >
                    {searching() ? t("retrieval.searching") : t("retrieval.search")}
                  </Button>
                </form>

                {/* Expanded queries from agent search */}
                <Show when={expandedQueries().length > 0}>
                  <div class="mb-3">
                    <div class="mb-1 flex items-center gap-2 text-xs text-cf-text-tertiary">
                      <span>{t("retrieval.expandedQueries")}</span>
                      <Show when={totalCandidates() > 0}>
                        <Badge variant="default">
                          {t("retrieval.candidates", { n: totalCandidates() })}
                        </Badge>
                      </Show>
                    </div>
                    <div class="flex flex-wrap gap-1">
                      <For each={expandedQueries()}>
                        {(eq) => <Badge variant="info">{eq}</Badge>}
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
            <h4 class="mb-2 text-sm font-medium text-cf-text-tertiary">
              {t("retrieval.results", { n: searchResults().length })}
            </h4>
            <div class="space-y-2">
              <For each={searchResults()}>
                {(hit, idx) => (
                  <div class="rounded-cf-sm border border-cf-border bg-cf-bg-surface-alt p-3">
                    <div class="flex items-center justify-between">
                      <button
                        type="button"
                        class="flex items-center gap-2 text-sm font-medium text-cf-text-primary hover:text-cf-accent"
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
                          <Badge variant="primary">{hit.symbol_name}</Badge>
                        </Show>
                        <Badge variant="default">{hit.language}</Badge>
                        <span class="text-xs text-cf-text-muted" title="Relevance score">
                          {fmt.score(hit.score)}
                        </span>
                      </div>
                    </div>
                    <Show when={expanded()[idx()]}>
                      <pre class="mt-2 max-h-64 overflow-auto rounded-cf-sm border border-cf-border bg-cf-bg-surface p-3 text-xs leading-relaxed text-cf-text-secondary">
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
        <div class="mt-4 border-t border-cf-border pt-4">
          <div class="mb-3 flex items-center justify-between">
            <h3 class="text-lg font-semibold">{t("retrieval.graph.title")}</h3>
            <Button
              variant="primary"
              size="sm"
              onClick={handleBuildGraph}
              disabled={buildingGraph() || graphStatus()?.status === "building"}
              loading={buildingGraph()}
            >
              {buildingGraph() || graphStatus()?.status === "building"
                ? t("retrieval.graph.rebuilding")
                : graphStatus()?.status === "ready"
                  ? t("retrieval.graph.rebuild")
                  : t("retrieval.graph.build")}
            </Button>
          </div>

          <Show
            when={!graphStatus.loading}
            fallback={<p class="text-sm text-cf-text-muted">{t("common.loading")}</p>}
          >
            <Show
              when={graphStatus()}
              fallback={<p class="text-sm text-cf-text-tertiary">{t("retrieval.graph.empty")}</p>}
            >
              {(gs) => (
                <>
                  <div class="mb-3 grid grid-cols-3 gap-3">
                    <div class="rounded-cf-sm border border-cf-border-subtle bg-cf-bg-surface-alt p-2 text-center">
                      <div class="text-lg font-semibold text-cf-text-primary">
                        {fmt.compact(gs().node_count)}
                      </div>
                      <div class="text-xs text-cf-text-tertiary">{t("retrieval.graph.nodes")}</div>
                    </div>
                    <div class="rounded-cf-sm border border-cf-border-subtle bg-cf-bg-surface-alt p-2 text-center">
                      <div class="text-lg font-semibold text-cf-text-primary">
                        {fmt.compact(gs().edge_count)}
                      </div>
                      <div class="text-xs text-cf-text-tertiary">{t("retrieval.graph.edges")}</div>
                    </div>
                    <div class="rounded-cf-sm border border-cf-border-subtle bg-cf-bg-surface-alt p-2 text-center">
                      <Badge variant={statusBadgeVariant(gs().status)} pill>
                        {gs().status}
                      </Badge>
                      <div class="text-xs text-cf-text-tertiary">{t("common.status")}</div>
                    </div>
                  </div>

                  <Show when={gs().languages.length > 0}>
                    <div class="mb-3">
                      <span class="mr-2 text-xs text-cf-text-tertiary">
                        {t("retrieval.graph.languages")}
                      </span>
                      <div class="inline-flex flex-wrap gap-1">
                        <For each={gs().languages}>
                          {(lang) => <Badge variant="info">{lang}</Badge>}
                        </For>
                      </div>
                    </div>
                  </Show>

                  <Show when={gs().built_at}>
                    <div class="mb-3 text-xs text-cf-text-muted">
                      {t("retrieval.graph.built", {
                        date: fmt.dateTime(gs().built_at ?? ""),
                      })}
                    </div>
                  </Show>

                  <Show when={gs().error}>
                    <Alert variant="error">
                      {t("retrieval.graph.error")} {gs().error}
                    </Alert>
                  </Show>
                </>
              )}
            </Show>
          </Show>
        </div>
      </Card.Body>
    </Card>
  );
}

export { RetrievalPanel };
