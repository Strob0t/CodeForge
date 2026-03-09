import { createResource, createSignal, For, onCleanup, Show } from "solid-js";

import { api } from "~/api/client";
import type { AgentEvent, AgentEventType, Session, TrajectorySummary } from "~/api/types";
import { DiffPreview } from "~/components/DiffPreview";
import { useToast } from "~/components/Toast";
import { useI18n } from "~/i18n";
import { Button, Card } from "~/ui";

interface TrajectoryPanelProps {
  runId: string;
  conversationId?: string;
  onSessionCreated?: (session: Session) => void;
}

const EVENT_COLORS: Record<string, string> = {
  "agent.started": "bg-cf-info",
  "agent.step_done": "bg-cf-accent",
  "agent.tool_called": "bg-cf-warning",
  "agent.tool_result": "bg-cf-warning",
  "agent.finished": "bg-cf-success",
  "agent.error": "bg-cf-danger",
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
          <span class="text-xs font-medium text-cf-text-tertiary">{t("trajectory.tool")}</span>
          <span class="ml-2 rounded bg-cf-warning-bg px-1.5 py-0.5 font-mono text-xs text-cf-warning-fg">
            {(payload().tool as string) ?? "?"}
          </span>
        </div>
        <Show when={payload().input}>
          <div>
            <p class="mb-1 text-xs font-medium text-cf-text-tertiary">{t("trajectory.input")}</p>
            <pre class="max-h-40 overflow-auto rounded-cf-sm bg-cf-bg-inset p-2 text-xs text-cf-text-secondary">
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
            <span class="text-xs font-medium text-cf-text-tertiary">{t("trajectory.tool")}</span>
            <span class="ml-2 rounded bg-cf-warning-bg px-1.5 py-0.5 font-mono text-xs text-cf-warning-fg">
              {payload().tool as string}
            </span>
          </div>
        </Show>

        {/* Diff preview for tool results with unified diff content */}
        <Show when={diffContent()}>
          {(diff) => (
            <div>
              <p class="mb-1 text-xs font-medium text-cf-text-tertiary">{t("diff.title")}</p>
              <DiffPreview diff={diff()} maxHeight={300} />
            </div>
          )}
        </Show>

        {/* Regular output (only if no diff was detected) */}
        <Show when={!diffContent() && payload().output}>
          <div>
            <p class="mb-1 text-xs font-medium text-cf-text-tertiary">{t("trajectory.output")}</p>
            <pre class="max-h-40 overflow-auto rounded-cf-sm bg-cf-bg-inset p-2 text-xs text-cf-text-secondary">
              {typeof payload().output === "string"
                ? (payload().output as string)
                : JSON.stringify(payload().output, null, 2)}
            </pre>
          </div>
        </Show>
        <Show when={payload().error}>
          <div>
            <p class="mb-1 text-xs font-medium text-cf-danger-fg">{t("trajectory.errorOutput")}</p>
            <pre class="max-h-32 overflow-auto rounded-cf-sm bg-cf-danger-bg p-2 text-xs text-cf-danger-fg">
              {payload().error as string}
            </pre>
          </div>
        </Show>
      </Show>

      {/* Diff preview for non-tool events (e.g. delivery) */}
      <Show when={!isToolCall() && !isToolResult() && diffContent()}>
        {(diff) => (
          <div>
            <p class="mb-1 text-xs font-medium text-cf-text-tertiary">{t("diff.title")}</p>
            <DiffPreview diff={diff()} maxHeight={300} />
          </div>
        )}
      </Show>

      <Show when={!isToolCall() && !isToolResult() && !diffContent()}>
        <pre class="max-h-40 overflow-auto whitespace-pre-wrap text-xs text-cf-text-secondary">
          {JSON.stringify(payload(), null, 2)}
        </pre>
      </Show>
    </div>
  );
}

export default function TrajectoryPanel(props: TrajectoryPanelProps) {
  const { t, tp, fmt } = useI18n();
  const { show: toast } = useToast();
  const [typeFilter, setTypeFilter] = createSignal("");
  const [cursor, setCursor] = createSignal("");
  const [forking, setForking] = createSignal(false);
  const [rewinding, setRewinding] = createSignal(false);
  const [resuming, setResuming] = createSignal(false);
  const [rewindConfirmId, setRewindConfirmId] = createSignal<string | null>(null);

  const handleFork = async (eventId: string) => {
    setForking(true);
    try {
      const session = await api.runs.fork(props.runId, { from_event_id: eventId });
      toast("success", t("session.forkSuccess"));
      props.onSessionCreated?.(session);
    } catch {
      toast("error", t("session.forkFailed"));
    } finally {
      setForking(false);
    }
  };

  const handleRewind = async (eventId: string) => {
    setRewinding(true);
    setRewindConfirmId(null);
    try {
      const session = await api.runs.rewind(props.runId, { to_event_id: eventId });
      toast("success", `${t("session.rewindSuccess")} (${session.id.slice(0, 8)})`);
      props.onSessionCreated?.(session);
    } catch {
      toast("error", t("session.rewindFailed"));
    } finally {
      setRewinding(false);
    }
  };

  /** Request rewind with confirmation */
  const requestRewind = (eventId: string) => {
    setRewindConfirmId(eventId);
  };
  const cancelRewind = () => setRewindConfirmId(null);

  const handleResume = async () => {
    setResuming(true);
    try {
      const session = await api.runs.resume(props.runId);
      toast("success", t("session.resumeSuccess"));
      props.onSessionCreated?.(session);
    } catch {
      toast("error", t("session.resumeFailed"));
    } finally {
      setResuming(false);
    }
  };

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
    <Card>
      <Card.Header>
        <div class="flex items-center justify-between">
          <h3 class="text-lg font-semibold">{t("trajectory.title")}</h3>
          <div class="flex items-center gap-2">
            <Button
              variant={replayMode() ? "primary" : "secondary"}
              size="sm"
              onClick={() => (replayMode() ? exitReplay() : enterReplay())}
              aria-pressed={replayMode()}
            >
              {replayMode() ? t("trajectory.exitReplay") : t("trajectory.replay")}
            </Button>
            <Button variant="secondary" size="sm" onClick={handleExport}>
              {t("trajectory.exportJson")}
            </Button>
            <Button
              variant="secondary"
              size="sm"
              onClick={() => void handleResume()}
              disabled={resuming()}
              aria-label={t("session.resume")}
            >
              {resuming() ? "..." : t("session.resume")}
            </Button>
          </div>
        </div>
      </Card.Header>

      <Card.Body>
        {/* Stats Summary */}
        <Show when={stats()}>
          {(s) => (
            <div class="mb-4 flex flex-wrap gap-4 rounded-cf-sm bg-cf-bg-inset p-3 text-sm">
              <span>{tp("trajectory.events", s().total_events)}</span>
              <span>
                <span class="text-cf-text-tertiary">{t("trajectory.duration")}</span>{" "}
                {fmt.duration(s().duration_ms)}
              </span>
              <span>{tp("trajectory.toolCalls", s().tool_call_count)}</span>
              <span class={s().error_count > 0 ? "text-cf-danger-fg" : ""}>
                {tp("trajectory.errors", s().error_count)}
              </span>
            </div>
          )}
        </Show>

        {/* Filters (hidden in replay mode) */}
        <Show when={!replayMode()}>
          <div class="mb-4 flex flex-wrap gap-2">
            <Button
              variant={!typeFilter() ? "primary" : "secondary"}
              size="sm"
              onClick={() => {
                setTypeFilter("");
                setCursor("");
              }}
              aria-pressed={!typeFilter()}
              aria-label={t("trajectory.filterAllAria")}
            >
              {t("trajectory.filterAll")}
            </Button>
            <For each={EVENT_TYPES}>
              {(evType) => (
                <Button
                  variant={typeFilter() === evType ? "primary" : "secondary"}
                  size="sm"
                  onClick={() => {
                    setTypeFilter(evType);
                    setCursor("");
                  }}
                  aria-pressed={typeFilter() === evType}
                  aria-label={t("trajectory.filterAria", { type: evType.replace("agent.", "") })}
                >
                  {evType.replace("agent.", "")}
                </Button>
              )}
            </For>
          </div>
        </Show>

        {/* Replay Mode */}
        <Show when={replayMode() && events().length > 0}>
          {/* Replay controls */}
          <div class="mb-3 rounded-cf-sm bg-cf-bg-inset p-3">
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
                class="w-full accent-cf-accent"
                aria-label={t("trajectory.scrubberAria")}
              />
              {/* Mini timeline dots */}
              <div class="mt-1 flex gap-px">
                <For each={events()}>
                  {(ev, i) => (
                    <div
                      class={`h-1 flex-1 rounded-sm transition-colors ${
                        i() <= replayIndex()
                          ? (EVENT_COLORS[ev.type] ?? "bg-cf-text-muted")
                          : "bg-cf-bg-inset"
                      }`}
                    />
                  )}
                </For>
              </div>
            </div>

            {/* Control buttons */}
            <div class="flex items-center justify-between">
              <div class="flex items-center gap-2">
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={stepPrev}
                  disabled={replayIndex() <= 0}
                  aria-label={t("trajectory.prevStep")}
                >
                  {"\u25C0"}
                </Button>
                <Button
                  variant={playing() ? "danger" : "primary"}
                  size="sm"
                  onClick={togglePlayback}
                  aria-label={playing() ? t("trajectory.pauseReplay") : t("trajectory.playReplay")}
                >
                  {playing() ? "\u23F8" : "\u25B6"}
                </Button>
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={stepNext}
                  disabled={replayIndex() >= events().length - 1}
                  aria-label={t("trajectory.nextStep")}
                >
                  {"\u25B6"}
                </Button>
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={cycleSpeed}
                  title={t("trajectory.speedLabel")}
                >
                  {PLAYBACK_SPEEDS[speedIdx()]}x
                </Button>
              </div>
              <span class="text-xs text-cf-text-tertiary">
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
                class="rounded-cf-sm border border-cf-border"
                aria-live="polite"
                aria-label={t("trajectory.eventAria", {
                  type: ev().type,
                  time: fmt.time(ev().created_at),
                })}
              >
                <div class="flex items-center gap-2 border-b border-cf-border-subtle px-4 py-3">
                  <span
                    class={`h-3 w-3 rounded-full ${EVENT_COLORS[ev().type] ?? "bg-cf-border-input"}`}
                    aria-hidden="true"
                  />
                  <span class="font-mono text-sm font-medium text-cf-text-secondary">
                    {ev().type}
                  </span>
                  <span class="flex-1" />
                  <span class="text-xs text-cf-text-muted">{fmt.time(ev().created_at)}</span>
                  <span class="text-xs text-cf-text-muted">v{ev().version}</span>
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
            fallback={<p class="text-sm text-cf-text-muted">{t("common.loading")}</p>}
          >
            <div class="space-y-1">
              <For each={events()}>
                {(ev: AgentEvent) => (
                  <div
                    class={`cursor-pointer rounded-cf-sm border hover:border-cf-border ${
                      expandedId() === ev.id ? "border-cf-accent" : "border-cf-border-subtle"
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
                    <div class="group flex items-center gap-2 px-3 py-2">
                      <span
                        class={`h-2.5 w-2.5 rounded-full ${EVENT_COLORS[ev.type] ?? "bg-cf-border-input"}`}
                        aria-hidden="true"
                      />
                      <span class="font-mono text-xs text-cf-text-tertiary">{ev.type}</span>
                      <span class="flex-1" />
                      {/* Hover-visible rewind button */}
                      <button
                        type="button"
                        class="hidden group-hover:inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-[10px] text-cf-text-muted hover:text-cf-accent hover:bg-cf-accent/10 transition-colors"
                        title={t("session.rewindToHere")}
                        onClick={(e) => {
                          e.stopPropagation();
                          requestRewind(ev.id);
                        }}
                      >
                        {"\u21A9"} {t("session.rewind")}
                      </button>
                      <span class="text-xs text-cf-text-muted">{fmt.time(ev.created_at)}</span>
                      <span class="text-xs text-cf-text-muted">v{ev.version}</span>
                    </div>

                    <Show when={expandedId() === ev.id}>
                      <div class="border-t border-cf-border-subtle bg-cf-bg-inset px-3 py-2">
                        <EventDetail event={ev} />
                        <div class="flex gap-2 mt-2 pt-2 border-t border-cf-border-subtle">
                          <Button
                            variant="secondary"
                            size="sm"
                            onClick={() => void handleFork(ev.id)}
                            disabled={forking()}
                            aria-label={t("session.forkFromHere")}
                          >
                            {forking() ? "..." : t("session.fork")}
                          </Button>
                          <Button
                            variant="secondary"
                            size="sm"
                            onClick={() => requestRewind(ev.id)}
                            disabled={rewinding()}
                            aria-label={t("session.rewindToHere")}
                          >
                            {rewinding() ? "..." : t("session.rewind")}
                          </Button>
                        </div>
                      </div>
                    </Show>
                  </div>
                )}
              </For>
            </div>

            {/* Pagination */}
            <div class="mt-3 flex items-center justify-between text-sm">
              <span class="text-xs text-cf-text-muted">
                {t("trajectory.total", { n: trajectory()?.total ?? 0 })}
              </span>
              <div class="flex gap-2">
                <Show when={cursor()}>
                  <Button variant="secondary" size="sm" onClick={handlePrevPage}>
                    {t("trajectory.first")}
                  </Button>
                </Show>
                <Show when={trajectory()?.has_more}>
                  <Button variant="secondary" size="sm" onClick={handleNextPage}>
                    {t("trajectory.next")}
                  </Button>
                </Show>
              </div>
            </div>
          </Show>
        </Show>
      </Card.Body>

      {/* Rewind confirmation dialog */}
      <Show when={rewindConfirmId()}>
        <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/40">
          <div class="rounded-cf-md bg-cf-bg-surface border border-cf-border shadow-lg p-5 max-w-sm mx-4">
            <p class="text-sm text-cf-text-primary mb-4">{t("trajectory.rewindConfirm")}</p>
            <div class="flex justify-end gap-2">
              <Button variant="secondary" size="sm" onClick={cancelRewind}>
                {t("trajectory.cancel")}
              </Button>
              <Button
                variant="primary"
                size="sm"
                disabled={rewinding()}
                onClick={() => {
                  const id = rewindConfirmId();
                  if (id) void handleRewind(id);
                }}
              >
                {rewinding() ? "..." : t("trajectory.rewindConfirmBtn")}
              </Button>
            </div>
          </div>
        </div>
      </Show>
    </Card>
  );
}
