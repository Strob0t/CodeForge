import { createResource, createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type { BenchmarkRun, CostAnalysis } from "~/api/types";
import { useI18n } from "~/i18n";
import { Card, CostDisplay, EmptyState, LoadingState, Select } from "~/ui";

interface CostAnalysisViewProps {
  runs: BenchmarkRun[];
}

export function CostAnalysisView(props: CostAnalysisViewProps) {
  const { t } = useI18n();
  const [selectedRunId, setSelectedRunId] = createSignal<string>("");
  const [analysis] = createResource(selectedRunId, (id) =>
    id ? api.benchmarks.costAnalysis(id) : undefined,
  );

  return (
    <div class="space-y-4">
      {/* Run selector */}
      <div class="flex items-center gap-3">
        <label class="text-sm font-medium">{t("benchmark.costAnalysis.selectRun")}</label>
        <Select
          value={selectedRunId()}
          onChange={(e) => setSelectedRunId(e.currentTarget.value)}
          class="w-full sm:w-80"
        >
          <option value="">{t("common.select")}</option>
          <For each={props.runs}>
            {(r: BenchmarkRun) => (
              <option value={r.id}>
                {r.dataset} / {r.model} ({r.status})
              </option>
            )}
          </For>
        </Select>
      </div>

      <Show when={!analysis.loading} fallback={selectedRunId() ? <LoadingState /> : null}>
        <Show
          when={analysis()}
          fallback={
            selectedRunId() ? null : <EmptyState title={t("benchmark.costAnalysis.empty")} />
          }
        >
          {(ca: () => CostAnalysis) => (
            <>
              {/* Summary cards */}
              <div class="grid grid-cols-2 gap-3 sm:grid-cols-4">
                <SummaryCard
                  label={t("benchmark.costAnalysis.totalCost")}
                  value={<CostDisplay usd={ca().total_cost_usd} />}
                />
                <SummaryCard
                  label={t("benchmark.costAnalysis.avgScore")}
                  value={<span class="font-mono">{ca().avg_score.toFixed(3)}</span>}
                />
                <SummaryCard
                  label={t("benchmark.costAnalysis.costPerPoint")}
                  value={<CostDisplay usd={ca().cost_per_score_point} />}
                />
                <SummaryCard
                  label={t("benchmark.costAnalysis.tokenEfficiency")}
                  value={<span class="font-mono">{ca().token_efficiency.toFixed(1)}</span>}
                />
              </div>

              {/* Token totals */}
              <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
                <Card class="p-3">
                  <div class="text-xs text-gray-500">{t("benchmark.costAnalysis.tokensIn")}</div>
                  <div class="mt-1 text-lg font-semibold">
                    {ca().total_tokens_in.toLocaleString()}
                  </div>
                </Card>
                <Card class="p-3">
                  <div class="text-xs text-gray-500">{t("benchmark.costAnalysis.tokensOut")}</div>
                  <div class="mt-1 text-lg font-semibold">
                    {ca().total_tokens_out.toLocaleString()}
                  </div>
                </Card>
              </div>

              {/* Task breakdown table */}
              <Show when={ca().task_breakdown?.length}>
                <Card class="overflow-x-auto p-0">
                  <div class="px-4 py-3 text-sm font-semibold">
                    {t("benchmark.costAnalysis.taskBreakdown")}
                  </div>
                  <table class="w-full text-left text-sm">
                    <thead>
                      <tr class="border-b text-xs text-gray-500 dark:border-gray-700">
                        <th class="px-4 py-2">{t("benchmark.taskName")}</th>
                        <th class="px-4 py-2 text-right">{t("benchmark.cost")}</th>
                        <th class="px-4 py-2 text-right">{t("benchmark.costAnalysis.tokensIn")}</th>
                        <th class="px-4 py-2 text-right">
                          {t("benchmark.costAnalysis.tokensOut")}
                        </th>
                        <th class="px-4 py-2 text-right">{t("benchmark.scores")}</th>
                      </tr>
                    </thead>
                    <tbody>
                      <For each={ca().task_breakdown}>
                        {(task) => (
                          <tr class="border-b last:border-0 dark:border-gray-700">
                            <td class="px-4 py-2 font-medium">{task.task_name}</td>
                            <td class="px-4 py-2 text-right font-mono text-xs">
                              <CostDisplay usd={task.cost_usd} />
                            </td>
                            <td class="px-4 py-2 text-right text-xs">
                              {task.tokens_in.toLocaleString()}
                            </td>
                            <td class="px-4 py-2 text-right text-xs">
                              {task.tokens_out.toLocaleString()}
                            </td>
                            <td class="px-4 py-2 text-right font-mono text-xs">
                              {task.score.toFixed(3)}
                            </td>
                          </tr>
                        )}
                      </For>
                    </tbody>
                  </table>
                </Card>
              </Show>

              {/* Export link */}
              <div class="flex gap-2">
                <a
                  href={api.benchmarks.exportTrainingUrl(ca().run_id, "json")}
                  target="_blank"
                  rel="noopener noreferrer"
                  class="text-sm text-blue-600 underline hover:text-blue-800 dark:text-blue-400"
                >
                  {t("benchmark.export.training")} (JSON)
                </a>
                <a
                  href={api.benchmarks.exportTrainingUrl(ca().run_id, "jsonl")}
                  target="_blank"
                  rel="noopener noreferrer"
                  class="text-sm text-blue-600 underline hover:text-blue-800 dark:text-blue-400"
                >
                  {t("benchmark.export.training")} (JSONL)
                </a>
              </div>
            </>
          )}
        </Show>
      </Show>
    </div>
  );
}

function SummaryCard(props: { label: string; value: import("solid-js").JSX.Element }) {
  return (
    <Card class="p-3">
      <div class="text-xs text-gray-500">{props.label}</div>
      <div class="mt-1 text-lg font-semibold">{props.value}</div>
    </Card>
  );
}
