import { createResource, createSignal, For, onCleanup, Show } from "solid-js";

import { api } from "~/api/client";
import type { Agent } from "~/api/types";
import { createCodeForgeWS } from "~/api/websocket";

import AgentLane from "./AgentLane";
import MessageFlow from "./MessageFlow";
import SharedContextPanel from "./SharedContextPanel";

export default function WarRoom(props: { projectId: string }) {
  const { onMessage } = createCodeForgeWS();
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

  // Debounced refetch on WS events
  const [refetchTimer, setRefetchTimer] = createSignal<ReturnType<typeof setTimeout> | null>(null);
  function debouncedRefetch() {
    const existing = refetchTimer();
    if (existing) clearTimeout(existing);
    setRefetchTimer(setTimeout(() => refetch(), 500));
  }

  const cleanup = onMessage((msg) => {
    const p = msg.payload;
    const projectId = props.projectId;

    switch (msg.type) {
      case "agent.status": {
        if ((p.project_id as string) === projectId) debouncedRefetch();
        break;
      }
      case "run.status": {
        if ((p.project_id as string) === projectId) debouncedRefetch();
        break;
      }
      case "activework.claimed":
      case "activework.released": {
        if ((p.project_id as string) === projectId) debouncedRefetch();
        break;
      }
    }
  });
  onCleanup(() => {
    cleanup();
    const t = refetchTimer();
    if (t) clearTimeout(t);
  });

  return (
    <div class="flex flex-col h-full">
      <div class="relative flex-1 overflow-y-auto p-4" ref={gridRef}>
        <Show
          when={(agents() ?? []).length > 0}
          fallback={
            <div class="flex flex-col items-center justify-center h-full text-cf-text-muted">
              <svg
                class="h-12 w-12 mb-3 opacity-30"
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
              <p class="text-sm">No agents currently active</p>
              <p class="text-xs mt-1">Start an agent to see live activity here</p>
            </div>
          }
        >
          <div
            class="grid gap-4"
            style={{ "grid-template-columns": "repeat(auto-fill, minmax(320px, 1fr))" }}
          >
            <For each={agents() ?? []}>
              {(agent) => (
                <div data-agent-id={agent.id}>
                  <AgentLane agent={agent} />
                </div>
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
