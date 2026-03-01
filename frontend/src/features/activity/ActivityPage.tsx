import { createSignal, For, onCleanup, Show } from "solid-js";

import { createCodeForgeWS, type WSMessage } from "~/api/websocket";
import { severityVariant } from "~/config/statusVariants";
import { useI18n } from "~/i18n";
import { Badge, Button, Card, EmptyState, PageLayout, Select } from "~/ui";

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

  // eslint-disable-next-line solid/reactivity -- paused() is intentionally read at message-receive time
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
    <PageLayout
      title={t("activity.title")}
      action={
        <div class="flex items-center gap-3">
          <Badge variant={connected() ? "success" : "danger"} pill>
            {connected() ? t("activity.connected") : t("activity.disconnected")}
          </Badge>
          <Select
            value={filterType()}
            aria-label={t("activity.filterLabel")}
            onChange={(e) => setFilterType(e.currentTarget.value)}
            class="w-auto"
          >
            <option value="">{t("activity.allEvents")}</option>
            <For each={eventTypes()}>{(type) => <option value={type}>{type}</option>}</For>
          </Select>
          <Button
            variant={paused() ? "primary" : "secondary"}
            size="sm"
            onClick={() => setPaused((v) => !v)}
          >
            {paused() ? t("activity.resume") : t("activity.pause")}
          </Button>
          <Button variant="secondary" size="sm" onClick={() => setEntries([])}>
            {t("activity.clear")}
          </Button>
        </div>
      }
    >
      <Show
        when={filtered().length > 0}
        fallback={
          <Card>
            <Card.Body>
              <EmptyState
                title={entries().length === 0 ? t("activity.empty") : t("activity.noMatch")}
              />
            </Card.Body>
          </Card>
        }
      >
        <div class="space-y-1" role="log" aria-label={t("activity.streamLabel")} aria-live="polite">
          <For each={filtered()}>
            {(entry) => (
              <Card>
                <div class="flex items-center gap-3 px-4 py-2">
                  <span class="text-sm" aria-hidden="true">
                    {TYPE_ICONS[entry.type] ?? "\u2022"}
                  </span>
                  <span class="whitespace-nowrap text-xs tabular-nums text-cf-text-muted">
                    {fmt.time(entry.time.toISOString())}
                  </span>
                  <Badge variant={severityVariant[entry.severity]} pill>
                    {entry.type.split(".").pop()}
                  </Badge>
                  <span class="flex-1 text-sm text-cf-text-secondary">{entry.summary}</span>
                  <Show when={entry.projectId}>
                    <a
                      href={`/projects/${entry.projectId}`}
                      class="whitespace-nowrap text-xs text-cf-accent hover:underline"
                    >
                      {entry.projectId.slice(0, 8)}
                    </a>
                  </Show>
                </div>
              </Card>
            )}
          </For>
        </div>
      </Show>

      <Show when={filtered().length > 0}>
        <p class="mt-3 text-xs text-cf-text-muted">
          {t("activity.showing", {
            count: String(filtered().length),
            total: String(entries().length),
          })}
        </p>
      </Show>
    </PageLayout>
  );
}
