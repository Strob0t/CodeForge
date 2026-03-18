import {
  createEffect,
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
  LiveFeedEvent,
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
import {
  agentEventToLiveFeedEvent,
  emptyLiveFeedState,
  type FeatureEntry,
  type LiveFeedState,
  MAX_EVENTS,
  resultToFeatureEntry,
  statsFromSummary,
} from "./liveFeedState";
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
  const { onMessage, connected } = useWebSocket();
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

  // Live feed state per running benchmark — persists across card close/reopen
  const [liveFeedStates, setLiveFeedStates] = createSignal<Map<string, LiveFeedState>>(new Map());

  // Helper: update a single run's LiveFeedState
  const updateRunState = (runId: string, updater: (prev: LiveFeedState) => LiveFeedState) => {
    setLiveFeedStates((prev) => {
      const next = new Map(prev);
      const current = next.get(runId) ?? emptyLiveFeedState();
      next.set(runId, updater(current));
      return next;
    });
  };

  // WebSocket: auto-refresh runs list and update live feed state
  const cleanupWS = onMessage((msg) => {
    // Auto-refresh runs list on progress/completion
    if (msg.type === "benchmark.run.progress" || msg.type === "benchmark.task.completed") {
      refetch();
    }

    // ---- Live feed state updates ----
    if (msg.type === "trajectory.event") {
      const p = msg.payload as {
        run_id: string;
        project_id: string;
        event_type: string;
        sequence_number?: number;
        tool_name?: string;
        model?: string;
        input?: string;
        output?: string;
        success?: boolean;
        step?: number;
        cost_usd?: number;
        tokens_in?: number;
        tokens_out?: number;
      };
      updateRunState(p.run_id, (state) => {
        // Dedup: skip events already seen via REST hydration or prior WS delivery.
        const seqNum = p.sequence_number ?? 0;
        if (seqNum > 0 && seqNum <= state.lastSequenceNumber) {
          return state;
        }

        const evt: LiveFeedEvent = {
          id: crypto.randomUUID(),
          timestamp: Date.now(),
          run_id: p.run_id,
          project_id: p.project_id ?? "",
          event_type: p.event_type,
          sequence_number: seqNum,
          tool_name: p.tool_name,
          model: p.model,
          input: p.input,
          output: p.output,
          success: p.success,
          step: p.step,
          cost_usd: p.cost_usd,
          tokens_in: p.tokens_in,
          tokens_out: p.tokens_out,
        };

        const events = [...state.events, evt];
        const trimmed =
          events.length > MAX_EVENTS ? events.slice(events.length - MAX_EVENTS) : events;

        const stats = { ...state.stats };
        if (p.event_type === "agent.tool_called") {
          stats.toolCallCount++;
          if (p.success !== false) stats.toolSuccessCount++;
        }
        stats.totalTokensIn += p.tokens_in ?? 0;
        stats.totalTokensOut += p.tokens_out ?? 0;

        const features = new Map(state.features);
        for (const [id, f] of features) {
          if (f.status === "running") {
            features.set(id, {
              ...f,
              events: [...f.events, evt],
              cost: f.cost + (p.cost_usd ?? 0),
              step: p.step ?? f.step,
            });
            break;
          }
        }

        const newSeq = seqNum > state.lastSequenceNumber ? seqNum : state.lastSequenceNumber;
        return {
          ...state,
          events: trimmed,
          stats,
          features,
          lastEventId: evt.id,
          lastSequenceNumber: newSeq,
        };
      });
    }

    if (msg.type === "benchmark.run.progress") {
      const p = msg.payload as {
        run_id: string;
        completed_tasks: number;
        total_tasks: number;
        avg_score: number;
        total_cost_usd: number;
      };
      updateRunState(p.run_id, (state) => {
        const progress = {
          completed_tasks: p.completed_tasks,
          total_tasks: p.total_tasks,
          avg_score: p.avg_score,
          total_cost_usd: p.total_cost_usd,
        };
        const stats = { ...state.stats };
        stats.avgScore = p.avg_score;
        stats.costPerTask = p.completed_tasks > 0 ? p.total_cost_usd / p.completed_tasks : 0;
        return { ...state, progress, stats };
      });
    }

    if (msg.type === "benchmark.task.started") {
      const p = msg.payload as {
        run_id: string;
        task_id: string;
        task_name: string;
        index: number;
        total: number;
      };
      updateRunState(p.run_id, (state) => {
        const features = new Map(state.features);
        if (!features.has(p.task_id)) {
          features.set(p.task_id, {
            id: p.task_id,
            name: p.task_name,
            status: "running",
            events: [],
            startedAt: Date.now(),
            cost: 0,
            step: 0,
          });
        }
        return { ...state, features };
      });
    }

    if (msg.type === "benchmark.task.completed") {
      const p = msg.payload as {
        run_id: string;
        task_id: string;
        task_name: string;
        score: number;
        cost_usd: number;
      };
      updateRunState(p.run_id, (state) => {
        const features = new Map(state.features);
        const existing = features.get(p.task_id);
        features.set(p.task_id, {
          id: p.task_id,
          name: p.task_name,
          status: "completed",
          events: existing?.events ?? [],
          startedAt: existing?.startedAt,
          cost: p.cost_usd,
          step: existing?.step ?? 0,
          score: p.score,
        });
        return { ...state, features };
      });
    }

    if (msg.type === "autoagent.status") {
      const p = msg.payload as { run_id?: string; current_feature_id?: string };
      const { run_id: statusRunId, current_feature_id: currentFeatureId } = p;
      if (statusRunId && currentFeatureId) {
        updateRunState(statusRunId, (state) => {
          const features = new Map(state.features);
          for (const [id, f] of features) {
            if (f.status === "running" && id !== currentFeatureId) {
              features.set(id, { ...f, status: "pending" });
            }
          }
          const target = features.get(currentFeatureId);
          if (target && target.status !== "completed") {
            features.set(currentFeatureId, { ...target, status: "running" });
          }
          return { ...state, features };
        });
      }
    }
  });
  onCleanup(cleanupWS);

  // Hydrate live feed state for running runs from API
  createEffect(() => {
    const runList = runs();
    if (!runList) return;
    const runningRuns = runList.filter((r: BenchmarkRun) => r.status === "running");

    for (const run of runningRuns) {
      const existing = liveFeedStates().get(run.id);
      if (existing?.hydratedFromApi) continue;

      Promise.all([api.trajectory.get(run.id, { limit: 200 }), api.benchmarks.listResults(run.id)])
        .then(([trajectory, resultsList]) => {
          const events = trajectory.events.map(agentEventToLiveFeedEvent);
          const stats = statsFromSummary(trajectory.stats, resultsList);

          const features = new Map<string, FeatureEntry>();
          for (const r of resultsList) {
            features.set(r.task_id, resultToFeatureEntry(r));
          }

          const completedCount = resultsList.length;
          const avgScore =
            completedCount > 0
              ? resultsList.reduce((sum: number, r) => {
                  const first = r.scores ? (Object.values(r.scores)[0] ?? 0) : 0;
                  return sum + (first as number);
                }, 0) / completedCount
              : 0;

          const progress = {
            completed_tasks: completedCount,
            total_tasks: null as number | null,
            avg_score: avgScore,
            total_cost_usd: trajectory.stats.total_cost_usd,
          };

          const lastEvent = events.length > 0 ? events[events.length - 1] : null;

          // Track the maximum sequence_number from REST for dedup against WS events.
          const maxSeq = events.reduce((max, e) => Math.max(max, e.sequence_number ?? 0), 0);

          updateRunState(run.id, (prev) => ({
            events: prev.events.length > events.length ? prev.events : events,
            progress: prev.progress ?? progress,
            features: prev.features.size > features.size ? prev.features : features,
            stats: prev.hydratedFromApi ? prev.stats : stats,
            hydratedFromApi: true,
            lastEventId: prev.lastEventId ?? lastEvent?.id ?? null,
            lastSequenceNumber: Math.max(prev.lastSequenceNumber, maxSeq),
          }));
        })
        .catch((err: unknown) => {
          console.warn(`[LiveFeed] hydration failed for run ${run.id}:`, err);
          updateRunState(run.id, (prev) => ({ ...prev, hydratedFromApi: true }));
        });
    }
  });

  // WS reconnect gap-fill: when the WebSocket reconnects, re-fetch events
  // since the last known sequence_number to fill any gaps from the disconnect.
  const [wasConnected, setWasConnected] = createSignal(false);
  createEffect(() => {
    const isConnected = connected();
    const prev = wasConnected();
    setWasConnected(isConnected);

    // Only trigger gap-fill on actual reconnect (was disconnected, now connected).
    if (!isConnected || !prev === !isConnected) return;
    if (!prev && isConnected) {
      const runList = runs();
      if (!runList) return;
      const runningRuns = runList.filter((r: BenchmarkRun) => r.status === "running");
      for (const run of runningRuns) {
        const state = liveFeedStates().get(run.id);
        if (!state || state.lastSequenceNumber === 0) continue;

        api.trajectory
          .get(run.id, { limit: 500, after_sequence: state.lastSequenceNumber })
          .then((trajectory) => {
            const gapEvents = trajectory.events.map(agentEventToLiveFeedEvent);
            if (gapEvents.length === 0) return;

            const maxSeq = gapEvents.reduce((max, e) => Math.max(max, e.sequence_number ?? 0), 0);

            updateRunState(run.id, (prev) => {
              const merged = [...prev.events, ...gapEvents];
              const trimmed =
                merged.length > MAX_EVENTS ? merged.slice(merged.length - MAX_EVENTS) : merged;
              return {
                ...prev,
                events: trimmed,
                lastSequenceNumber: Math.max(prev.lastSequenceNumber, maxSeq),
              };
            });
          })
          .catch((err: unknown) => {
            console.warn(`[LiveFeed] reconnect gap-fill failed for run ${run.id}:`, err);
          });
      }
    }
  });

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
                      onClick={(e: MouseEvent) => {
                        const target = e.target as HTMLElement;
                        if (
                          target.closest("table") ||
                          target.closest("button") ||
                          target.closest("a")
                        )
                          return;
                        setSelectedRun(selectedRun() === run.id ? null : run.id);
                      }}
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

                        {/* Minimal pulse bar for non-selected running runs */}
                        <Show when={run.status === "running" && selectedRun() !== run.id}>
                          <div class="mt-2">
                            <div class="h-1.5 w-full overflow-hidden rounded-full bg-cf-bg-secondary">
                              <div
                                class="h-1.5 animate-pulse rounded-full bg-cf-accent"
                                style={{ width: "100%" }}
                              />
                            </div>
                            <span class="mt-1 text-xs text-cf-text-muted">Running...</span>
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
                          <Show when={run.status === "running"}>
                            <BenchmarkLiveFeed
                              state={liveFeedStates().get(run.id) ?? emptyLiveFeedState()}
                              startedAt={run.created_at}
                            />
                          </Show>
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
