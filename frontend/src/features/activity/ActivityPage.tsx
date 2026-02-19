import { createSignal, For, onCleanup, Show } from "solid-js";

import { createCodeForgeWS, type WSMessage } from "~/api/websocket";
import { useI18n } from "~/i18n";

/** A single activity entry shown in the stream */
interface ActivityEntry {
  id: number;
  time: Date;
  type: string;
  projectId: string;
  summary: string;
  severity: "info" | "success" | "warning" | "error";
}

const MAX_ENTRIES = 200;

let nextId = 0;

const SEVERITY_COLORS: Record<ActivityEntry["severity"], string> = {
  info: "bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400",
  success: "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400",
  warning: "bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400",
  error: "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400",
};

const TYPE_ICONS: Record<string, string> = {
  "run.status": "\u25B6",
  "run.toolcall": "\u2699",
  "run.budget_alert": "\u26A0",
  "run.qualitygate": "\u2714",
  "run.delivery": "\uD83D\uDCE6",
  "agent.status": "\uD83E\uDD16",
  "task.status": "\u2611",
  "plan.status": "\uD83D\uDCCB",
  "plan.step.status": "\u2192",
  "repomap.status": "\uD83D\uDDFA",
  "retrieval.status": "\uD83D\uDD0D",
  "roadmap.status": "\uD83D\uDEE3",
};

function classifyMessage(msg: WSMessage): ActivityEntry | null {
  const p = msg.payload;
  const projectId = (p.project_id as string) ?? "";

  switch (msg.type) {
    case "run.status": {
      const status = p.status as string;
      const runId = ((p.run_id as string) ?? "").slice(0, 8);
      const severity =
        status === "completed"
          ? "success"
          : status === "failed" || status === "timeout"
            ? "error"
            : "info";
      return {
        id: nextId++,
        time: new Date(),
        type: msg.type,
        projectId,
        summary: `Run ${runId} \u2192 ${status}`,
        severity,
      };
    }
    case "run.toolcall": {
      const tool = (p.tool as string) ?? "?";
      const phase = (p.phase as string) ?? "";
      return {
        id: nextId++,
        time: new Date(),
        type: msg.type,
        projectId,
        summary: `Tool call: ${tool} (${phase})`,
        severity: phase === "denied" ? "warning" : "info",
      };
    }
    case "run.budget_alert": {
      const pct = p.percentage as number;
      return {
        id: nextId++,
        time: new Date(),
        type: msg.type,
        projectId,
        summary: `Budget alert: ${pct}% used`,
        severity: "warning",
      };
    }
    case "run.qualitygate": {
      const passed = p.passed as boolean;
      return {
        id: nextId++,
        time: new Date(),
        type: msg.type,
        projectId,
        summary: `Quality gate ${passed ? "passed" : "failed"}`,
        severity: passed ? "success" : "error",
      };
    }
    case "run.delivery": {
      const status = p.status as string;
      return {
        id: nextId++,
        time: new Date(),
        type: msg.type,
        projectId,
        summary: `Delivery ${status}`,
        severity: status === "completed" ? "success" : status === "failed" ? "error" : "info",
      };
    }
    case "agent.status": {
      const name = (p.name as string) ?? "";
      const status = p.status as string;
      return {
        id: nextId++,
        time: new Date(),
        type: msg.type,
        projectId,
        summary: `Agent ${name} \u2192 ${status}`,
        severity: status === "error" ? "error" : "info",
      };
    }
    case "task.status": {
      const title = (p.title as string) ?? "";
      const status = p.status as string;
      return {
        id: nextId++,
        time: new Date(),
        type: msg.type,
        projectId,
        summary: `Task "${title.slice(0, 40)}" \u2192 ${status}`,
        severity: status === "error" ? "error" : "info",
      };
    }
    case "plan.status": {
      const name = (p.name as string) ?? "";
      const status = p.status as string;
      return {
        id: nextId++,
        time: new Date(),
        type: msg.type,
        projectId,
        summary: `Plan "${name}" \u2192 ${status}`,
        severity: status === "completed" ? "success" : status === "failed" ? "error" : "info",
      };
    }
    case "plan.step.status": {
      const status = p.status as string;
      return {
        id: nextId++,
        time: new Date(),
        type: msg.type,
        projectId,
        summary: `Plan step \u2192 ${status}`,
        severity: status === "completed" ? "success" : status === "failed" ? "error" : "info",
      };
    }
    case "repomap.status":
    case "retrieval.status":
    case "roadmap.status": {
      const status = (p.status as string) ?? "";
      const label = msg.type.split(".")[0];
      return {
        id: nextId++,
        time: new Date(),
        type: msg.type,
        projectId,
        summary: `${label.charAt(0).toUpperCase() + label.slice(1)} ${status}`,
        severity: status === "error" ? "error" : "info",
      };
    }
    default:
      return null;
  }
}

export default function ActivityPage() {
  const { t, fmt } = useI18n();
  const { connected, onMessage } = createCodeForgeWS();
  const [entries, setEntries] = createSignal<ActivityEntry[]>([]);
  const [filterType, setFilterType] = createSignal<string>("");
  const [paused, setPaused] = createSignal(false);

  const cleanup = onMessage((msg) => {
    if (paused()) return;
    const entry = classifyMessage(msg);
    if (entry) {
      setEntries((prev) => [entry, ...prev].slice(0, MAX_ENTRIES));
    }
  });

  onCleanup(cleanup);

  const filtered = () => {
    const f = filterType();
    if (!f) return entries();
    return entries().filter((e) => e.type === f);
  };

  const eventTypes = () => {
    const types = new Set(entries().map((e) => e.type));
    return [...types].sort();
  };

  return (
    <div>
      <div class="mb-6 flex items-center justify-between">
        <div class="flex items-center gap-3">
          <h2 class="text-2xl font-bold text-gray-900 dark:text-gray-100">{t("activity.title")}</h2>
          <span
            class={`rounded-full px-2 py-0.5 text-xs ${
              connected()
                ? "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400"
                : "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400"
            }`}
            role="status"
          >
            {connected() ? t("activity.connected") : t("activity.disconnected")}
          </span>
        </div>
        <div class="flex items-center gap-2">
          <select
            class="rounded border border-gray-300 px-2 py-1.5 text-sm dark:border-gray-600 dark:bg-gray-700"
            value={filterType()}
            aria-label={t("activity.filterLabel")}
            onChange={(e) => setFilterType(e.currentTarget.value)}
          >
            <option value="">{t("activity.allEvents")}</option>
            <For each={eventTypes()}>{(type) => <option value={type}>{type}</option>}</For>
          </select>
          <button
            type="button"
            class={`rounded px-3 py-1.5 text-sm font-medium ${
              paused()
                ? "bg-green-600 text-white hover:bg-green-700"
                : "bg-yellow-600 text-white hover:bg-yellow-700"
            }`}
            onClick={() => setPaused((v) => !v)}
          >
            {paused() ? t("activity.resume") : t("activity.pause")}
          </button>
          <button
            type="button"
            class="rounded bg-gray-200 px-3 py-1.5 text-sm font-medium text-gray-700 hover:bg-gray-300 dark:bg-gray-700 dark:text-gray-300 dark:hover:bg-gray-600"
            onClick={() => setEntries([])}
          >
            {t("activity.clear")}
          </button>
        </div>
      </div>

      <Show
        when={filtered().length > 0}
        fallback={
          <div class="rounded-lg border border-gray-200 bg-white p-8 text-center dark:border-gray-700 dark:bg-gray-800">
            <p class="text-sm text-gray-500 dark:text-gray-400">
              {entries().length === 0 ? t("activity.empty") : t("activity.noMatch")}
            </p>
          </div>
        }
      >
        <div class="space-y-1" role="log" aria-label={t("activity.streamLabel")} aria-live="polite">
          <For each={filtered()}>
            {(entry) => (
              <div class="flex items-center gap-3 rounded-lg border border-gray-100 bg-white px-4 py-2 dark:border-gray-700/50 dark:bg-gray-800">
                <span class="text-sm" aria-hidden="true">
                  {TYPE_ICONS[entry.type] ?? "\u2022"}
                </span>
                <span class="whitespace-nowrap text-xs tabular-nums text-gray-400 dark:text-gray-500">
                  {fmt.time(entry.time.toISOString())}
                </span>
                <span
                  class={`rounded px-1.5 py-0.5 text-xs font-medium ${SEVERITY_COLORS[entry.severity]}`}
                >
                  {entry.type.split(".").pop()}
                </span>
                <span class="flex-1 text-sm text-gray-700 dark:text-gray-300">{entry.summary}</span>
                <Show when={entry.projectId}>
                  <a
                    href={`/projects/${entry.projectId}`}
                    class="whitespace-nowrap text-xs text-blue-600 hover:underline dark:text-blue-400"
                  >
                    {entry.projectId.slice(0, 8)}
                  </a>
                </Show>
              </div>
            )}
          </For>
        </div>
      </Show>

      <Show when={filtered().length > 0}>
        <p class="mt-3 text-xs text-gray-400 dark:text-gray-500">
          {t("activity.showing", {
            count: String(filtered().length),
            total: String(entries().length),
          })}
        </p>
      </Show>
    </div>
  );
}
