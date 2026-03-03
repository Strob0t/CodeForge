import { createSignal, For, Show } from "solid-js";

import type { BenchmarkResult } from "~/api/types";
import { useI18n } from "~/i18n";
import { CostDisplay, LoadingState } from "~/ui";

interface BenchmarkRunDetailProps {
  results: BenchmarkResult[] | undefined;
  loading: boolean;
  formatDuration: (ms: number) => string;
}

export function BenchmarkRunDetail(props: BenchmarkRunDetailProps) {
  const { t } = useI18n();
  const [expandedRow, setExpandedRow] = createSignal<string | null>(null);

  return (
    <div class="mt-4 border-t pt-4 dark:border-gray-700">
      <Show when={!props.loading} fallback={<LoadingState />}>
        <Show
          when={props.results?.length}
          fallback={<p class="text-sm text-gray-500">{t("benchmark.noResults")}</p>}
        >
          <table class="w-full text-sm">
            <thead>
              <tr class="border-b text-left text-xs text-gray-500 dark:border-gray-700">
                <th class="pb-2">{t("benchmark.taskName")}</th>
                <th class="pb-2">{t("benchmark.scores")}</th>
                <th class="pb-2">{t("benchmark.cost")}</th>
                <th class="pb-2">{t("benchmark.duration")}</th>
                <th class="pb-2" />
              </tr>
            </thead>
            <tbody>
              <For each={props.results}>
                {(res: BenchmarkResult) => {
                  const isExpanded = () => expandedRow() === res.task_id;
                  const hasDetails =
                    res.actual_output ||
                    (res.tool_calls && res.tool_calls.length > 0) ||
                    res.functional_test_output;

                  return (
                    <>
                      <tr
                        class={`border-b dark:border-gray-700 ${hasDetails ? "cursor-pointer hover:bg-gray-50 dark:hover:bg-gray-800" : ""}`}
                        onClick={() =>
                          hasDetails && setExpandedRow(isExpanded() ? null : res.task_id)
                        }
                      >
                        <td class="py-2 font-medium">{res.task_name}</td>
                        <td class="py-2">
                          <div class="flex flex-wrap gap-1">
                            <For each={Object.entries(res.scores)}>
                              {([k, v]) => (
                                <span class="rounded bg-gray-100 px-1.5 py-0.5 text-xs dark:bg-gray-800">
                                  {k}: {(v as number).toFixed(3)}
                                </span>
                              )}
                            </For>
                          </div>
                        </td>
                        <td class="py-2 font-mono text-xs">
                          <CostDisplay usd={res.cost_usd} />
                        </td>
                        <td class="py-2 text-xs">{props.formatDuration(res.duration_ms)}</td>
                        <td class="py-2 text-xs text-gray-400">
                          {hasDetails ? (isExpanded() ? "\u25B2" : "\u25BC") : ""}
                        </td>
                      </tr>

                      {/* Expanded detail row */}
                      <Show when={isExpanded()}>
                        <tr class="border-b bg-gray-50 dark:border-gray-700 dark:bg-gray-900">
                          <td colspan="5" class="px-4 py-3">
                            <div class="space-y-3">
                              {/* Actual Output */}
                              <Show when={res.actual_output}>
                                <div>
                                  <div class="mb-1 text-xs font-semibold text-gray-500">
                                    Actual Output
                                  </div>
                                  <pre class="max-h-40 overflow-auto rounded bg-gray-100 p-2 text-xs dark:bg-gray-800">
                                    {res.actual_output}
                                  </pre>
                                </div>
                              </Show>

                              {/* Tool Calls */}
                              <Show when={res.tool_calls?.length}>
                                <div>
                                  <div class="mb-1 text-xs font-semibold text-gray-500">
                                    Tool Calls ({res.tool_calls?.length ?? 0})
                                  </div>
                                  <pre class="max-h-40 overflow-auto rounded bg-gray-100 p-2 text-xs dark:bg-gray-800">
                                    {JSON.stringify(res.tool_calls, null, 2)}
                                  </pre>
                                </div>
                              </Show>

                              {/* Functional Test Output */}
                              <Show when={res.functional_test_output}>
                                <div>
                                  <div class="mb-1 text-xs font-semibold text-gray-500">
                                    Functional Test Output
                                  </div>
                                  <pre class="max-h-32 overflow-auto rounded bg-gray-100 p-2 text-xs dark:bg-gray-800">
                                    {res.functional_test_output}
                                  </pre>
                                </div>
                              </Show>

                              {/* Evaluator Scores */}
                              <Show
                                when={
                                  res.evaluator_scores &&
                                  Object.keys(res.evaluator_scores).length > 0
                                }
                              >
                                <div>
                                  <div class="mb-1 text-xs font-semibold text-gray-500">
                                    Evaluator Scores
                                  </div>
                                  <div class="flex flex-wrap gap-2">
                                    <For each={Object.entries(res.evaluator_scores ?? {})}>
                                      {([evalName, scores]) => (
                                        <div class="rounded border px-2 py-1 text-xs dark:border-gray-700">
                                          <span class="font-medium">{evalName}:</span>{" "}
                                          <For
                                            each={Object.entries(scores as Record<string, number>)}
                                          >
                                            {([k, v]) => (
                                              <span class="ml-1">
                                                {k}={v.toFixed(3)}
                                              </span>
                                            )}
                                          </For>
                                        </div>
                                      )}
                                    </For>
                                  </div>
                                </div>
                              </Show>

                              {/* Files Changed */}
                              <Show when={res.files_changed?.length}>
                                <div>
                                  <div class="mb-1 text-xs font-semibold text-gray-500">
                                    Files Changed ({res.files_changed?.length ?? 0})
                                  </div>
                                  <div class="flex flex-wrap gap-1">
                                    <For each={res.files_changed ?? []}>
                                      {(f) => (
                                        <span class="rounded bg-blue-50 px-1.5 py-0.5 text-xs font-mono dark:bg-blue-900/30">
                                          {f}
                                        </span>
                                      )}
                                    </For>
                                  </div>
                                </div>
                              </Show>
                            </div>
                          </td>
                        </tr>
                      </Show>
                    </>
                  );
                }}
              </For>
            </tbody>
          </table>
        </Show>
      </Show>
    </div>
  );
}
