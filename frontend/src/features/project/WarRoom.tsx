import {
  createEffect,
  createMemo,
  createResource,
  createSignal,
  For,
  onCleanup,
  Show,
} from "solid-js";

import { api } from "~/api/client";
import type { Agent, Run } from "~/api/types";
import type { WSMessage } from "~/api/websocket";
import { useWebSocket } from "~/components/WebSocketProvider";
import { useI18n } from "~/i18n";

import AgentLane from "./AgentLane";
import {
  type AgentWork,
  agentWorkFromRuns,
  collectLaneEvent,
  EMPTY_LANE_FEED,
  IDLE_WORK,
  isLaneEvent,
  isProjectEvent,
  type LaneFeed,
  payloadString,
  reduceAgentWork,
} from "./liveEvents";
import MessageFlow from "./MessageFlow";
import SharedContextPanel from "./SharedContextPanel";

interface WarRoomProps {
  projectId: string;
  onNavigate?: (target: string) => void;
}

export default function WarRoom(props: WarRoomProps) {
  const { t } = useI18n();
  const { onMessage } = useWebSocket();
  let gridRef: HTMLDivElement | undefined;

  const [agents, { refetch }] = createResource(
    () => props.projectId,
    async (id) => {
      try {
        return await api.agents.active(id);
      } catch {
        return [] as Agent[];
      }
    },
  );

  // The project's recent runs attach a lane opened mid-run to its agent's run.
  const [recentRuns, { refetch: refetchRuns }] = createResource(
    () => props.projectId,
    async (id) => {
      try {
        return await api.costs.recentRuns(id, 50);
      } catch {
        return [] as Run[];
      }
    },
  );

  // What each agent works on, followed from the project's events. Kept here and
  // not in the lanes: a lane mounts only after the agent list is refetched,
  // after the agent.status of the agent's run start.
  const [works, setWorks] = createSignal<Record<string, AgentWork>>({});
  const workOf = (agentId: string): AgentWork =>
    works()[agentId] ?? agentWorkFromRuns(recentRuns() ?? [], agentId) ?? IDLE_WORK;

  // Lanes are keyed by agent ID so a refetch keeps them mounted.
  const agentIds = () => (agents() ?? []).map((a) => a.id);
  const agentById = (id: string) => (agents() ?? []).find((a) => a.id === id);

  // The output and tool calls of the agents' work, collected here for the
  // same reason: a run's first output arrives before its agent's lane mounts.
  const [feed, setFeed] = createSignal<LaneFeed>(EMPTY_LANE_FEED);
  // Recomputed only when the agents or their work change, not per event.
  const trackedWork = createMemo((): AgentWork[] =>
    [...new Set([...Object.keys(works()), ...agentIds()])].map(workOf),
  );
  const laneOutputs = (agentId: string) => feed().outputs[workOf(agentId).taskId ?? ""] ?? [];
  const laneToolCalls = (agentId: string) => feed().toolCalls[workOf(agentId).runId ?? ""] ?? [];

  // Debounced refetch on WS events
  const [refetchTimer, setRefetchTimer] = createSignal<ReturnType<typeof setTimeout> | null>(null);
  function debouncedRefetch() {
    const existing = refetchTimer();
    if (existing) clearTimeout(existing);
    setRefetchTimer(
      setTimeout(() => {
        refetch();
        refetchRuns();
      }, 500),
    );
  }

  // The agents' work follows the project's events; the agent list (and the
  // runs a lane opened mid-run attaches to) follow agent status and claims.
  function followProjectEvent(msg: WSMessage): void {
    const agentId = payloadString(msg.payload, "agent_id");
    if (agentId) {
      setWorks((prev) => {
        const current =
          prev[agentId] ?? agentWorkFromRuns(recentRuns() ?? [], agentId) ?? IDLE_WORK;
        const next = reduceAgentWork(current, msg, agentId);
        return next === current ? prev : { ...prev, [agentId]: next };
      });
    }

    switch (msg.type) {
      case "agent.status":
      case "activework.claimed":
      case "activework.released":
        debouncedRefetch();
        break;
    }
  }

  createEffect(() => {
    const projectId = props.projectId;
    setWorks({});
    setFeed(EMPTY_LANE_FEED);
    // eslint-disable-next-line solid/reactivity -- subscription callback, not a reactive computation
    const unsub = onMessage((msg) => {
      if (isProjectEvent(msg, projectId)) followProjectEvent(msg);
      // task.output and run.toolcall name no project, only their task or run.
      if (isLaneEvent(msg)) setFeed((prev) => collectLaneEvent(prev, msg, trackedWork()));
    });
    onCleanup(unsub);
  });
  onCleanup(() => {
    const t = refetchTimer();
    if (t) clearTimeout(t);
  });

  return (
    <div class="flex flex-col h-full">
      <div class="relative flex-1 overflow-y-auto p-4" ref={gridRef}>
        <Show
          when={(agents() ?? []).length > 0}
          fallback={
            <div class="flex flex-col items-center justify-center gap-3 py-16 text-center">
              <svg
                class="h-12 w-12 opacity-30 text-cf-text-muted"
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
                stroke-width="1.5"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  d="M18 18.72a9.094 9.094 0 0 0 3.741-.479 3 3 0 0 0-4.682-2.72m.94 3.198.001.031c0 .225-.012.447-.037.666A11.944 11.944 0 0 1 12 21c-2.17 0-4.207-.576-5.963-1.584A6.062 6.062 0 0 1 6 18.719m12 0a5.971 5.971 0 0 0-.941-3.197m0 0A5.995 5.995 0 0 0 12 12.75a5.995 5.995 0 0 0-5.058 2.772m0 0a3 3 0 0 0-4.681 2.72 8.986 8.986 0 0 0 3.74.477m.94-3.197a5.971 5.971 0 0 0-.94 3.197M15 6.75a3 3 0 1 1-6 0 3 3 0 0 1 6 0Zm6 3a2.25 2.25 0 1 1-4.5 0 2.25 2.25 0 0 1 4.5 0Zm-13.5 0a2.25 2.25 0 1 1-4.5 0 2.25 2.25 0 0 1 4.5 0Z"
                />
              </svg>
              <p class="text-sm text-cf-text-muted">{t("empty.warroom")}</p>
              <button
                class="text-sm text-cf-accent hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cf-focus-ring focus-visible:ring-offset-2"
                onClick={() => props.onNavigate?.("chat")}
              >
                {t("empty.warroom.action")}
              </button>
            </div>
          }
        >
          <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3">
            <For each={agentIds()}>
              {(id) => (
                <Show when={agentById(id)}>
                  {(agent) => (
                    <div data-agent-id={id}>
                      <AgentLane
                        agent={agent()}
                        work={workOf(id)}
                        outputs={laneOutputs(id)}
                        toolCalls={laneToolCalls(id)}
                      />
                    </div>
                  )}
                </Show>
              )}
            </For>
          </div>
          <MessageFlow containerRef={gridRef} />
        </Show>
      </div>
      <SharedContextPanel />
    </div>
  );
}
