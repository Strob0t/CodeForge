import { createEffect, createMemo, createResource, createSignal, onCleanup } from "solid-js";

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
import { useToast } from "~/components/Toast";
import { useWebSocket } from "~/components/WebSocketProvider";
import { useFormState } from "~/hooks/useFormState";
import { useI18n } from "~/i18n";

import {
  agentEventToLiveFeedEvent,
  emptyLiveFeedState,
  type FeatureEntry,
  type LiveFeedState,
  MAX_EVENTS,
  resultToFeatureEntry,
  statsFromSummary,
} from "./liveFeedState";

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Hook
// ---------------------------------------------------------------------------

export function useBenchmarkPage() {
  const { t } = useI18n();
  const { show: toast } = useToast();
  const { onMessage, connected } = useWebSocket();

  // ---- Data resources ----
  const [runs, { refetch }] = createResource(() => api.benchmarks.listRuns());
  const [suites] = createResource(() => api.benchmarks.listSuites());

  // ---- Tab persistence via URL search params ----
  const initialTab = new URLSearchParams(window.location.search).get("tab") || "runs";
  const [activeTab, setActiveTabRaw] = createSignal(initialTab);
  const setActiveTab = (tab: string) => {
    setActiveTabRaw(tab);
    const url = new URL(window.location.href);
    url.searchParams.set("tab", tab);
    window.history.replaceState({}, "", url.toString());
  };

  // ---- Live feed state per running benchmark ----
  const [liveFeedStates, setLiveFeedStates] = createSignal<Map<string, LiveFeedState>>(new Map());

  const updateRunState = (runId: string, updater: (prev: LiveFeedState) => LiveFeedState) => {
    setLiveFeedStates((prev) => {
      const next = new Map(prev);
      const current = next.get(runId) ?? emptyLiveFeedState();
      next.set(runId, updater(current));
      return next;
    });
  };

  // ---- WebSocket: auto-refresh + live feed ----
  const cleanupWS = onMessage((msg) => {
    if (msg.type === "benchmark.run.progress" || msg.type === "benchmark.task.completed") {
      refetch();
    }

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

  // ---- Hydrate live feed state from API for running runs ----
  createEffect(() => {
    const runList = runs();
    if (!runList) return;
    const runningRuns = runList.filter((r: BenchmarkRun) => r.status === "running");

    for (const run of runningRuns) {
      const existing = liveFeedStates().get(run.id);
      if (existing?.hydratedFromApi) continue;

      void (async () => {
        try {
          const [trajectory, resultsList] = await Promise.all([
            api.trajectory.get(run.id, { limit: 200 }),
            api.benchmarks.listResults(run.id),
          ]);
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
        } catch {
          updateRunState(run.id, (prev) => ({ ...prev, hydratedFromApi: true }));
        }
      })();
    }
  });

  // ---- WS reconnect gap-fill ----
  const [wasConnected, setWasConnected] = createSignal(false);
  createEffect(() => {
    const isConnected = connected();
    const prev = wasConnected();
    setWasConnected(isConnected);

    if (!isConnected || !prev === !isConnected) return;
    if (!prev && isConnected) {
      const runList = runs();
      if (!runList) return;
      const runningRuns = runList.filter((r: BenchmarkRun) => r.status === "running");
      for (const run of runningRuns) {
        const state = liveFeedStates().get(run.id);
        if (!state || state.lastSequenceNumber === 0) continue;

        void (async () => {
          try {
            const trajectory = await api.trajectory.get(run.id, {
              limit: 500,
              after_sequence: state.lastSequenceNumber,
            });
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
          } catch {
            // best-effort gap-fill
          }
        })();
      }
    }
  });

  // ---- New run form ----
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

  // ---- Run detail ----
  const [selectedRun, setSelectedRun] = createSignal<string | null>(null);
  const [results] = createResource(selectedRun, (id) =>
    id ? api.benchmarks.listResults(id) : undefined,
  );

  // ---- Tab items ----
  const TABS = [
    { value: "runs", label: "" },
    { value: "leaderboard", label: "" },
    { value: "costAnalysis", label: "" },
    { value: "multiCompare", label: "" },
    { value: "suites", label: "" },
  ] as const;

  const tabItems = createMemo(() =>
    TABS.map((tab) => ({
      value: tab.value,
      label: t(`benchmark.tab.${tab.value}` as keyof typeof t),
    })),
  );

  // ---- Handlers ----

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

  return {
    // Resources
    runs,
    suites,
    results,

    // Tab state
    activeTab,
    setActiveTab,
    tabItems,

    // Live feed state
    liveFeedStates,

    // Form state
    showForm,
    setShowForm,
    form,

    // Run detail
    selectedRun,
    setSelectedRun,

    // Derived
    selectedSuite,
    localSuites,
    externalSuites,

    // Handlers
    handleCreate,
    handleDelete,
    handleCancel,
    handleSuiteChange,
    toggleMetric,
    formatDuration,
  };
}
