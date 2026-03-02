import { createResource, createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type { BenchmarkSuite, LeaderboardEntry } from "~/api/types";
import { useI18n } from "~/i18n";
import { Badge, Card, CostDisplay, EmptyState, LoadingState, Select } from "~/ui";

export function LeaderboardView() {
  const { t } = useI18n();
  const [suiteId, setSuiteId] = createSignal<string>("");
  const [suites] = createResource(() => api.benchmarks.listSuites());
  const [entries, { refetch }] = createResource(
    () => suiteId() || "__all__",
    () => api.benchmarks.leaderboard(suiteId() || undefined),
  );

  const medal = (idx: number): string => {
    if (idx === 0) return "#FFD700"; // gold
    if (idx === 1) return "#C0C0C0"; // silver
    if (idx === 2) return "#CD7F32"; // bronze
    return "";
  };

  return (
    <div class="space-y-4">
      {/* Suite filter */}
      <div class="flex items-center gap-3">
        <label class="text-sm font-medium">{t("benchmark.leaderboard.filterBySuite")}</label>
        <Select
          value={suiteId()}
          onChange={(e) => {
            setSuiteId(e.currentTarget.value);
            refetch();
          }}
          class="w-60"
        >
          <option value="">{t("benchmark.leaderboard.allSuites")}</option>
          <For each={suites() ?? []}>
            {(s: BenchmarkSuite) => <option value={s.id}>{s.name}</option>}
          </For>
        </Select>
      </div>

      <Show when={!entries.loading} fallback={<LoadingState />}>
        <Show
          when={entries()?.length}
          fallback={<EmptyState title={t("benchmark.leaderboard.empty")} />}
        >
          <Card class="overflow-x-auto p-0">
            <table class="w-full text-left text-sm">
              <thead>
                <tr class="border-b text-xs text-gray-500 dark:border-gray-700">
                  <th class="px-4 py-3">#</th>
                  <th class="px-4 py-3">{t("benchmark.model")}</th>
                  <th class="px-4 py-3 text-right">{t("benchmark.leaderboard.avgScore")}</th>
                  <th class="px-4 py-3 text-right">{t("benchmark.leaderboard.totalCost")}</th>
                  <th class="px-4 py-3 text-right">{t("benchmark.leaderboard.taskCount")}</th>
                  <th class="px-4 py-3 text-right">{t("benchmark.leaderboard.costPerPoint")}</th>
                  <th class="px-4 py-3 text-right">{t("benchmark.leaderboard.tokenEfficiency")}</th>
                  <th class="px-4 py-3 text-right">{t("benchmark.duration")}</th>
                </tr>
              </thead>
              <tbody>
                <For each={entries()}>
                  {(entry: LeaderboardEntry, idx) => (
                    <tr class="border-b last:border-0 dark:border-gray-700">
                      <td class="px-4 py-2.5">
                        <span
                          class="inline-flex h-6 w-6 items-center justify-center rounded-full text-xs font-bold"
                          style={medal(idx()) ? { background: medal(idx()), color: "#000" } : {}}
                        >
                          {idx() + 1}
                        </span>
                      </td>
                      <td class="px-4 py-2.5 font-medium">{entry.model}</td>
                      <td class="px-4 py-2.5 text-right">
                        <Badge
                          variant={
                            entry.avg_score >= 0.8
                              ? "success"
                              : entry.avg_score >= 0.5
                                ? "warning"
                                : "danger"
                          }
                        >
                          {entry.avg_score.toFixed(3)}
                        </Badge>
                      </td>
                      <td class="px-4 py-2.5 text-right font-mono text-xs">
                        <CostDisplay usd={entry.total_cost_usd} />
                      </td>
                      <td class="px-4 py-2.5 text-right">{entry.task_count}</td>
                      <td class="px-4 py-2.5 text-right font-mono text-xs">
                        <CostDisplay usd={entry.cost_per_score_point} />
                      </td>
                      <td class="px-4 py-2.5 text-right font-mono text-xs">
                        {entry.token_efficiency.toFixed(1)}
                      </td>
                      <td class="px-4 py-2.5 text-right text-xs text-gray-500">
                        {entry.duration_ms < 1000
                          ? `${entry.duration_ms}ms`
                          : `${(entry.duration_ms / 1000).toFixed(1)}s`}
                      </td>
                    </tr>
                  )}
                </For>
              </tbody>
            </table>
          </Card>
        </Show>
      </Show>
    </div>
  );
}
