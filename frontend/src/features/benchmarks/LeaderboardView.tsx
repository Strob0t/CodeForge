import { createResource, createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type { BenchmarkSuite, LeaderboardEntry } from "~/api/types";
import { useI18n } from "~/i18n";
import { Badge, Card, CostDisplay, EmptyState, LoadingState, Select } from "~/ui";

type SortMetric =
  | "avg_score"
  | "total_cost_usd"
  | "cost_per_score_point"
  | "token_efficiency"
  | "duration_ms";

export function LeaderboardView() {
  const { t } = useI18n();
  const [suiteId, setSuiteId] = createSignal<string>("");
  const [sortMetric, setSortMetric] = createSignal<SortMetric>("avg_score");
  const [suites] = createResource(() => api.benchmarks.listSuites());
  const [entries, { refetch }] = createResource(
    () => suiteId() || "__all__",
    () => api.benchmarks.leaderboard(suiteId() || undefined),
  );

  const sortedEntries = (): LeaderboardEntry[] => {
    const raw = entries() ?? [];
    const metric = sortMetric();
    return [...raw].sort((a, b) => {
      if (metric === "avg_score") return b.avg_score - a.avg_score;
      if (metric === "total_cost_usd") return a.total_cost_usd - b.total_cost_usd;
      if (metric === "cost_per_score_point") return a.cost_per_score_point - b.cost_per_score_point;
      if (metric === "token_efficiency") return b.token_efficiency - a.token_efficiency;
      if (metric === "duration_ms") return a.duration_ms - b.duration_ms;
      return 0;
    });
  };

  const medal = (idx: number): string => {
    if (idx === 0) return "#FFD700"; // gold
    if (idx === 1) return "#C0C0C0"; // silver
    if (idx === 2) return "#CD7F32"; // bronze
    return "";
  };

  return (
    <div class="space-y-4">
      {/* Filters */}
      <div class="flex flex-wrap items-center gap-3">
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

        <label class="ml-4 text-sm font-medium">{t("benchmark.leaderboard.sortBy")}</label>
        <Select
          value={sortMetric()}
          onChange={(e) => setSortMetric(e.currentTarget.value as SortMetric)}
          class="w-48"
        >
          <option value="avg_score">{t("benchmark.leaderboard.avgScore")}</option>
          <option value="total_cost_usd">{t("benchmark.leaderboard.totalCost")}</option>
          <option value="cost_per_score_point">{t("benchmark.leaderboard.costPerPoint")}</option>
          <option value="token_efficiency">{t("benchmark.leaderboard.tokenEfficiency")}</option>
          <option value="duration_ms">{t("benchmark.duration")}</option>
        </Select>
      </div>

      <Show when={!entries.loading} fallback={<LoadingState />}>
        <Show
          when={sortedEntries().length}
          fallback={<EmptyState title={t("benchmark.leaderboard.empty")} />}
        >
          <Card class="overflow-x-auto p-0">
            <table class="w-full text-left text-sm">
              <thead>
                <tr class="border-b border-cf-border text-xs text-cf-text-muted">
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
                <For each={sortedEntries()}>
                  {(entry: LeaderboardEntry, idx) => (
                    <tr class="border-b border-cf-border last:border-0">
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
                      <td class="px-4 py-2.5 text-right text-xs text-cf-text-muted">
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
