import { createResource, createSignal, For, onCleanup, Show } from "solid-js";

import { api } from "~/api/client";
import type { AgentEvent, AgentEventType, TrajectorySummary } from "~/api/types";
import { DiffPreview } from "~/components/DiffPreview";
import { useI18n } from "~/i18n";

interface TrajectoryPanelProps {
  runId: string;
}

const EVENT_COLORS: Record<string, string> = {
  "agent.started": "bg-blue-400",
  "agent.step_done": "bg-blue-300",
  "agent.tool_called": "bg-yellow-400",
  "agent.tool_result": "bg-yellow-300",
  "agent.finished": "bg-green-400",
  "agent.error": "bg-red-400",
};

const EVENT_TYPES: AgentEventType[] = [
  "agent.started",
  "agent.step_done",
  "agent.tool_called",
  "agent.tool_result",
  "agent.finished",
  "agent.error",
];

const PLAYBACK_SPEEDS = [0.5, 1, 2, 4] as const;

/** Render tool call payload with structured sections */
/** Check if a string looks like a unified diff */
function looksLikeDiff(text: string): boolean {
  return (
    (text.includes("--- ") && text.includes("+++ ")) ||
    text.includes("@@ ") ||
    text.startsWith("diff --git")
  );
}

/** Extract diff content from payload (check .diff, .patch, .output fields) */
function extractDiff(payload: Record<string, unknown>): string | null {
  for (const key of ["diff", "patch", "output"]) {
    const val = payload[key];
    if (typeof val === "string" && looksLikeDiff(val)) {
      return val;
    }
  }
  return null;
}

function EventDetail(props: { event: AgentEvent }) {
  const { t } = useI18n();
  const payload = () => props.event.payload;
  const isToolCall = () => props.event.type === "agent.tool_called";
  const isToolResult = () => props.event.type === "agent.tool_result";
  const diffContent = () => extractDiff(payload());

  return (
    <div class="space-y-2">
      <Show when={isToolCall()}>
        <div>
          <span class="text-xs font-medium text-gray-500 dark:text-gray-400">
            {t("trajectory.tool")}
          </span>
          <span class="ml-2 rounded bg-yellow-100 px-1.5 py-0.5 font-mono text-xs text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-300">
            {(payload().tool as string) ?? "?"}
          </span>
        </div>
        <Show when={payload().input}>
          <div>
            <p class="mb-1 text-xs font-medium text-gray-500 dark:text-gray-400">
              {t("trajectory.input")}
            </p>
            <pre class="max-h-40 overflow-auto rounded bg-gray-100 p-2 text-xs text-gray-700 dark:bg-gray-900 dark:text-gray-300">
              {typeof payload().input === "string"
                ? (payload().input as string)
                : JSON.stringify(payload().input, null, 2)}
            </pre>
          </div>
        </Show>
      </Show>

      <Show when={isToolResult()}>
        <Show when={payload().tool}>
          <div>
            <span class="text-xs font-medium text-gray-500 dark:text-gray-400">
              {t("trajectory.tool")}
            </span>
            <span class="ml-2 rounded bg-yellow-100 px-1.5 py-0.5 font-mono text-xs text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-300">
              {payload().tool as string}
            </span>
          </div>
        </Show>

        {/* Diff preview for tool results with unified diff content */}
        <Show when={diffContent()}>
          {(diff) => (
            <div>
              <p class="mb-1 text-xs font-medium text-gray-500 dark:text-gray-400">
                {t("diff.title")}
              </p>
              <DiffPreview diff={diff()} maxHeight={300} />
            </div>
          )}
        </Show>

        {/* Regular output (only if no diff was detected) */}
        <Show when={!diffContent() && payload().output}>
          <div>
            <p class="mb-1 text-xs font-medium text-gray-500 dark:text-gray-400">
              {t("trajectory.output")}
            </p>
            <pre class="max-h-40 overflow-auto rounded bg-gray-100 p-2 text-xs text-gray-700 dark:bg-gray-900 dark:text-gray-300">
              {typeof payload().output === "string"
                ? (payload().output as string)
                : JSON.stringify(payload().output, null, 2)}
            </pre>
          </div>
        </Show>
        <Show when={payload().error}>
          <div>
            <p class="mb-1 text-xs font-medium text-red-500 dark:text-red-400">
              {t("trajectory.errorOutput")}
            </p>
            <pre class="max-h-32 overflow-auto rounded bg-red-50 p-2 text-xs text-red-700 dark:bg-red-900/20 dark:text-red-300">
              {payload().error as string}
            </pre>
          </div>
        </Show>
      </Show>

      {/* Diff preview for non-tool events (e.g. delivery) */}
      <Show when={!isToolCall() && !isToolResult() && diffContent()}>
        {(diff) => (
          <div>
            <p class="mb-1 text-xs font-medium text-gray-500 dark:text-gray-400">
              {t("diff.title")}
            </p>
            <DiffPreview diff={diff()} maxHeight={300} />
          </div>
        )}
      </Show>

      <Show when={!isToolCall() && !isToolResult() && !diffContent()}>
        <pre class="max-h-40 overflow-auto whitespace-pre-wrap text-xs text-gray-700 dark:text-gray-300">
          {JSON.stringify(payload(), null, 2)}
        </pre>
      </Show>
    </div>
  );
}

export default function TrajectoryPanel(props: TrajectoryPanelProps) {
  const { t, tp, fmt } = useI18n();
  const [typeFilter, setTypeFilter] = createSignal("");
  const [cursor, setCursor] = createSignal("");

  // Replay mode state
  const [replayMode, setReplayMode] = createSignal(false);
  const [replayIndex, setReplayIndex] = createSignal(0);
  const [playing, setPlaying] = createSignal(false);
  const [speedIdx, setSpeedIdx] = createSignal(1); // index into PLAYBACK_SPEEDS
  let playTimer: ReturnType<typeof setInterval> | undefined;

  const [trajectory, { refetch }] = createResource(
    () => ({ runId: props.runId, types: typeFilter(), cursor: cursor() }),
    (opts) =>
      api.trajectory.get(opts.runId, {
        types: opts.types || undefined,
        cursor: opts.cursor || undefined,
        limit: 50,
      }),
  );

  const [expandedId, setExpandedId] = createSignal<string | null>(null);

  const stats = (): TrajectorySummary | undefined => trajectory()?.stats;
  const events = (): AgentEvent[] => trajectory()?.events ?? [];
  const currentEvent = (): AgentEvent | undefined => events()[replayIndex()];

  const stopPlayback = () => {
    setPlaying(false);
    if (playTimer !== undefined) {
      clearInterval(playTimer);
      playTimer = undefined;
    }
  };

  const startPlayback = () => {
    stopPlayback();
    const evts = events();
    if (evts.length === 0) return;
    setPlaying(true);
    const interval = 1500 / PLAYBACK_SPEEDS[speedIdx()];
    playTimer = setInterval(() => {
      setReplayIndex((prev) => {
        const next = prev + 1;
        if (next >= evts.length) {
          stopPlayback();
          return prev;
        }
        return next;
      });
    }, interval);
  };

  const togglePlayback = () => {
    if (playing()) {
      stopPlayback();
    } else {
      startPlayback();
    }
  };

  const cycleSpeed = () => {
    const next = (speedIdx() + 1) % PLAYBACK_SPEEDS.length;
    setSpeedIdx(next);
    if (playing()) {
      startPlayback(); // restart with new speed
    }
  };

  const stepPrev = () => {
    stopPlayback();
    setReplayIndex((prev) => Math.max(0, prev - 1));
  };

  const stepNext = () => {
    stopPlayback();
    setReplayIndex((prev) => Math.min(events().length - 1, prev + 1));
  };

  const enterReplay = () => {
    setReplayMode(true);
    setReplayIndex(0);
    stopPlayback();
  };

  const exitReplay = () => {
    stopPlayback();
    setReplayMode(false);
  };

  onCleanup(stopPlayback);

  const handleExport = () => {
    window.open(api.trajectory.exportUrl(props.runId), "_blank");
  };

  const handleNextPage = () => {
    const page = trajectory();
    if (page?.cursor) {
      setCursor(page.cursor);
    }
  };

  const handlePrevPage = () => {
    setCursor("");
    refetch();
  };

  const toggleExpand = (id: string) => {
    setExpandedId((prev) => (prev === id ? null : id));
  };

  return (
    <div class="rounded-lg border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-gray-800">
      <div class="mb-3 flex items-center justify-between">
        <h3 class="text-lg font-semibold">{t("trajectory.title")}</h3>
        <div class="flex items-center gap-2">
          <button
            type="button"
            class={`rounded px-3 py-1 text-xs ${
              replayMode()
                ? "bg-blue-600 text-white hover:bg-blue-700"
                : "bg-gray-100 hover:bg-gray-200 dark:bg-gray-700 dark:hover:bg-gray-600"
            }`}
            onClick={() => (replayMode() ? exitReplay() : enterReplay())}
            aria-pressed={replayMode()}
          >
            {replayMode() ? t("trajectory.exitReplay") : t("trajectory.replay")}
          </button>
          <button
            class="rounded bg-gray-100 px-3 py-1 text-xs hover:bg-gray-200 dark:bg-gray-700 dark:hover:bg-gray-600"
            onClick={handleExport}
          >
            {t("trajectory.exportJson")}
          </button>
        </div>
      </div>

      {/* Stats Summary */}
      <Show when={stats()}>
        {(s) => (
          <div class="mb-4 flex flex-wrap gap-4 rounded bg-gray-50 p-3 text-sm dark:bg-gray-700">
            <span>{tp("trajectory.events", s().total_events)}</span>
            <span>
              <span class="text-gray-500 dark:text-gray-400">{t("trajectory.duration")}</span>{" "}
              {fmt.duration(s().duration_ms)}
            </span>
            <span>{tp("trajectory.toolCalls", s().tool_call_count)}</span>
            <span class={s().error_count > 0 ? "text-red-600 dark:text-red-400" : ""}>
              {tp("trajectory.errors", s().error_count)}
            </span>
          </div>
        )}
      </Show>

      {/* Filters (hidden in replay mode) */}
      <Show when={!replayMode()}>
        <div class="mb-4 flex flex-wrap gap-2">
          <button
            type="button"
            class={`rounded px-2 py-1 text-xs ${!typeFilter() ? "bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400" : "bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-gray-700 dark:text-gray-400 dark:hover:bg-gray-600"}`}
            onClick={() => {
              setTypeFilter("");
              setCursor("");
            }}
            aria-pressed={!typeFilter()}
            aria-label={t("trajectory.filterAllAria")}
          >
            {t("trajectory.filterAll")}
          </button>
          <For each={EVENT_TYPES}>
            {(evType) => (
              <button
                type="button"
                class={`rounded px-2 py-1 text-xs ${typeFilter() === evType ? "bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400" : "bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-gray-700 dark:text-gray-400 dark:hover:bg-gray-600"}`}
                onClick={() => {
                  setTypeFilter(evType);
                  setCursor("");
                }}
                aria-pressed={typeFilter() === evType}
                aria-label={t("trajectory.filterAria", { type: evType.replace("agent.", "") })}
              >
                {evType.replace("agent.", "")}
              </button>
            )}
          </For>
        </div>
      </Show>

      {/* Replay Mode */}
      <Show when={replayMode() && events().length > 0}>
        {/* Replay controls */}
        <div class="mb-3 rounded bg-gray-50 p-3 dark:bg-gray-700">
          {/* Scrubber bar */}
          <div class="mb-2">
            <input
              type="range"
              min="0"
              max={Math.max(0, events().length - 1)}
              value={replayIndex()}
              onInput={(e) => {
                stopPlayback();
                setReplayIndex(parseInt(e.currentTarget.value, 10));
              }}
              class="w-full accent-blue-600"
              aria-label={t("trajectory.scrubberAria")}
            />
            {/* Mini timeline dots */}
            <div class="mt-1 flex gap-px">
              <For each={events()}>
                {(ev, i) => (
                  <div
                    class={`h-1 flex-1 rounded-sm transition-colors ${
                      i() <= replayIndex()
                        ? (EVENT_COLORS[ev.type] ?? "bg-gray-400")
                        : "bg-gray-200 dark:bg-gray-600"
                    }`}
                  />
                )}
              </For>
            </div>
          </div>

          {/* Control buttons */}
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-2">
              <button
                type="button"
                class="rounded bg-gray-200 px-2 py-1 text-xs font-medium hover:bg-gray-300 disabled:opacity-50 dark:bg-gray-600 dark:hover:bg-gray-500"
                onClick={stepPrev}
                disabled={replayIndex() <= 0}
                aria-label={t("trajectory.prevStep")}
              >
                {"\u25C0"}
              </button>
              <button
                type="button"
                class={`rounded px-3 py-1 text-xs font-medium text-white ${
                  playing()
                    ? "bg-yellow-500 hover:bg-yellow-600"
                    : "bg-green-600 hover:bg-green-700"
                }`}
                onClick={togglePlayback}
                aria-label={playing() ? t("trajectory.pauseReplay") : t("trajectory.playReplay")}
              >
                {playing() ? "\u23F8" : "\u25B6"}
              </button>
              <button
                type="button"
                class="rounded bg-gray-200 px-2 py-1 text-xs font-medium hover:bg-gray-300 disabled:opacity-50 dark:bg-gray-600 dark:hover:bg-gray-500"
                onClick={stepNext}
                disabled={replayIndex() >= events().length - 1}
                aria-label={t("trajectory.nextStep")}
              >
                {"\u25B6"}
              </button>
              <button
                type="button"
                class="rounded bg-gray-200 px-2 py-1 text-xs hover:bg-gray-300 dark:bg-gray-600 dark:hover:bg-gray-500"
                onClick={cycleSpeed}
                title={t("trajectory.speedLabel")}
              >
                {PLAYBACK_SPEEDS[speedIdx()]}x
              </button>
            </div>
            <span class="text-xs text-gray-500 dark:text-gray-400">
              {t("trajectory.stepOf", {
                current: String(replayIndex() + 1),
                total: String(events().length),
              })}
            </span>
          </div>
        </div>

        {/* Current event detail */}
        <Show when={currentEvent()}>
          {(ev) => (
            <div
              class="rounded border border-gray-200 dark:border-gray-700"
              aria-live="polite"
              aria-label={t("trajectory.eventAria", {
                type: ev().type,
                time: fmt.time(ev().created_at),
              })}
            >
              <div class="flex items-center gap-2 border-b border-gray-100 px-4 py-3 dark:border-gray-700">
                <span
                  class={`h-3 w-3 rounded-full ${EVENT_COLORS[ev().type] ?? "bg-gray-300"}`}
                  aria-hidden="true"
                />
                <span class="font-mono text-sm font-medium text-gray-700 dark:text-gray-300">
                  {ev().type}
                </span>
                <span class="flex-1" />
                <span class="text-xs text-gray-400 dark:text-gray-500">
                  {fmt.time(ev().created_at)}
                </span>
                <span class="text-xs text-gray-400 dark:text-gray-500">v{ev().version}</span>
              </div>
              <div class="px-4 py-3">
                <EventDetail event={ev()} />
              </div>
            </div>
          )}
        </Show>
      </Show>

      {/* Browse Mode (existing timeline) */}
      <Show when={!replayMode()}>
        <Show
          when={trajectory()}
          fallback={<p class="text-sm text-gray-500 dark:text-gray-400">{t("common.loading")}</p>}
        >
          <div class="space-y-1">
            <For each={events()}>
              {(ev: AgentEvent) => (
                <div
                  class={`cursor-pointer rounded border hover:border-gray-200 dark:hover:border-gray-600 ${
                    expandedId() === ev.id
                      ? "border-blue-200 dark:border-blue-700"
                      : "border-gray-100 dark:border-gray-700"
                  }`}
                  role="button"
                  tabIndex={0}
                  aria-expanded={expandedId() === ev.id}
                  aria-label={t("trajectory.eventAria", {
                    type: ev.type,
                    time: fmt.time(ev.created_at),
                  })}
                  onClick={() => toggleExpand(ev.id)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" || e.key === " ") {
                      e.preventDefault();
                      toggleExpand(ev.id);
                    }
                  }}
                >
                  <div class="flex items-center gap-2 px-3 py-2">
                    <span
                      class={`h-2.5 w-2.5 rounded-full ${EVENT_COLORS[ev.type] ?? "bg-gray-300"}`}
                      aria-hidden="true"
                    />
                    <span class="font-mono text-xs text-gray-600 dark:text-gray-400">
                      {ev.type}
                    </span>
                    <span class="flex-1" />
                    <span class="text-xs text-gray-400 dark:text-gray-500">
                      {fmt.time(ev.created_at)}
                    </span>
                    <span class="text-xs text-gray-400 dark:text-gray-500">v{ev.version}</span>
                  </div>

                  <Show when={expandedId() === ev.id}>
                    <div class="border-t border-gray-100 bg-gray-50 px-3 py-2 dark:border-gray-700 dark:bg-gray-700">
                      <EventDetail event={ev} />
                    </div>
                  </Show>
                </div>
              )}
            </For>
          </div>

          {/* Pagination */}
          <div class="mt-3 flex items-center justify-between text-sm">
            <span class="text-xs text-gray-500 dark:text-gray-400">
              {t("trajectory.total", { n: trajectory()?.total ?? 0 })}
            </span>
            <div class="flex gap-2">
              <Show when={cursor()}>
                <button
                  class="rounded bg-gray-100 px-3 py-1 text-xs hover:bg-gray-200 dark:bg-gray-700 dark:hover:bg-gray-600"
                  onClick={handlePrevPage}
                >
                  {t("trajectory.first")}
                </button>
              </Show>
              <Show when={trajectory()?.has_more}>
                <button
                  class="rounded bg-gray-100 px-3 py-1 text-xs hover:bg-gray-200 dark:bg-gray-700 dark:hover:bg-gray-600"
                  onClick={handleNextPage}
                >
                  {t("trajectory.next")}
                </button>
              </Show>
            </div>
          </div>
        </Show>
      </Show>
    </div>
  );
}
