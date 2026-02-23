import { A } from "@solidjs/router";
import { createResource, For, Show } from "solid-js";

import { api } from "~/api/client";
import type {
  DailyCost,
  ModelCostSummary,
  ProjectCostSummary,
  Run,
  ToolCostSummary,
} from "~/api/types";
import { useI18n } from "~/i18n";

export default function CostDashboardPage() {
  const { t, fmt } = useI18n();
  const [globalCosts] = createResource(() => api.costs.global());

  // Compute totals from global summary
  const totals = () => {
    const items = globalCosts() ?? [];
    return items.reduce(
      (acc, p) => ({
        cost: acc.cost + p.total_cost_usd,
        tokensIn: acc.tokensIn + p.total_tokens_in,
        tokensOut: acc.tokensOut + p.total_tokens_out,
        runs: acc.runs + p.run_count,
      }),
      { cost: 0, tokensIn: 0, tokensOut: 0, runs: 0 },
    );
  };

  return (
    <div>
      <h2 class="mb-6 text-2xl font-bold">{t("costs.title")}</h2>

      {/* Global Totals */}
      <div class="mb-6 grid grid-cols-4 gap-4">
        <div class="rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-4">
          <p class="text-sm text-gray-500 dark:text-gray-400">{t("costs.totalCost")}</p>
          <p class="mt-1 text-2xl font-bold text-gray-900 dark:text-gray-100">
            {fmt.currency(totals().cost)}
          </p>
        </div>
        <div class="rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-4">
          <p class="text-sm text-gray-500 dark:text-gray-400">{t("costs.tokensIn")}</p>
          <p class="mt-1 text-2xl font-bold text-gray-900 dark:text-gray-100">
            {fmt.compact(totals().tokensIn)}
          </p>
        </div>
        <div class="rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-4">
          <p class="text-sm text-gray-500 dark:text-gray-400">{t("costs.tokensOut")}</p>
          <p class="mt-1 text-2xl font-bold text-gray-900 dark:text-gray-100">
            {fmt.compact(totals().tokensOut)}
          </p>
        </div>
        <div class="rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-4">
          <p class="text-sm text-gray-500 dark:text-gray-400">{t("costs.totalRuns")}</p>
          <p class="mt-1 text-2xl font-bold text-gray-900 dark:text-gray-100">{totals().runs}</p>
        </div>
      </div>

      {/* Project Breakdown */}
      <div class="mb-6 rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-4">
        <h3 class="mb-3 text-lg font-semibold">{t("costs.byProject")}</h3>
        <Show
          when={(globalCosts() ?? []).length > 0}
          fallback={<p class="text-sm text-gray-400 dark:text-gray-500">{t("costs.empty")}</p>}
        >
          <table class="w-full text-sm">
            <thead>
              <tr class="border-b border-gray-100 dark:border-gray-700 text-left text-gray-500 dark:text-gray-400">
                <th scope="col" class="pb-2 font-medium">
                  {t("costs.table.project")}
                </th>
                <th scope="col" class="pb-2 text-right font-medium">
                  {t("costs.table.cost")}
                </th>
                <th scope="col" class="pb-2 text-right font-medium">
                  {t("costs.table.tokensIn")}
                </th>
                <th scope="col" class="pb-2 text-right font-medium">
                  {t("costs.table.tokensOut")}
                </th>
                <th scope="col" class="pb-2 text-right font-medium">
                  {t("costs.table.runs")}
                </th>
              </tr>
            </thead>
            <tbody>
              <For each={globalCosts() ?? []}>
                {(p: ProjectCostSummary) => (
                  <tr class="border-b border-gray-50 dark:border-gray-700">
                    <td class="py-2">
                      <A
                        href={`/projects/${p.project_id}`}
                        class="text-blue-600 dark:text-blue-400 hover:underline"
                      >
                        {p.project_name || p.project_id}
                      </A>
                    </td>
                    <td class="py-2 text-right font-mono">{fmt.currency(p.total_cost_usd)}</td>
                    <td class="py-2 text-right font-mono">{fmt.compact(p.total_tokens_in)}</td>
                    <td class="py-2 text-right font-mono">{fmt.compact(p.total_tokens_out)}</td>
                    <td class="py-2 text-right">{p.run_count}</td>
                  </tr>
                )}
              </For>
            </tbody>
          </table>
        </Show>
      </div>
    </div>
  );
}

/** Reusable cost section for a specific project */
export function ProjectCostSection(props: { projectId: string }) {
  const { t, tp, fmt } = useI18n();
  const [summary] = createResource(
    () => props.projectId,
    (id) => api.costs.project(id),
  );
  const [byModel] = createResource(
    () => props.projectId,
    (id) => api.costs.byModel(id),
  );
  const [daily] = createResource(
    () => props.projectId,
    (id) => api.costs.daily(id, 30),
  );
  const [recentRuns] = createResource(
    () => props.projectId,
    (id) => api.costs.recentRuns(id, 10),
  );
  const [byTool] = createResource(
    () => props.projectId,
    (id) => api.costs.byTool(id),
  );

  const maxDailyCost = () => {
    const items = daily() ?? [];
    return Math.max(...items.map((d) => d.cost_usd), 0.001);
  };

  return (
    <div class="rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-4">
      <h3 class="mb-3 text-lg font-semibold">{t("costs.overview")}</h3>

      {/* Summary Cards */}
      <Show when={summary()}>
        {(s) => (
          <div class="mb-4 grid grid-cols-4 gap-3">
            <div class="rounded bg-gray-50 dark:bg-gray-900 p-3">
              <p class="text-xs text-gray-500 dark:text-gray-400">{t("costs.totalCost")}</p>
              <p class="text-lg font-bold">{fmt.currency(s().total_cost_usd)}</p>
            </div>
            <div class="rounded bg-gray-50 dark:bg-gray-900 p-3">
              <p class="text-xs text-gray-500 dark:text-gray-400">{t("costs.tokensIn")}</p>
              <p class="text-lg font-bold">{fmt.compact(s().total_tokens_in)}</p>
            </div>
            <div class="rounded bg-gray-50 dark:bg-gray-900 p-3">
              <p class="text-xs text-gray-500 dark:text-gray-400">{t("costs.tokensOut")}</p>
              <p class="text-lg font-bold">{fmt.compact(s().total_tokens_out)}</p>
            </div>
            <div class="rounded bg-gray-50 dark:bg-gray-900 p-3">
              <p class="text-xs text-gray-500 dark:text-gray-400">{t("costs.table.runs")}</p>
              <p class="text-lg font-bold">{s().run_count}</p>
            </div>
          </div>
        )}
      </Show>

      {/* Model Breakdown */}
      <Show when={(byModel() ?? []).length > 0}>
        <div class="mb-4">
          <h4 class="mb-2 text-sm font-medium text-gray-500 dark:text-gray-400">
            {t("costs.byModel")}
          </h4>
          <div class="space-y-1">
            <For each={byModel() ?? []}>
              {(m: ModelCostSummary) => (
                <div class="flex items-center justify-between rounded bg-gray-50 dark:bg-gray-900 px-3 py-2 text-sm">
                  <span class="font-mono text-xs">{m.model || t("costs.unknown")}</span>
                  <div class="flex gap-4 text-xs text-gray-500 dark:text-gray-400">
                    <span>{fmt.currency(m.total_cost_usd)}</span>
                    <span>
                      {fmt.compact(m.total_tokens_in)} {t("costs.in")}
                    </span>
                    <span>
                      {fmt.compact(m.total_tokens_out)} {t("costs.out")}
                    </span>
                    <span>{tp("costs.runs", m.run_count)}</span>
                  </div>
                </div>
              )}
            </For>
          </div>
        </div>
      </Show>

      {/* Tool Breakdown */}
      <Show when={(byTool() ?? []).length > 0}>
        <div class="mb-4">
          <h4 class="mb-2 text-sm font-medium text-gray-500 dark:text-gray-400">
            {t("costs.byTool")}
          </h4>
          <div class="space-y-1">
            <For each={byTool() ?? []}>
              {(item: ToolCostSummary) => (
                <div class="flex items-center justify-between rounded bg-gray-50 dark:bg-gray-900 px-3 py-2 text-sm">
                  <div class="flex items-center gap-2">
                    <span class="font-mono text-xs">{item.tool || t("costs.unknown")}</span>
                    <span class="text-xs text-gray-400 dark:text-gray-500">{item.model || ""}</span>
                  </div>
                  <div class="flex gap-4 text-xs text-gray-500 dark:text-gray-400">
                    <span>{fmt.currency(item.cost_usd)}</span>
                    <span>
                      {fmt.compact(item.tokens_in)} {t("costs.in")}
                    </span>
                    <span>
                      {fmt.compact(item.tokens_out)} {t("costs.out")}
                    </span>
                    <span>{tp("costs.calls", item.call_count)}</span>
                  </div>
                </div>
              )}
            </For>
          </div>
        </div>
      </Show>

      {/* Daily Cost Chart (CSS bars) */}
      <Show when={(daily() ?? []).length > 0}>
        <div class="mb-4">
          <h4 class="mb-2 text-sm font-medium text-gray-500 dark:text-gray-400">
            {t("costs.dailyChart")}
          </h4>
          <div
            class="flex items-end gap-0.5"
            style={{ height: "80px" }}
            role="img"
            aria-label={t("costs.dailyChartAria")}
          >
            <For each={daily() ?? []}>
              {(d: DailyCost) => {
                const pct = () => Math.max((d.cost_usd / maxDailyCost()) * 100, 2);
                return (
                  <div
                    class="flex-1 rounded-t bg-blue-400 hover:bg-blue-500"
                    style={{ height: `${pct()}%` }}
                    title={`${d.date}: ${fmt.currency(d.cost_usd)} (${d.run_count} runs)`}
                  />
                );
              }}
            </For>
          </div>
        </div>
      </Show>

      {/* Recent Runs */}
      <Show when={(recentRuns() ?? []).length > 0}>
        <div>
          <h4 class="mb-2 text-sm font-medium text-gray-500 dark:text-gray-400">
            {t("costs.recentRuns")}
          </h4>
          <div class="space-y-1">
            <For each={recentRuns() ?? []}>
              {(r: Run) => (
                <div class="flex items-center justify-between rounded bg-gray-50 dark:bg-gray-900 px-3 py-2 text-sm">
                  <div class="flex items-center gap-2">
                    <span class="font-mono text-xs text-gray-400 dark:text-gray-500">
                      {r.id.slice(0, 8)}
                    </span>
                    <span class="text-xs text-gray-500 dark:text-gray-400">{r.model || "-"}</span>
                  </div>
                  <div class="flex gap-3 text-xs text-gray-500 dark:text-gray-400">
                    <span>{fmt.currency(r.cost_usd)}</span>
                    <span>
                      {fmt.compact(r.tokens_in)} {t("costs.in")}
                    </span>
                    <span>
                      {fmt.compact(r.tokens_out)} {t("costs.out")}
                    </span>
                    <span>{tp("costs.steps", r.step_count)}</span>
                  </div>
                </div>
              )}
            </For>
          </div>
        </div>
      </Show>
    </div>
  );
}
