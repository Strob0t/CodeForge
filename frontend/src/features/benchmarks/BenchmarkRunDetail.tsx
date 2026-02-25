import { For, Show } from "solid-js";

import type { BenchmarkResult } from "~/api/types";
import { useI18n } from "~/i18n";
import { LoadingState } from "~/ui";

interface BenchmarkRunDetailProps {
  results: BenchmarkResult[] | undefined;
  loading: boolean;
  formatDuration: (ms: number) => string;
}

export function BenchmarkRunDetail(props: BenchmarkRunDetailProps) {
  const { t } = useI18n();

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
              </tr>
            </thead>
            <tbody>
              <For each={props.results}>
                {(res: BenchmarkResult) => (
                  <tr class="border-b dark:border-gray-700">
                    <td class="py-2 font-medium">{res.task_name}</td>
                    <td class="py-2">
                      <div class="flex gap-2">
                        <For each={Object.entries(res.scores)}>
                          {([k, v]) => (
                            <span class="rounded bg-gray-100 px-1.5 py-0.5 text-xs dark:bg-gray-800">
                              {k}: {(v as number).toFixed(3)}
                            </span>
                          )}
                        </For>
                      </div>
                    </td>
                    <td class="py-2 font-mono text-xs">${res.cost_usd.toFixed(4)}</td>
                    <td class="py-2 text-xs">{props.formatDuration(res.duration_ms)}</td>
                  </tr>
                )}
              </For>
            </tbody>
          </table>
        </Show>
      </Show>
    </div>
  );
}
