import { createSignal, onCleanup, For, Show, type Component } from "solid-js";
import { useNavigate } from "@solidjs/router";
import { useWebSocket } from "~/components/WebSocketProvider";

interface TimelineEvent {
  id: string;
  type: string;
  tier: number;
  projectId?: string;
  projectName?: string;
  agentName?: string;
  summary: string;
  timestamp: number;
}

/** Maps WS event types to priority tiers (1 = highest / most urgent). */
const TIER_MAP: Record<string, number> = {
  "agent.error": 1,
  "run.failed": 1,
  "run.stall_detected": 1,
  "run.budget_alert": 2,
  "run.qualitygate.failed": 2,
  "run.completed": 3,
  "run.delivery.completed": 3,
  "plan.completed": 3,
  "agent.started": 4,
  "run.started": 4,
  "task.status": 4,
  "agent.step_done": 5,
  "agent.tool_called": 5,
};

/** Colored dot classes per tier. */
const TIER_COLORS: Record<number, string> = {
  1: "bg-[var(--cf-danger)]",
  2: "bg-[var(--cf-warning)]",
  3: "bg-[var(--cf-success)]",
  4: "bg-[var(--cf-accent)]",
  5: "bg-[var(--cf-text-muted)]",
};

/** Returns a human-readable relative timestamp (e.g. "3m ago"). */
function relativeTime(ts: number): string {
  const diff = Math.floor((Date.now() - ts) / 1000);
  if (diff < 60) return `${diff}s ago`;
  if (diff < 3600) return `${Math.floor(diff / 60)}m ago`;
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`;
  return `${Math.floor(diff / 86400)}d ago`;
}

const MAX_EVENTS = 100;
const VISIBLE_DEFAULT = 15;

const ActivityTimeline: Component = () => {
  const [events, setEvents] = createSignal<TimelineEvent[]>([]);
  const [showAll, setShowAll] = createSignal(false);
  const navigate = useNavigate();
  const ws = useWebSocket();

  const cleanup = ws.onMessage((msg) => {
    const tier = TIER_MAP[msg.type];
    if (tier === undefined) return;

    const payload = msg.payload;
    const evt: TimelineEvent = {
      id: `${msg.type}-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
      type: msg.type,
      tier,
      projectId: typeof payload.project_id === "string" ? payload.project_id : undefined,
      projectName: typeof payload.project_name === "string" ? payload.project_name : undefined,
      agentName: typeof payload.agent_name === "string" ? payload.agent_name : undefined,
      summary:
        (typeof payload.summary === "string" ? payload.summary : undefined) ??
        (typeof payload.error === "string" ? payload.error : undefined) ??
        msg.type,
      timestamp: Date.now(),
    };

    setEvents((prev) => {
      const next = [evt, ...prev].slice(0, MAX_EVENTS);
      next.sort((a, b) => a.tier - b.tier || b.timestamp - a.timestamp);
      return next;
    });
  });

  onCleanup(cleanup);

  const visibleEvents = (): TimelineEvent[] => {
    const all = events();
    return showAll() ? all : all.slice(0, VISIBLE_DEFAULT);
  };

  return (
    <div class="flex flex-col gap-1">
      <h3 class="text-sm font-semibold text-[var(--cf-text-primary)]">Activity</h3>

      <Show when={events().length === 0}>
        <p class="py-4 text-center text-xs text-[var(--cf-text-muted)]">No recent activity</p>
      </Show>

      <div class="space-y-0">
        <For each={visibleEvents()}>
          {(evt) => (
            <div class="flex items-start gap-2 border-l-2 border-[var(--cf-border)] py-1.5 pl-3">
              <span
                class={`mt-1 inline-block h-2 w-2 shrink-0 rounded-full ${TIER_COLORS[evt.tier]}`}
              />
              <div class="min-w-0 flex-1">
                <p class="truncate text-xs text-[var(--cf-text-secondary)]">
                  {evt.projectName ?? "System"}
                  {evt.agentName ? ` - ${evt.agentName}` : ""}
                </p>
                <p
                  class="truncate text-xs"
                  classList={{
                    "font-semibold text-[var(--cf-text-primary)]": evt.tier <= 2,
                    "text-[var(--cf-text-secondary)]": evt.tier > 2,
                  }}
                >
                  {evt.summary}
                </p>
                <p class="text-[10px] text-[var(--cf-text-muted)]">{relativeTime(evt.timestamp)}</p>
              </div>
              <Show when={evt.projectId}>
                <button
                  class="shrink-0 text-xs text-[var(--cf-accent)] hover:underline"
                  onClick={() => navigate(`/projects/${evt.projectId}`)}
                  title="Go to project"
                >
                  {"\u2192"}
                </button>
              </Show>
            </div>
          )}
        </For>
      </div>

      <Show when={events().length > VISIBLE_DEFAULT && !showAll()}>
        <button
          class="mt-1 text-xs text-[var(--cf-accent)] hover:underline"
          onClick={() => setShowAll(true)}
        >
          Show more...
        </button>
      </Show>
    </div>
  );
};

export default ActivityTimeline;
