import { A } from "@solidjs/router";
import { createResource, For, Show } from "solid-js";

import { api } from "~/api/client";
import type { DailyCost, ModelCostSummary, ProjectCostSummary, Run } from "~/api/types";

function formatCost(usd: number): string {
  return usd < 0.01 && usd > 0 ? `$${usd.toFixed(6)}` : `$${usd.toFixed(4)}`;
}

function formatTokens(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`;
  return String(n);
}

export default function CostDashboardPage() {
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
      <h2 class="mb-6 text-2xl font-bold">Cost Dashboard</h2>

      {/* Global Totals */}
      <div class="mb-6 grid grid-cols-4 gap-4">
        <div class="rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-4">
          <p class="text-sm text-gray-500 dark:text-gray-400">Total Cost</p>
          <p class="mt-1 text-2xl font-bold text-gray-900 dark:text-gray-100">
            {formatCost(totals().cost)}
          </p>
        </div>
        <div class="rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-4">
          <p class="text-sm text-gray-500 dark:text-gray-400">Tokens In</p>
          <p class="mt-1 text-2xl font-bold text-gray-900 dark:text-gray-100">
            {formatTokens(totals().tokensIn)}
          </p>
        </div>
        <div class="rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-4">
          <p class="text-sm text-gray-500 dark:text-gray-400">Tokens Out</p>
          <p class="mt-1 text-2xl font-bold text-gray-900 dark:text-gray-100">
            {formatTokens(totals().tokensOut)}
          </p>
        </div>
        <div class="rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-4">
          <p class="text-sm text-gray-500 dark:text-gray-400">Total Runs</p>
          <p class="mt-1 text-2xl font-bold text-gray-900 dark:text-gray-100">{totals().runs}</p>
        </div>
      </div>

      {/* Project Breakdown */}
      <div class="mb-6 rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-4">
        <h3 class="mb-3 text-lg font-semibold">Cost by Project</h3>
        <Show
          when={(globalCosts() ?? []).length > 0}
          fallback={<p class="text-sm text-gray-400 dark:text-gray-500">No cost data yet.</p>}
        >
          <table class="w-full text-sm">
            <thead>
              <tr class="border-b border-gray-100 dark:border-gray-700 text-left text-gray-500 dark:text-gray-400">
                <th class="pb-2 font-medium">Project</th>
                <th class="pb-2 text-right font-medium">Cost</th>
                <th class="pb-2 text-right font-medium">Tokens In</th>
                <th class="pb-2 text-right font-medium">Tokens Out</th>
                <th class="pb-2 text-right font-medium">Runs</th>
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
                    <td class="py-2 text-right font-mono">{formatCost(p.total_cost_usd)}</td>
                    <td class="py-2 text-right font-mono">{formatTokens(p.total_tokens_in)}</td>
                    <td class="py-2 text-right font-mono">{formatTokens(p.total_tokens_out)}</td>
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

  const maxDailyCost = () => {
    const items = daily() ?? [];
    return Math.max(...items.map((d) => d.cost_usd), 0.001);
  };

  return (
    <div class="rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-4">
      <h3 class="mb-3 text-lg font-semibold">Cost Overview</h3>

      {/* Summary Cards */}
      <Show when={summary()}>
        {(s) => (
          <div class="mb-4 grid grid-cols-4 gap-3">
            <div class="rounded bg-gray-50 dark:bg-gray-900 p-3">
              <p class="text-xs text-gray-500 dark:text-gray-400">Total Cost</p>
              <p class="text-lg font-bold">{formatCost(s().total_cost_usd)}</p>
            </div>
            <div class="rounded bg-gray-50 dark:bg-gray-900 p-3">
              <p class="text-xs text-gray-500 dark:text-gray-400">Tokens In</p>
              <p class="text-lg font-bold">{formatTokens(s().total_tokens_in)}</p>
            </div>
            <div class="rounded bg-gray-50 dark:bg-gray-900 p-3">
              <p class="text-xs text-gray-500 dark:text-gray-400">Tokens Out</p>
              <p class="text-lg font-bold">{formatTokens(s().total_tokens_out)}</p>
            </div>
            <div class="rounded bg-gray-50 dark:bg-gray-900 p-3">
              <p class="text-xs text-gray-500 dark:text-gray-400">Runs</p>
              <p class="text-lg font-bold">{s().run_count}</p>
            </div>
          </div>
        )}
      </Show>

      {/* Model Breakdown */}
      <Show when={(byModel() ?? []).length > 0}>
        <div class="mb-4">
          <h4 class="mb-2 text-sm font-medium text-gray-500 dark:text-gray-400">Cost by Model</h4>
          <div class="space-y-1">
            <For each={byModel() ?? []}>
              {(m: ModelCostSummary) => (
                <div class="flex items-center justify-between rounded bg-gray-50 dark:bg-gray-900 px-3 py-2 text-sm">
                  <span class="font-mono text-xs">{m.model || "(unknown)"}</span>
                  <div class="flex gap-4 text-xs text-gray-500 dark:text-gray-400">
                    <span>{formatCost(m.total_cost_usd)}</span>
                    <span>{formatTokens(m.total_tokens_in)} in</span>
                    <span>{formatTokens(m.total_tokens_out)} out</span>
                    <span>{m.run_count} runs</span>
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
            Daily Cost (last 30 days)
          </h4>
          <div class="flex items-end gap-0.5" style={{ height: "80px" }}>
            <For each={daily() ?? []}>
              {(d: DailyCost) => {
                const pct = () => Math.max((d.cost_usd / maxDailyCost()) * 100, 2);
                return (
                  <div
                    class="flex-1 rounded-t bg-blue-400 hover:bg-blue-500"
                    style={{ height: `${pct()}%` }}
                    title={`${d.date}: ${formatCost(d.cost_usd)} (${d.run_count} runs)`}
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
          <h4 class="mb-2 text-sm font-medium text-gray-500 dark:text-gray-400">Recent Runs</h4>
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
                    <span>{formatCost(r.cost_usd)}</span>
                    <span>{formatTokens(r.tokens_in)} in</span>
                    <span>{formatTokens(r.tokens_out)} out</span>
                    <span>{r.step_count} steps</span>
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
