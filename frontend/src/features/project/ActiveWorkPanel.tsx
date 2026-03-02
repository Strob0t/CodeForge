import { createResource, createSignal, For, onCleanup, Show } from "solid-js";

import { api } from "~/api/client";
import type { ActiveWorkItem } from "~/api/types";
import { createCodeForgeWS } from "~/api/websocket";
import { Badge } from "~/ui";

interface Props {
  projectId: string;
}

export default function ActiveWorkPanel(props: Props) {
  const { onMessage } = createCodeForgeWS();
  const [collapsed, setCollapsed] = createSignal(false);

  const [items, { refetch }] = createResource(
    () => props.projectId,
    async (id) => {
      try {
        return await api.activeWork.list(id);
      } catch {
        return [];
      }
    },
  );

  // Refetch on relevant WS events
  let debounceTimer: ReturnType<typeof setTimeout> | undefined;
  const debouncedRefetch = () => {
    clearTimeout(debounceTimer);
    debounceTimer = setTimeout(() => refetch(), 500);
  };

  // eslint-disable-next-line solid/reactivity -- props.projectId is intentionally read at message-receive time
  const cleanup = onMessage((msg) => {
    const payload = msg.payload;
    const pid = (payload as Record<string, unknown>).project_id as string | undefined;
    if (pid && pid !== props.projectId) return;

    switch (msg.type) {
      case "activework.claimed":
      case "activework.released":
      case "task.status":
      case "run.status":
        debouncedRefetch();
        break;
    }
  });
  onCleanup(() => {
    cleanup();
    clearTimeout(debounceTimer);
  });

  const activeItems = (): ActiveWorkItem[] => items() ?? [];
  const count = () => activeItems().length;

  return (
    <Show when={count() > 0}>
      <div class="border-b border-cf-border bg-cf-bg-secondary/50">
        <button
          type="button"
          class="flex w-full items-center justify-between px-4 py-2 text-left hover:bg-cf-bg-tertiary/30 transition-colors"
          onClick={() => setCollapsed((v) => !v)}
        >
          <div class="flex items-center gap-2">
            <span class="text-xs font-semibold uppercase tracking-wider text-cf-text-tertiary">
              Active Work
            </span>
            <Badge variant="info" pill>
              {count()}
            </Badge>
          </div>
          <svg
            class={`h-3.5 w-3.5 text-cf-text-tertiary transition-transform ${collapsed() ? "-rotate-90" : ""}`}
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
            stroke-width="2"
          >
            <path stroke-linecap="round" stroke-linejoin="round" d="M19 9l-7 7-7-7" />
          </svg>
        </button>

        <Show when={!collapsed()}>
          <div class="px-4 pb-2 space-y-1">
            <For each={activeItems()}>
              {(item) => (
                <div class="flex items-center justify-between rounded-cf-sm px-2.5 py-1.5 text-sm bg-cf-bg-primary border border-cf-border-subtle">
                  <div class="flex items-center gap-2 min-w-0">
                    <StatusDot status={item.task_status} />
                    <span class="truncate font-medium text-cf-text-primary" title={item.task_title}>
                      {item.task_title}
                    </span>
                  </div>
                  <div class="flex items-center gap-2 flex-shrink-0 ml-2">
                    <Show when={item.agent_mode}>
                      <Badge variant="neutral" pill>
                        {item.agent_mode}
                      </Badge>
                    </Show>
                    <span class="text-xs text-cf-text-tertiary">{item.agent_name}</span>
                    <Show when={item.step_count}>
                      <span class="text-xs text-cf-text-muted">{item.step_count} steps</span>
                    </Show>
                    <Show when={item.cost_usd}>
                      <span class="text-xs text-cf-text-muted">
                        ${(item.cost_usd ?? 0).toFixed(3)}
                      </span>
                    </Show>
                  </div>
                </div>
              )}
            </For>
          </div>
        </Show>
      </div>
    </Show>
  );
}

function StatusDot(props: { status: string }) {
  const color = () => {
    switch (props.status) {
      case "running":
        return "bg-green-400";
      case "queued":
        return "bg-yellow-400";
      default:
        return "bg-cf-text-muted";
    }
  };
  const pulse = () => (props.status === "running" ? "animate-pulse" : "");

  return <span class={`inline-block h-2 w-2 rounded-full ${color()} ${pulse()}`} />;
}
