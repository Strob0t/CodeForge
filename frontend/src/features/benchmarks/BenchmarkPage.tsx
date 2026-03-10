import {
  createMemo,
  createResource,
  createSignal,
  For,
  Match,
  onCleanup,
  Show,
  Switch,
} from "solid-js";

import { api } from "~/api/client";
import type {
  BenchmarkExecMode,
  BenchmarkRun,
  BenchmarkSuite,
  BenchmarkType,
  CreateBenchmarkRunRequest,
  ProviderConfig,
} from "~/api/types";
import type { RoutingReport as RoutingReportType } from "~/api/types";
import { useToast } from "~/components/Toast";
import { useWebSocket } from "~/components/WebSocketProvider";
import { benchmarkStatusVariant, getVariant } from "~/config/statusVariants";
import { useFormState } from "~/hooks/useFormState";
import { useI18n } from "~/i18n";
import {
  Badge,
  Button,
  Card,
  Checkbox,
  CostDisplay,
  EmptyState,
  FormField,
  LoadingState,
  ModelCombobox,
  PageLayout,
  Select,
  Tabs,
} from "~/ui";

import { BenchmarkCompare } from "./BenchmarkCompare";
import { BenchmarkLiveFeed } from "./BenchmarkLiveFeed";
import { BenchmarkRunDetail } from "./BenchmarkRunDetail";
import { CostAnalysisView } from "./CostAnalysisView";
import { LeaderboardView } from "./LeaderboardView";
import { MultiCompareView } from "./MultiCompareView";
import { PromptOptimizationPanel } from "./PromptOptimizationPanel";
import { RoutingReport } from "./RoutingReport";
import { SuiteManagement } from "./SuiteManagement";
import { TaskSettings } from "./TaskSettings";

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

/** Map provider names to default benchmark types. */
const PROVIDER_TYPE_MAP: Record<string, BenchmarkType> = {
  deepeval: "simple",
  swebench: "agent",
  bigcodebench: "simple",
  humaneval: "simple",
  mbpp: "simple",
  cruxeval: "simple",
  livecodebench: "simple",
  sparcbench: "agent",
  aider_polyglot: "agent",
};

export default function BenchmarkPage() {
  const { t } = useI18n();
  const { show: toast } = useToast();
  const { onMessage } = useWebSocket();
  const [runs, { refetch }] = createResource(() => api.benchmarks.listRuns());
  const [suites] = createResource(() => api.benchmarks.listSuites());

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
  const formDefaults = {
    suiteId: "",
    model: "",
    metrics: ["correctness"] as string[],
    benchmarkType: "simple" as BenchmarkType,
    execMode: "mount" as BenchmarkExecMode,
    providerConfig: {} as ProviderConfig,
  };
  const form = useFormState(formDefaults);

  // Run detail
  const [selectedRun, setSelectedRun] = createSignal<string | null>(null);
  const [results] = createResource(selectedRun, (id) =>
    id ? api.benchmarks.listResults(id) : undefined,
  );

  const tabItems = createMemo(() =>
    TABS.map((tab) => ({
      value: tab.value,
      label: t(`benchmark.tab.${tab.value}` as keyof typeof t),
    })),
  );

  const resetForm = () => form.reset();

  const handleCreate = async (e: SubmitEvent) => {
    e.preventDefault();
    const hasProviderConfig = Object.keys(form.state.providerConfig).length > 0;
    const req: CreateBenchmarkRunRequest = {
      suite_id: form.state.suiteId || undefined,
      model: form.state.model,
      metrics: form.state.metrics,
      benchmark_type: form.state.benchmarkType,
      exec_mode: form.state.benchmarkType === "agent" ? form.state.execMode : undefined,
      provider_config: hasProviderConfig ? form.state.providerConfig : undefined,
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
    const prev = form.state.metrics;
    form.setState("metrics", prev.includes(m) ? prev.filter((x) => x !== m) : [...prev, m]);
  };

  const selectedSuite = (): BenchmarkSuite | undefined =>
    suites()?.find((s) => s.id === form.state.suiteId);

  const localSuites = () => suites()?.filter((s) => s.type === "deepeval") ?? [];
  const externalSuites = () => suites()?.filter((s) => s.type !== "deepeval") ?? [];

  const handleSuiteChange = (suiteId: string) => {
    form.setState("suiteId", suiteId);
    form.setState("providerConfig", {} as ProviderConfig);
    const suite = suites()?.find((s) => s.id === suiteId);
    if (suite) {
      form.setState("benchmarkType", PROVIDER_TYPE_MAP[suite.provider_name] ?? "simple");
    }
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
                <FormField label={t("benchmark.suite")} id="benchmark-suite">
                  <Select
                    value={form.state.suiteId}
                    onChange={(e) => handleSuiteChange(e.currentTarget.value)}
                  >
                    <option value="">{t("common.select")}</option>
                    <Show when={localSuites().length}>
                      <optgroup label={t("benchmark.suiteLocal")}>
                        <For each={localSuites()}>
                          {(s: BenchmarkSuite) => (
                            <option value={s.id}>
                              {s.name} ({s.task_count} tasks)
                            </option>
                          )}
                        </For>
                      </optgroup>
                    </Show>
                    <Show when={externalSuites().length}>
                      <optgroup label={t("benchmark.suiteExternal")}>
                        <For each={externalSuites()}>
                          {(s: BenchmarkSuite) => (
                            <option value={s.id}>
                              {s.name} ({s.task_count} tasks)
                            </option>
                          )}
                        </For>
                      </optgroup>
                    </Show>
                  </Select>
                </FormField>

                <Show when={selectedSuite()}>
                  {(suite) => (
                    <TaskSettings
                      providerName={suite().provider_name}
                      config={form.state.providerConfig}
                      onChange={(c) => form.setState("providerConfig", c)}
                      taskCount={suite().task_count}
                    />
                  )}
                </Show>

                <FormField label={t("benchmark.model")} id="benchmark-model">
                  <div class="space-y-2">
                    <Checkbox
                      label={t("benchmark.modelAuto")}
                      checked={form.state.model === "auto"}
                      onChange={(v) => form.setState("model", v ? "auto" : "")}
                    />
                    <Show when={form.state.model !== "auto"}>
                      <ModelCombobox
                        id="benchmark-model"
                        value={form.state.model}
                        onInput={(v) => form.setState("model", v)}
                        required
                      />
                    </Show>
                  </div>
                </FormField>

                <FormField label={t("benchmark.benchmarkType")} id="benchmark-type">
                  <Select
                    value={form.state.benchmarkType}
                    onChange={(e) =>
                      form.setState("benchmarkType", e.currentTarget.value as BenchmarkType)
                    }
                  >
                    <option value="simple">{`Simple (prompt \u2192 output)`}</option>
                    <option value="tool_use">Tool Use (with tool calling)</option>
                    <option value="agent">Agent (multi-turn, exec modes)</option>
                  </Select>
                </FormField>

                <Show when={form.state.benchmarkType === "agent"}>
                  <FormField label={t("benchmark.execMode")} id="benchmark-exec-mode">
                    <Select
                      value={form.state.execMode}
                      onChange={(e) =>
                        form.setState("execMode", e.currentTarget.value as BenchmarkExecMode)
                      }
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
                        <Button
                          variant="pill"
                          size="xs"
                          class={
                            form.state.metrics.includes(m)
                              ? "border-cf-accent bg-cf-accent/10 text-cf-accent"
                              : ""
                          }
                          onClick={() => toggleMetric(m)}
                        >
                          {m}
                        </Button>
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
                                variant="danger"
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

                        {/* Live feed for selected running runs, minimal pulse bar otherwise */}
                        <Show when={run.status === "running" && selectedRun() === run.id}>
                          <BenchmarkLiveFeed runId={run.id} startedAt={run.created_at} />
                        </Show>
                        <Show when={run.status === "running" && selectedRun() !== run.id}>
                          <div class="mt-2">
                            <div class="h-1.5 w-full overflow-hidden rounded-full bg-gray-200 dark:bg-gray-700">
                              <div
                                class="h-1.5 animate-pulse rounded-full bg-blue-500"
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
                          <Show
                            when={
                              run.model === "auto" &&
                              run.status === "completed" &&
                              run.summary_scores?.routing_report
                            }
                          >
                            <RoutingReport
                              report={
                                run.summary_scores.routing_report as unknown as RoutingReportType
                              }
                            />
                          </Show>
                          <Show when={run.status === "completed"}>
                            <PromptOptimizationPanel runId={run.id} suiteId={run.suite_id ?? ""} />
                          </Show>
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
