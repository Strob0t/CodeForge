import { createVirtualizer } from "@tanstack/solid-virtual";
import { createEffect, createMemo, createSignal, For, on, onCleanup, Show } from "solid-js";

import type { BenchmarkLiveProgress, LiveFeedEvent } from "~/api/types";
import { useWebSocket } from "~/components/WebSocketProvider";

interface BenchmarkLiveFeedProps {
  runId: string;
  startedAt: string;
}

interface CompletedTask {
  name: string;
  score: number;
  cost: number;
}

/** Format seconds as MM:SS or HH:MM:SS. */
function formatElapsed(totalSeconds: number): string {
  const h = Math.floor(totalSeconds / 3600);
  const m = Math.floor((totalSeconds % 3600) / 60);
  const s = totalSeconds % 60;
  const mm = String(m).padStart(2, "0");
  const ss = String(s).padStart(2, "0");
  if (h > 0) {
    return `${String(h).padStart(2, "0")}:${mm}:${ss}`;
  }
  return `${mm}:${ss}`;
}

/** Truncate a string to a maximum length, appending ellipsis if needed. */
function truncate(text: string, maxLen: number): string {
  if (text.length <= maxLen) return text;
  return text.slice(0, maxLen) + "\u2026";
}

/**
 * BenchmarkLiveFeed -- real-time event feed for a running benchmark.
 *
 * Subscribes to WebSocket messages (trajectory.event, benchmark.run.progress,
 * benchmark.task.completed) filtered by run_id and renders:
 *   1. A progress header with bar, task count, cost, and elapsed timer
 *   2. A feature accordion showing completed / in-progress tasks
 *   3. A virtualized, auto-scrolling event log
 */
export function BenchmarkLiveFeed(props: BenchmarkLiveFeedProps) {
  // ---- State ----
  const [events, setEvents] = createSignal<LiveFeedEvent[]>([]);
  const [progress, setProgress] = createSignal<BenchmarkLiveProgress | null>(null);
  const [completedTasks, setCompletedTasks] = createSignal<Map<string, CompletedTask>>(new Map());
  const [elapsed, setElapsed] = createSignal(0);
  const [autoScroll, setAutoScroll] = createSignal(true);
  const [filterTask, setFilterTask] = createSignal<string | null>(null);

  // ---- Elapsed timer ----
  // Wrap in createEffect so Solid tracks props.startedAt reactively
  createEffect(() => {
    const startTime = new Date(props.startedAt).getTime();
    const id = setInterval(() => setElapsed(Math.floor((Date.now() - startTime) / 1000)), 1000);
    onCleanup(() => clearInterval(id));
  });

  // ---- WebSocket subscription ----
  // The onMessage callback is an imperative event listener, not a tracked scope.
  // Props accessed inside are stable for the component lifetime (runId never changes).
  const { onMessage } = useWebSocket();
  // eslint-disable-next-line solid/reactivity
  const cleanup = onMessage((msg) => {
    if (msg.type === "trajectory.event") {
      const p = msg.payload;
      if ((p.run_id as string) !== props.runId) return;
      const evt: LiveFeedEvent = {
        id: crypto.randomUUID(),
        timestamp: Date.now(),
        run_id: p.run_id as string,
        project_id: (p.project_id as string) ?? "",
        event_type: p.event_type as string,
        tool_name: p.tool_name as string | undefined,
        model: p.model as string | undefined,
        input: p.input as string | undefined,
        output: p.output as string | undefined,
        success: p.success as boolean | undefined,
        step: p.step as number | undefined,
        cost_usd: p.cost_usd as number | undefined,
        tokens_in: p.tokens_in as number | undefined,
        tokens_out: p.tokens_out as number | undefined,
      };
      setEvents((prev) => [...prev, evt]);
    }
    if (msg.type === "benchmark.run.progress" && (msg.payload.run_id as string) === props.runId) {
      setProgress({
        completed_tasks: msg.payload.completed_tasks as number,
        total_tasks: msg.payload.total_tasks as number,
        avg_score: msg.payload.avg_score as number,
        total_cost_usd: msg.payload.total_cost_usd as number,
      });
    }
    if (msg.type === "benchmark.task.completed" && (msg.payload.run_id as string) === props.runId) {
      const p = msg.payload;
      setCompletedTasks((prev) => {
        const next = new Map(prev);
        next.set(p.task_id as string, {
          name: p.task_name as string,
          score: p.score as number,
          cost: p.cost_usd as number,
        });
        return next;
      });
    }
  });
  onCleanup(cleanup);

  // ---- Derived data ----
  const filteredEvents = createMemo(() => {
    const task = filterTask();
    if (!task) return events();
    // When a task filter is active, show only events that mention the task name
    // in the output or input fields (heuristic -- we don't have a task_id on trajectory events)
    return events().filter(
      (e) => e.output?.includes(task) || e.input?.includes(task) || e.tool_name?.includes(task),
    );
  });

  const completedTaskList = createMemo(() => Array.from(completedTasks().values()));

  const pct = createMemo(() => {
    const p = progress();
    if (!p || p.total_tasks === 0) return 0;
    return Math.round((p.completed_tasks / p.total_tasks) * 100);
  });

  // ---- Virtualizer ----
  let scrollContainerRef!: HTMLDivElement;

  const virtualizer = createVirtualizer({
    get count() {
      return filteredEvents().length;
    },
    getScrollElement: () => scrollContainerRef,
    estimateSize: () => 36,
    overscan: 10,
  });

  // Auto-scroll on new events
  const scrollToEnd = () => {
    const count = filteredEvents().length;
    if (count > 0) {
      virtualizer.scrollToIndex(count - 1, { align: "end", behavior: "smooth" });
    }
  };

  // Watch events length to trigger auto-scroll
  createEffect(
    on(
      () => filteredEvents().length,
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
    if (!atBottom && autoScroll()) {
      setAutoScroll(false);
    } else if (atBottom && !autoScroll()) {
      setAutoScroll(true);
    }
  };

  // ---- Event row renderer ----
  function renderEventRow(evt: LiveFeedEvent): string {
    switch (evt.event_type) {
      case "agent.tool_called":
        return `>_ ${evt.tool_name ?? "tool"}: ${truncate(evt.output ?? "", 120)}`;
      case "agent.step_done":
        return `# Step ${evt.step ?? "?"} | ${evt.model ?? "?"} | ${evt.tokens_in ?? 0}+${evt.tokens_out ?? 0} tok | $${(evt.cost_usd ?? 0).toFixed(4)}`;
      case "agent.finished":
        return `Finished | $${(evt.cost_usd ?? 0).toFixed(4)} total`;
      default:
        return evt.event_type;
    }
  }

  function eventBadge(evt: LiveFeedEvent): { text: string; class: string } | null {
    if (evt.event_type !== "agent.tool_called") return null;
    if (evt.success === true) return { text: "OK", class: "text-green-400" };
    if (evt.success === false) return { text: "FAIL", class: "text-cf-danger-fg" };
    return null;
  }

  return (
    <div class="mt-3 space-y-3">
      {/* ---- Progress Header ---- */}
      <div class="flex items-center gap-3 text-sm">
        <div class="flex-1">
          <Show
            when={progress()}
            fallback={
              <div class="h-2 w-full overflow-hidden rounded-full bg-cf-bg-secondary">
                <div class="h-2 animate-pulse rounded-full bg-blue-500" style={{ width: "100%" }} />
              </div>
            }
          >
            <div class="h-2 w-full overflow-hidden rounded-full bg-cf-bg-secondary">
              <div
                class="h-2 rounded-full bg-blue-500 transition-all duration-300"
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

      {/* ---- Feature Accordion ---- */}
      <Show when={completedTaskList().length > 0 || (progress()?.total_tasks ?? 0) > 0}>
        <div class="space-y-1">
          <For each={completedTaskList()}>
            {(task) => (
              <button
                type="button"
                class={`flex w-full items-center gap-2 rounded px-2 py-1 text-xs transition hover:bg-cf-bg-secondary ${
                  filterTask() === task.name
                    ? "bg-cf-bg-secondary text-cf-accent"
                    : "text-cf-text-secondary"
                }`}
                onClick={() => setFilterTask(filterTask() === task.name ? null : task.name)}
              >
                <span class="text-green-500">{"\u2713"}</span>
                <span class="flex-1 truncate text-left">{task.name}</span>
                <span class="font-mono">{task.score.toFixed(1)}</span>
                <span class="text-cf-text-muted">${task.cost.toFixed(4)}</span>
              </button>
            )}
          </For>
          {/* Show current (in-progress) task indicator */}
          <Show
            when={(() => {
              const p = progress();
              return p && p.completed_tasks < p.total_tasks ? p : undefined;
            })()}
          >
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
                const evt = () => filteredEvents()[virtualRow.index];
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
                          <span class="shrink-0 text-cf-text-muted opacity-50">
                            {new Date(e().timestamp).toLocaleTimeString()}
                          </span>
                          <span class="min-w-0 flex-1 truncate">{renderEventRow(e())}</span>
                          <Show when={badge()}>
                            {(b) => (
                              <span class={`shrink-0 font-bold ${b().class}`}>{b().text}</span>
                            )}
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
      <Show when={filteredEvents().length === 0}>
        <p class="py-2 text-center text-xs text-cf-text-muted">Waiting for events...</p>
      </Show>
    </div>
  );
}
