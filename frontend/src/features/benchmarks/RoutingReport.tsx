import { For, Show } from "solid-js";

import type {
  FallbackEvent,
  ModelUsageStats,
  ProviderStatus,
  RoutingReport as RoutingReportType,
} from "~/api/types";
import { useI18n } from "~/i18n";
import { Badge, Card, CostDisplay } from "~/ui";

/** Color palette for the model distribution bar segments. */
const MODEL_COLORS = [
  "var(--color-cf-accent, #3b82f6)",
  "#10b981",
  "#f59e0b",
  "#ef4444",
  "#8b5cf6",
  "#ec4899",
  "#14b8a6",
  "#f97316",
];

function statusColor(status: ProviderStatus): string {
  if (!status.reachable) return "#ef4444"; // red
  if (status.errors > 0) return "#f59e0b"; // yellow
  return "#10b981"; // green
}

interface RoutingReportProps {
  report: RoutingReportType;
}

export function RoutingReport(props: RoutingReportProps) {
  const { t } = useI18n();

  const modelEntries = (): [string, ModelUsageStats][] =>
    Object.entries(props.report.models_used).sort(
      ([, a], [, b]) => b.task_percentage - a.task_percentage,
    );

  return (
    <Card class="mt-4 space-y-5 p-4">
      <h3 class="text-sm font-semibold">{t("benchmark.routingReport")}</h3>

      {/* Summary */}
      <div class="flex gap-6 text-sm">
        <div>
          <span class="text-gray-500 dark:text-gray-400">{t("benchmark.systemScore")}:</span>{" "}
          <span class="font-mono font-medium">{props.report.system_score.toFixed(3)}</span>
        </div>
        <div>
          <span class="text-gray-500 dark:text-gray-400">{t("benchmark.systemCost")}:</span>{" "}
          <CostDisplay usd={props.report.system_cost} />
        </div>
        <div>
          <span class="text-gray-500 dark:text-gray-400">{t("benchmark.fallbackEvents")}:</span>{" "}
          <span class="font-mono">{props.report.fallback_events}</span>
        </div>
      </div>

      {/* Model Distribution Bar */}
      <div>
        <div class="mb-1 text-xs font-medium text-gray-500 dark:text-gray-400">
          {t("benchmark.modelDistribution")}
        </div>
        <div class="flex h-5 w-full overflow-hidden rounded-full">
          <For each={modelEntries()}>
            {([model, stats], idx) => (
              <div
                class="flex items-center justify-center text-[10px] font-medium text-white transition-all"
                style={{
                  width: `${Math.max(stats.task_percentage, 2)}%`,
                  "background-color": MODEL_COLORS[idx() % MODEL_COLORS.length],
                }}
                title={`${model}: ${stats.task_percentage.toFixed(1)}%`}
              >
                <Show when={stats.task_percentage >= 8}>{model.split("/").pop()}</Show>
              </div>
            )}
          </For>
        </div>
        {/* Legend */}
        <div class="mt-1.5 flex flex-wrap gap-x-3 gap-y-1">
          <For each={modelEntries()}>
            {([model], idx) => (
              <div class="flex items-center gap-1 text-xs">
                <span
                  class="inline-block h-2.5 w-2.5 rounded-sm"
                  style={{ "background-color": MODEL_COLORS[idx() % MODEL_COLORS.length] }}
                />
                <span class="text-gray-600 dark:text-gray-300">{model}</span>
              </div>
            )}
          </For>
        </div>
      </div>

      {/* Per-model stats table */}
      <div>
        <table class="w-full text-sm">
          <thead>
            <tr class="border-b text-left text-xs text-gray-500 dark:border-gray-700">
              <th class="pb-2">{t("benchmark.model")}</th>
              <th class="pb-2 text-right">{t("benchmark.tasks")}</th>
              <th class="pb-2 text-right">{t("benchmark.percentage")}</th>
              <th class="pb-2 text-right">{t("benchmark.avgScore")}</th>
              <th class="pb-2 text-right">{t("benchmark.avgCost")}</th>
            </tr>
          </thead>
          <tbody>
            <For each={modelEntries()}>
              {([model, stats]) => (
                <tr class="border-b dark:border-gray-700">
                  <td class="py-1.5 font-mono text-xs">{model}</td>
                  <td class="py-1.5 text-right">{stats.task_count}</td>
                  <td class="py-1.5 text-right">{stats.task_percentage.toFixed(1)}%</td>
                  <td class="py-1.5 text-right font-mono">{stats.avg_score.toFixed(3)}</td>
                  <td class="py-1.5 text-right">
                    <CostDisplay usd={stats.avg_cost_per_task} />
                  </td>
                </tr>
              )}
            </For>
          </tbody>
        </table>
      </div>

      {/* Fallback events */}
      <div>
        <div class="mb-1 text-xs font-medium text-gray-500 dark:text-gray-400">
          {t("benchmark.fallbackEvents")}
        </div>
        <Show
          when={props.report.fallback_details.length > 0}
          fallback={<p class="text-xs text-gray-400">{t("benchmark.noFallbacks")}</p>}
        >
          <div class="max-h-40 space-y-1 overflow-auto">
            <For each={props.report.fallback_details}>
              {(evt: FallbackEvent) => (
                <div class="flex items-center gap-2 rounded bg-gray-50 px-2 py-1 text-xs dark:bg-gray-800">
                  <span class="font-mono text-gray-500">{evt.task_id.slice(0, 8)}</span>
                  <Badge variant="default">{evt.primary}</Badge>
                  <span class="text-gray-400">{"\u2192"}</span>
                  <Badge variant="warning">{evt.fallback_to}</Badge>
                  <span class="text-gray-500">{evt.reason}</span>
                </div>
              )}
            </For>
          </div>
        </Show>
      </div>

      {/* Provider availability */}
      <div>
        <div class="mb-1 text-xs font-medium text-gray-500 dark:text-gray-400">
          {t("benchmark.providerStatus")}
        </div>
        <div class="flex flex-wrap gap-3">
          <For each={Object.entries(props.report.provider_availability)}>
            {([name, status]: [string, ProviderStatus]) => (
              <div class="flex items-center gap-1.5 rounded border px-2 py-1 text-xs dark:border-gray-700">
                <span
                  class="inline-block h-2.5 w-2.5 rounded-full"
                  style={{ "background-color": statusColor(status) }}
                />
                <span class="font-medium">{name}</span>
                <Show when={status.errors > 0}>
                  <span class="text-gray-400">({status.errors} errors)</span>
                </Show>
              </div>
            )}
          </For>
        </div>
      </div>
    </Card>
  );
}
