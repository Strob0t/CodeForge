import { createVirtualizer } from "@tanstack/solid-virtual";
import { createEffect, createMemo, createSignal, For, on, onCleanup, Show } from "solid-js";

import type { BenchmarkLiveProgress, LiveFeedEvent } from "~/api/types";
import { useWebSocket } from "~/components/WebSocketProvider";

const MAX_EVENTS = 5000;

interface BenchmarkLiveFeedProps {
  runId: string;
  startedAt: string;
}

interface FeatureEntry {
  id: string;
  name: string;
  status: "pending" | "running" | "completed" | "failed";
  events: LiveFeedEvent[];
  startedAt?: number;
  cost: number;
  step: number;
  score?: number;
}

// ---- Typed WS payload interfaces (single boundary cast per message type) ----

interface TrajectoryPayload {
  run_id: string;
  project_id: string;
  event_type: string;
  tool_name?: string;
  model?: string;
  input?: string;
  output?: string;
  success?: boolean;
  step?: number;
  cost_usd?: number;
  tokens_in?: number;
  tokens_out?: number;
}

interface ProgressPayload {
  run_id: string;
  completed_tasks: number;
  total_tasks: number;
  avg_score: number;
  total_cost_usd: number;
}

interface TaskStartedPayload {
  run_id: string;
  task_id: string;
  task_name: string;
  index: number;
  total: number;
}

interface TaskCompletedPayload {
  run_id: string;
  task_id: string;
  task_name: string;
  score: number;
  cost_usd: number;
}

interface AutoAgentStatusPayload {
  run_id?: string;
  project_id?: string;
  current_feature_id?: string;
  status?: string;
}

// ---- Helpers ----

function formatElapsed(totalSeconds: number): string {
  const h = Math.floor(totalSeconds / 3600);
  const m = Math.floor((totalSeconds % 3600) / 60);
  const s = Math.floor(totalSeconds % 60);
  const mm = String(m).padStart(2, "0");
  const ss = String(s).padStart(2, "0");
  if (h > 0) return `${String(h).padStart(2, "0")}:${mm}:${ss}`;
  return `${mm}:${ss}`;
}

function truncate(text: string, maxLen: number): string {
  if (text.length <= maxLen) return text;
  return text.slice(0, maxLen) + "\u2026";
}

function eventIcon(type: string): string {
  switch (type) {
    case "agent.tool_called":
      return ">_";
    case "agent.step_done":
      return "\u25A0"; // ▪ small square
    case "agent.finished":
      return "\u2713"; // ✓
    default:
      return "\u2022"; // •
  }
}

function renderEventText(evt: LiveFeedEvent): string {
  switch (evt.event_type) {
    case "agent.tool_called":
      return `${evt.tool_name ?? "tool"}: ${truncate(evt.output ?? "", 120)}`;
    case "agent.step_done":
      return `Step ${evt.step ?? "?"} | ${evt.model ?? "?"} | ${evt.tokens_in ?? 0}+${evt.tokens_out ?? 0} tok | $${(evt.cost_usd ?? 0).toFixed(4)}`;
    case "agent.finished":
      return `Agent finished | $${(evt.cost_usd ?? 0).toFixed(4)} total`;
    default:
      return evt.event_type;
  }
}

function eventBadge(evt: LiveFeedEvent): { text: string; cls: string } | null {
  if (evt.event_type !== "agent.tool_called") return null;
  if (evt.success === true) return { text: "OK", cls: "text-cf-success-fg" };
  if (evt.success === false) return { text: "FAIL", cls: "text-cf-danger-fg" };
  return null;
}

function featureStatusIcon(status: string): string {
  switch (status) {
    case "completed":
      return "\u2713"; // ✓
    case "failed":
      return "\u2717"; // ✗
    default:
      return "\u2022"; // •
  }
}

/**
 * BenchmarkLiveFeed -- real-time event feed for a running benchmark.
 *
 * Subscribes to WebSocket messages (trajectory.event, benchmark.run.progress,
 * benchmark.task.started, benchmark.task.completed, autoagent.status) filtered
 * by run_id and renders:
 *   1. A progress header with bar, task count, cost, and elapsed timer
 *   2. Feature accordions when 2+ features detected (collapsible per-feature view)
 *   3. A virtualized, auto-scrolling event log
 */
export function BenchmarkLiveFeed(props: BenchmarkLiveFeedProps) {
  // ---- State ----
  const [events, setEvents] = createSignal<LiveFeedEvent[]>([]);
  const [progress, setProgress] = createSignal<BenchmarkLiveProgress | null>(null);
  const [features, setFeatures] = createSignal<Map<string, FeatureEntry>>(new Map());
  const [currentFeatureId, setCurrentFeatureId] = createSignal<string | null>(null);
  const [elapsed, setElapsed] = createSignal(0);
  const [autoScroll, setAutoScroll] = createSignal(true);
  const [expandedFeature, setExpandedFeature] = createSignal<string | null>(null);

  // ---- Elapsed timer ----
  createEffect(() => {
    const startTime = new Date(props.startedAt).getTime();
    const id = setInterval(() => setElapsed(Math.floor((Date.now() - startTime) / 1000)), 1000);
    onCleanup(() => clearInterval(id));
  });

  // ---- WebSocket subscription ----
  const { onMessage } = useWebSocket();
  // eslint-disable-next-line solid/reactivity -- imperative listener, props.runId stable for component lifetime
  const cleanup = onMessage((msg) => {
    if (msg.type === "trajectory.event") {
      const p = msg.payload as unknown as TrajectoryPayload;
      if (p.run_id !== props.runId) return;
      const evt: LiveFeedEvent = {
        id: crypto.randomUUID(),
        timestamp: Date.now(),
        run_id: p.run_id,
        project_id: p.project_id ?? "",
        event_type: p.event_type,
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
      setEvents((prev) => {
        const next = [...prev, evt];
        return next.length > MAX_EVENTS ? next.slice(next.length - MAX_EVENTS) : next;
      });
      // Track event under current feature
      const fId = currentFeatureId();
      if (fId) {
        setFeatures((prev) => {
          const f = prev.get(fId);
          if (!f) return prev;
          const next = new Map(prev);
          next.set(fId, {
            ...f,
            events: [...f.events, evt],
            cost: f.cost + (evt.cost_usd ?? 0),
            step: evt.step ?? f.step,
          });
          return next;
        });
      }
    }

    if (msg.type === "benchmark.run.progress") {
      const p = msg.payload as unknown as ProgressPayload;
      if (p.run_id !== props.runId) return;
      setProgress({
        completed_tasks: p.completed_tasks,
        total_tasks: p.total_tasks,
        avg_score: p.avg_score,
        total_cost_usd: p.total_cost_usd,
      });
    }

    if (msg.type === "benchmark.task.started") {
      const p = msg.payload as unknown as TaskStartedPayload;
      if (p.run_id !== props.runId) return;
      setFeatures((prev) => {
        const next = new Map(prev);
        if (!next.has(p.task_id)) {
          next.set(p.task_id, {
            id: p.task_id,
            name: p.task_name,
            status: "running",
            events: [],
            startedAt: Date.now(),
            cost: 0,
            step: 0,
          });
        }
        return next;
      });
      setCurrentFeatureId(p.task_id);
    }

    if (msg.type === "benchmark.task.completed") {
      const p = msg.payload as unknown as TaskCompletedPayload;
      if (p.run_id !== props.runId) return;
      setFeatures((prev) => {
        const next = new Map(prev);
        const existing = next.get(p.task_id);
        next.set(p.task_id, {
          id: p.task_id,
          name: p.task_name,
          status: "completed",
          events: existing?.events ?? [],
          startedAt: existing?.startedAt,
          cost: p.cost_usd,
          step: existing?.step ?? 0,
          score: p.score,
        });
        return next;
      });
    }

    if (msg.type === "autoagent.status") {
      const p = msg.payload as unknown as AutoAgentStatusPayload;
      if (p.current_feature_id) {
        setCurrentFeatureId(p.current_feature_id);
      }
    }
  });
  onCleanup(cleanup);

  // ---- Derived data ----
  const featureList = createMemo(() => Array.from(features().values()));
  const useAccordion = createMemo(() => featureList().length > 1);

  const displayedEvents = createMemo(() => {
    const ef = expandedFeature();
    if (useAccordion() && ef) {
      const f = features().get(ef);
      return f?.events ?? [];
    }
    return events();
  });

  const pct = createMemo(() => {
    const p = progress();
    if (!p || p.total_tasks === 0) return 0;
    return Math.round((p.completed_tasks / p.total_tasks) * 100);
  });

  const currentTaskRunning = createMemo(() => {
    const p = progress();
    return p && p.completed_tasks < p.total_tasks ? p : undefined;
  });

  // ---- Virtualizer ----
  let scrollContainerRef!: HTMLDivElement;

  const virtualizer = createVirtualizer({
    get count() {
      return displayedEvents().length;
    },
    getScrollElement: () => scrollContainerRef,
    estimateSize: () => 36,
    overscan: 10,
  });

  const scrollToEnd = () => {
    const count = displayedEvents().length;
    if (count > 0) {
      virtualizer.scrollToIndex(count - 1, { align: "end", behavior: "smooth" });
    }
  };

  createEffect(
    on(
      () => displayedEvents().length,
      (len) => {
        if (len > 0 && autoScroll()) {
          queueMicrotask(scrollToEnd);
        }
      },
    ),
  );

  const handleScroll = () => {
    if (!scrollContainerRef) return;
    const { scrollTop, scrollHeight, clientHeight } = scrollContainerRef;
    const atBottom = scrollTop >= scrollHeight - clientHeight - 50;
    if (!atBottom && autoScroll()) setAutoScroll(false);
    else if (atBottom && !autoScroll()) setAutoScroll(true);
  };

  const toggleFeature = (id: string) => {
    setExpandedFeature((prev) => (prev === id ? null : id));
    setAutoScroll(true);
  };

  return (
    <div class="mt-3 space-y-3">
      {/* ---- Progress Header ---- */}
      <div class="flex items-center gap-3 text-sm">
        <div class="flex-1">
          <Show
            when={progress()}
            fallback={
              <div class="h-2 w-full overflow-hidden rounded-full bg-cf-bg-secondary">
                <div
                  class="h-2 animate-pulse rounded-full bg-cf-accent"
                  style={{ width: "100%" }}
                />
              </div>
            }
          >
            <div class="h-2 w-full overflow-hidden rounded-full bg-cf-bg-secondary">
              <div
                class="h-2 rounded-full bg-cf-accent transition-all duration-300"
                style={{ width: `${pct()}%` }}
              />
            </div>
          </Show>
        </div>
        <Show when={progress()}>
          {(p) => (
            <span class="whitespace-nowrap text-cf-text-secondary">
              {p().completed_tasks}/{p().total_tasks} tasks ({pct()}%)
            </span>
          )}
        </Show>
        <Show when={progress()}>
          {(p) => (
            <span class="whitespace-nowrap text-cf-text-muted">
              ${p().total_cost_usd.toFixed(2)}
            </span>
          )}
        </Show>
        <span class="whitespace-nowrap font-mono text-cf-text-muted">
          {formatElapsed(elapsed())}
        </span>
      </div>

      {/* ---- Feature Accordion (when 2+ features detected) ---- */}
      <Show when={useAccordion()}>
        <div class="space-y-1">
          <For each={featureList()}>
            {(feature) => (
              <div>
                <button
                  type="button"
                  class={`flex w-full items-center gap-2 rounded px-2 py-1.5 text-xs transition hover:bg-cf-bg-secondary ${
                    expandedFeature() === feature.id
                      ? "bg-cf-bg-secondary text-cf-accent"
                      : "text-cf-text-secondary"
                  }`}
                  onClick={() => toggleFeature(feature.id)}
                >
                  <Show
                    when={feature.status !== "running"}
                    fallback={
                      <span class="inline-block h-3 w-3 shrink-0 animate-spin rounded-full border-2 border-cf-text-muted border-t-transparent" />
                    }
                  >
                    <span
                      class={
                        feature.status === "completed"
                          ? "text-cf-success-fg"
                          : feature.status === "failed"
                            ? "text-cf-danger-fg"
                            : "text-cf-text-muted"
                      }
                    >
                      {featureStatusIcon(feature.status)}
                    </span>
                  </Show>
                  <span class="flex-1 truncate text-left">{feature.name}</span>
                  <span class="text-cf-text-muted">step {feature.step}</span>
                  <Show when={feature.score}>
                    {(score) => <span class="font-mono">{score().toFixed(1)}</span>}
                  </Show>
                  <span class="text-cf-text-muted">${feature.cost.toFixed(4)}</span>
                  <Show when={feature.startedAt}>
                    {(startedAt) => (
                      <span class="font-mono text-cf-text-muted">
                        {/* Read elapsed() to tick every second */}
                        {(elapsed(), formatElapsed(Math.floor((Date.now() - startedAt()) / 1000)))}
                      </span>
                    )}
                  </Show>
                  <span class="text-cf-text-muted">
                    {expandedFeature() === feature.id ? "\u25B4" : "\u25BE"}
                  </span>
                </button>
              </div>
            )}
          </For>
        </div>
      </Show>

      {/* ---- Flat task list (when 0-1 features, fallback from accordion) ---- */}
      <Show when={!useAccordion() && (featureList().length > 0 || currentTaskRunning())}>
        <div class="space-y-1">
          <For each={featureList()}>
            {(feature) => (
              <div class="flex items-center gap-2 px-2 py-1 text-xs text-cf-text-secondary">
                <Show
                  when={feature.status !== "running"}
                  fallback={
                    <span class="inline-block h-3 w-3 shrink-0 animate-spin rounded-full border-2 border-cf-text-muted border-t-transparent" />
                  }
                >
                  <span
                    class={
                      feature.status === "completed" ? "text-cf-success-fg" : "text-cf-text-muted"
                    }
                  >
                    {featureStatusIcon(feature.status)}
                  </span>
                </Show>
                <span class="flex-1 truncate">{feature.name}</span>
                <Show when={feature.score}>
                  {(score) => <span class="font-mono">{score().toFixed(1)}</span>}
                </Show>
                <span class="text-cf-text-muted">${feature.cost.toFixed(4)}</span>
              </div>
            )}
          </For>
          <Show when={currentTaskRunning()}>
            {(p) => (
              <div class="flex items-center gap-2 px-2 py-1 text-xs text-cf-text-muted">
                <span class="inline-block h-3 w-3 animate-spin rounded-full border-2 border-cf-text-muted border-t-transparent" />
                <span>Task {p().completed_tasks + 1} running...</span>
              </div>
            )}
          </Show>
        </div>
      </Show>

      {/* ---- Virtualized Event Feed ---- */}
      <div class="relative">
        <Show when={useAccordion() ? expandedFeature() : null}>
          {(ef) => (
            <p class="mb-1 text-xs text-cf-text-muted">
              Events for: {features().get(ef())?.name ?? ef()}
            </p>
          )}
        </Show>
        <div
          ref={scrollContainerRef}
          onScroll={handleScroll}
          class="max-h-80 overflow-y-auto rounded border border-cf-border bg-cf-bg-primary"
        >
          <div
            style={{
              height: `${virtualizer.getTotalSize()}px`,
              width: "100%",
              position: "relative",
            }}
          >
            <For each={virtualizer.getVirtualItems()}>
              {(virtualRow) => {
                const evt = () => displayedEvents()[virtualRow.index];
                return (
                  <Show when={evt()}>
                    {(e) => {
                      const badge = () => eventBadge(e());
                      return (
                        <div
                          class="absolute left-0 top-0 flex w-full items-center gap-2 px-2 text-xs font-mono text-cf-text-muted"
                          style={{
                            height: `${virtualRow.size}px`,
                            transform: `translateY(${virtualRow.start}px)`,
                          }}
                        >
                          <span class="shrink-0 w-4 text-center">{eventIcon(e().event_type)}</span>
                          <span class="shrink-0 opacity-50">
                            {new Date(e().timestamp).toLocaleTimeString()}
                          </span>
                          <span class="min-w-0 flex-1 truncate">{renderEventText(e())}</span>
                          <Show when={badge()}>
                            {(b) => <span class={`shrink-0 font-bold ${b().cls}`}>{b().text}</span>}
                          </Show>
                        </div>
                      );
                    }}
                  </Show>
                );
              }}
            </For>
          </div>
        </div>

        {/* Scroll-to-bottom button */}
        <Show when={!autoScroll()}>
          <button
            type="button"
            class="sticky bottom-2 left-1/2 z-10 -translate-x-1/2 rounded-full bg-cf-accent px-3 py-1 text-xs text-white shadow transition hover:opacity-90"
            onClick={() => {
              setAutoScroll(true);
              scrollToEnd();
            }}
          >
            {"\u2193"} Scroll to bottom
          </button>
        </Show>
      </div>

      {/* ---- Empty state ---- */}
      <Show when={displayedEvents().length === 0}>
        <p class="py-2 text-center text-xs text-cf-text-muted">Waiting for events...</p>
      </Show>
    </div>
  );
}
