import { createResource, createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type { AgentEvent, AgentEventType, TrajectorySummary } from "~/api/types";
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

export default function TrajectoryPanel(props: TrajectoryPanelProps) {
  const { t } = useI18n();
  const [typeFilter, setTypeFilter] = createSignal("");
  const [cursor, setCursor] = createSignal("");

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

  const EVENT_TYPES: AgentEventType[] = [
    "agent.started",
    "agent.step_done",
    "agent.tool_called",
    "agent.tool_result",
    "agent.finished",
    "agent.error",
  ];

  return (
    <div class="rounded-lg border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-gray-800">
      <div class="mb-3 flex items-center justify-between">
        <h3 class="text-lg font-semibold">{t("trajectory.title")}</h3>
        <button
          class="rounded bg-gray-100 px-3 py-1 text-xs hover:bg-gray-200 dark:bg-gray-700 dark:hover:bg-gray-600"
          onClick={handleExport}
        >
          {t("trajectory.exportJson")}
        </button>
      </div>

      {/* Stats Summary */}
      <Show when={stats()}>
        {(s) => (
          <div class="mb-4 flex flex-wrap gap-4 rounded bg-gray-50 p-3 text-sm dark:bg-gray-700">
            <span>
              <span class="text-gray-500 dark:text-gray-400">{t("trajectory.events")}</span>{" "}
              {s().total_events}
            </span>
            <span>
              <span class="text-gray-500 dark:text-gray-400">{t("trajectory.duration")}</span>{" "}
              {(s().duration_ms / 1000).toFixed(1)}s
            </span>
            <span>
              <span class="text-gray-500 dark:text-gray-400">{t("trajectory.toolCalls")}</span>{" "}
              {s().tool_call_count}
            </span>
            <span>
              <span class="text-gray-500 dark:text-gray-400">{t("trajectory.errors")}</span>{" "}
              <span class={s().error_count > 0 ? "text-red-600 dark:text-red-400" : ""}>
                {s().error_count}
              </span>
            </span>
          </div>
        )}
      </Show>

      {/* Filters */}
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

      {/* Timeline */}
      <Show
        when={trajectory()}
        fallback={<p class="text-sm text-gray-500 dark:text-gray-400">{t("common.loading")}</p>}
      >
        <div class="space-y-1">
          <For each={trajectory()?.events ?? []}>
            {(ev: AgentEvent) => (
              <div
                class="cursor-pointer rounded border border-gray-100 hover:border-gray-200 dark:border-gray-700 dark:hover:border-gray-600"
                role="button"
                tabIndex={0}
                aria-expanded={expandedId() === ev.id}
                aria-label={`Event: ${ev.type} at ${new Date(ev.created_at).toLocaleTimeString()}`}
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
                  <span class="font-mono text-xs text-gray-600 dark:text-gray-400">{ev.type}</span>
                  <span class="flex-1" />
                  <span class="text-xs text-gray-400 dark:text-gray-500">
                    {new Date(ev.created_at).toLocaleTimeString()}
                  </span>
                  <span class="text-xs text-gray-400 dark:text-gray-500">v{ev.version}</span>
                </div>

                <Show when={expandedId() === ev.id}>
                  <div class="border-t border-gray-100 bg-gray-50 px-3 py-2 dark:border-gray-700 dark:bg-gray-700">
                    <pre class="max-h-40 overflow-auto whitespace-pre-wrap text-xs text-gray-700 dark:text-gray-300">
                      {JSON.stringify(ev.payload, null, 2)}
                    </pre>
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
    </div>
  );
}
