import { createResource, createSignal, For, Match, onCleanup, Show, Switch } from "solid-js";

import { api } from "~/api/client";
import type {
  BenchmarkDatasetInfo,
  BenchmarkExecMode,
  BenchmarkRun,
  BenchmarkType,
  CreateBenchmarkRunRequest,
} from "~/api/types";
import { createCodeForgeWS } from "~/api/websocket";
import { useToast } from "~/components/Toast";
import { benchmarkStatusVariant, getVariant } from "~/config/statusVariants";
import { useI18n } from "~/i18n";
import {
  Badge,
  Button,
  Card,
  CostDisplay,
  EmptyState,
  FormField,
  Input,
  LoadingState,
  ModelCombobox,
  PageLayout,
  Select,
  Tabs,
} from "~/ui";

import { BenchmarkCompare } from "./BenchmarkCompare";
import { BenchmarkRunDetail } from "./BenchmarkRunDetail";
import { CostAnalysisView } from "./CostAnalysisView";
import { LeaderboardView } from "./LeaderboardView";
import { MultiCompareView } from "./MultiCompareView";
import { SuiteManagement } from "./SuiteManagement";

const METRIC_OPTIONS = [
  "correctness",
  "tool_correctness",
  "faithfulness",
  "answer_relevancy",
  "contextual_precision",
];

const TABS = [
  { value: "runs", label: "" },
  { value: "leaderboard", label: "" },
  { value: "costAnalysis", label: "" },
  { value: "multiCompare", label: "" },
  { value: "suites", label: "" },
] as const;

export default function BenchmarkPage() {
  const { t } = useI18n();
  const { show: toast } = useToast();
  const { onMessage } = createCodeForgeWS();
  const [runs, { refetch }] = createResource(() => api.benchmarks.listRuns());
  const [datasets] = createResource(() => api.benchmarks.listDatasets());

  // Tab persistence via URL search params
  const initialTab = new URLSearchParams(window.location.search).get("tab") || "runs";
  const [activeTab, setActiveTabRaw] = createSignal(initialTab);
  const setActiveTab = (tab: string) => {
    setActiveTabRaw(tab);
    const url = new URL(window.location.href);
    url.searchParams.set("tab", tab);
    window.history.replaceState({}, "", url.toString());
  };

  // WebSocket: auto-refresh runs when benchmark progress events arrive
  const cleanupWS = onMessage((msg) => {
    if (msg.type === "benchmark.run.progress" || msg.type === "benchmark.task.completed") {
      refetch();
    }
  });
  onCleanup(cleanupWS);

  // New run form
  const [showForm, setShowForm] = createSignal(false);
  const [dataset, setDataset] = createSignal("");
  const [model, setModel] = createSignal("");
  const [metrics, setMetrics] = createSignal<string[]>(["correctness"]);
  const [benchmarkType, setBenchmarkType] = createSignal<BenchmarkType>("simple");
  const [execMode, setExecMode] = createSignal<BenchmarkExecMode>("mount");

  // Run detail
  const [selectedRun, setSelectedRun] = createSignal<string | null>(null);
  const [results] = createResource(selectedRun, (id) =>
    id ? api.benchmarks.listResults(id) : undefined,
  );

  const tabItems = () =>
    TABS.map((tab) => ({
      value: tab.value,
      label: t(`benchmark.tab.${tab.value}` as keyof typeof t),
    }));

  const resetForm = () => {
    setDataset("");
    setModel("");
    setMetrics(["correctness"]);
    setBenchmarkType("simple");
    setExecMode("mount");
  };

  const handleCreate = async (e: SubmitEvent) => {
    e.preventDefault();
    const req: CreateBenchmarkRunRequest = {
      dataset: dataset(),
      model: model(),
      metrics: metrics(),
      benchmark_type: benchmarkType(),
      exec_mode: benchmarkType() === "agent" ? execMode() : undefined,
    };
    try {
      await api.benchmarks.createRun(req);
      toast("success", t("benchmark.toast.created"));
      setShowForm(false);
      resetForm();
      refetch();
    } catch {
      toast("error", t("benchmark.toast.createError"));
    }
  };

  const handleDelete = async (id: string) => {
    try {
      await api.benchmarks.deleteRun(id);
      toast("success", t("benchmark.toast.deleted"));
      if (selectedRun() === id) setSelectedRun(null);
      refetch();
    } catch {
      toast("error", t("benchmark.toast.deleteError"));
    }
  };

  const handleCancel = async (id: string) => {
    try {
      await api.benchmarks.cancelRun(id);
      toast("success", t("benchmark.toast.cancelled"));
      refetch();
    } catch {
      toast("error", t("benchmark.toast.cancelError"));
    }
  };

  const formatDuration = (ms: number) => {
    if (ms < 1000) return `${ms}ms`;
    return `${(ms / 1000).toFixed(1)}s`;
  };

  const toggleMetric = (m: string) => {
    setMetrics((prev) => (prev.includes(m) ? prev.filter((x) => x !== m) : [...prev, m]));
  };

  return (
    <PageLayout title={t("benchmark.title")} description={t("benchmark.subtitle")}>
      {/* Tab navigation */}
      <Tabs items={tabItems()} value={activeTab()} onChange={setActiveTab} class="mb-6" />

      <Switch>
        {/* ============ RUNS TAB ============ */}
        <Match when={activeTab() === "runs"}>
          <div class="mb-4 flex gap-2">
            <Button onClick={() => setShowForm(!showForm())} size="sm">
              {showForm() ? t("common.cancel") : t("benchmark.newRun")}
            </Button>
          </div>

          {/* New Run Form */}
          <Show when={showForm()}>
            <Card class="mb-6 p-4">
              <form onSubmit={handleCreate} class="space-y-4">
                <FormField label={t("benchmark.dataset")} id="benchmark-dataset">
                  <Show
                    when={datasets()?.length}
                    fallback={
                      <Input
                        value={dataset()}
                        onInput={(e) => setDataset(e.currentTarget.value)}
                        placeholder="basic-coding"
                        required
                      />
                    }
                  >
                    <Select value={dataset()} onChange={(e) => setDataset(e.currentTarget.value)}>
                      <option value="">{t("common.select")}</option>
                      <For each={datasets()}>
                        {(d: BenchmarkDatasetInfo) => (
                          <option value={d.path}>
                            {d.name} ({d.task_count} tasks)
                          </option>
                        )}
                      </For>
                    </Select>
                  </Show>
                </FormField>

                <FormField label={t("benchmark.model")} id="benchmark-model">
                  <ModelCombobox id="benchmark-model" value={model()} onInput={setModel} required />
                </FormField>

                <FormField label={t("benchmark.benchmarkType")} id="benchmark-type">
                  <Select
                    value={benchmarkType()}
                    onChange={(e) => setBenchmarkType(e.currentTarget.value as BenchmarkType)}
                  >
                    <option value="simple">Simple (prompt → output)</option>
                    <option value="tool_use">Tool Use (with tool calling)</option>
                    <option value="agent">Agent (multi-turn, exec modes)</option>
                  </Select>
                </FormField>

                <Show when={benchmarkType() === "agent"}>
                  <FormField label={t("benchmark.execMode")} id="benchmark-exec-mode">
                    <Select
                      value={execMode()}
                      onChange={(e) => setExecMode(e.currentTarget.value as BenchmarkExecMode)}
                    >
                      <option value="mount">Mount (direct file access)</option>
                      <option value="sandbox">Sandbox (isolated container)</option>
                      <option value="hybrid">Hybrid</option>
                    </Select>
                  </FormField>
                </Show>

                <FormField label={t("benchmark.metrics")} id="benchmark-metrics">
                  <div class="flex flex-wrap gap-2">
                    <For each={METRIC_OPTIONS}>
                      {(m) => (
                        <button
                          type="button"
                          class={`rounded px-2 py-1 text-xs font-medium transition ${
                            metrics().includes(m)
                              ? "bg-blue-600 text-white"
                              : "bg-gray-200 text-gray-700 dark:bg-gray-700 dark:text-gray-300"
                          }`}
                          onClick={() => toggleMetric(m)}
                        >
                          {m}
                        </button>
                      )}
                    </For>
                  </div>
                </FormField>

                <Button type="submit" variant="primary" size="sm">
                  {t("benchmark.startRun")}
                </Button>
              </form>
            </Card>
          </Show>

          {/* Run List */}
          <Show when={!runs.loading} fallback={<LoadingState />}>
            <Show when={runs()?.length} fallback={<EmptyState title={t("benchmark.empty")} />}>
              <div class="space-y-3">
                <For each={runs()}>
                  {(run: BenchmarkRun) => (
                    <div
                      class={`cursor-pointer transition hover:ring-1 hover:ring-blue-400 ${
                        selectedRun() === run.id ? "ring-2 ring-blue-500" : ""
                      }`}
                      onClick={() => setSelectedRun(selectedRun() === run.id ? null : run.id)}
                    >
                      <Card class="p-4">
                        <div class="flex items-center justify-between">
                          <div class="flex items-center gap-2">
                            <span class="font-medium">{run.dataset}</span>
                            <span class="text-sm text-gray-500">{run.model}</span>
                            <Show when={run.benchmark_type}>
                              <Badge
                                variant={
                                  run.benchmark_type === "agent"
                                    ? "info"
                                    : run.benchmark_type === "tool_use"
                                      ? "warning"
                                      : "default"
                                }
                              >
                                {run.benchmark_type}
                              </Badge>
                            </Show>
                            <Show when={run.exec_mode}>
                              <Badge variant="default">{run.exec_mode}</Badge>
                            </Show>
                          </div>
                          <div class="flex items-center gap-2">
                            <Badge
                              variant={getVariant(benchmarkStatusVariant, run.status, "warning")}
                            >
                              {run.status}
                            </Badge>
                            <span class="text-xs text-gray-400">
                              {formatDuration(run.total_duration_ms)}
                            </span>
                            <CostDisplay usd={run.total_cost} class="text-xs text-gray-400" />
                            <Show when={run.status === "running"}>
                              <Button
                                size="sm"
                                variant="warning"
                                onClick={(e: MouseEvent) => {
                                  e.stopPropagation();
                                  handleCancel(run.id);
                                }}
                              >
                                {t("common.cancel")}
                              </Button>
                            </Show>
                            <Show when={run.status === "completed"}>
                              <a
                                href={api.benchmarks.exportResultsUrl(run.id, "csv")}
                                target="_blank"
                                rel="noopener noreferrer"
                                class="rounded bg-gray-200 px-2 py-1 text-xs hover:bg-gray-300 dark:bg-gray-700 dark:hover:bg-gray-600"
                                onClick={(e: MouseEvent) => e.stopPropagation()}
                              >
                                CSV
                              </a>
                            </Show>
                            <Button
                              size="sm"
                              variant="danger"
                              onClick={(e: MouseEvent) => {
                                e.stopPropagation();
                                handleDelete(run.id);
                              }}
                            >
                              {t("common.delete")}
                            </Button>
                          </div>
                        </div>

                        <Show when={run.metrics?.length}>
                          <div class="mt-2 flex gap-1">
                            <For each={run.metrics}>
                              {(m) => <Badge variant="default">{m}</Badge>}
                            </For>
                          </div>
                        </Show>

                        {/* Progress bar for running runs */}
                        <Show when={run.status === "running"}>
                          <div class="mt-2">
                            <div class="h-2 w-full overflow-hidden rounded-full bg-gray-200 dark:bg-gray-700">
                              <div
                                class="h-2 animate-pulse rounded-full bg-blue-500"
                                style={{ width: "100%" }}
                              />
                            </div>
                            <span class="mt-1 text-xs text-gray-500">Running...</span>
                          </div>
                        </Show>

                        {/* Summary Scores */}
                        <Show
                          when={run.summary_scores && Object.keys(run.summary_scores).length > 0}
                        >
                          <div class="mt-2 flex gap-3 text-sm">
                            <For each={Object.entries(run.summary_scores)}>
                              {([key, val]) => (
                                <span>
                                  <span class="text-gray-500">{key}:</span>{" "}
                                  <span class="font-mono">{(val as number).toFixed(3)}</span>
                                </span>
                              )}
                            </For>
                          </div>
                        </Show>

                        {/* Expanded Results */}
                        <Show when={selectedRun() === run.id}>
                          <BenchmarkRunDetail
                            results={results()}
                            loading={results.loading}
                            formatDuration={formatDuration}
                          />
                        </Show>
                      </Card>
                    </div>
                  )}
                </For>
              </div>
            </Show>
          </Show>

          {/* Compare Section (2-run) */}
          <BenchmarkCompare runs={runs() ?? []} />

          {/* Datasets Info */}
          <Show when={datasets()?.length}>
            <Card class="mt-6 p-4">
              <h3 class="mb-3 text-sm font-semibold">{t("benchmark.datasets")}</h3>
              <div class="space-y-2">
                <For each={datasets()}>
                  {(d: BenchmarkDatasetInfo) => (
                    <div class="flex items-center justify-between text-sm">
                      <div>
                        <span class="font-medium">{d.name}</span>
                        <Show when={d.description}>
                          <span class="ml-2 text-gray-500">{d.description}</span>
                        </Show>
                      </div>
                      <Badge variant="default">
                        {d.task_count} {t("benchmark.tasks")}
                      </Badge>
                    </div>
                  )}
                </For>
              </div>
            </Card>
          </Show>
        </Match>

        {/* ============ LEADERBOARD TAB ============ */}
        <Match when={activeTab() === "leaderboard"}>
          <LeaderboardView />
        </Match>

        {/* ============ COST ANALYSIS TAB ============ */}
        <Match when={activeTab() === "costAnalysis"}>
          <CostAnalysisView runs={runs() ?? []} />
        </Match>

        {/* ============ MULTI-COMPARE TAB ============ */}
        <Match when={activeTab() === "multiCompare"}>
          <MultiCompareView runs={runs() ?? []} />
        </Match>

        {/* ============ SUITES TAB ============ */}
        <Match when={activeTab() === "suites"}>
          <SuiteManagement />
        </Match>
      </Switch>
    </PageLayout>
  );
}
