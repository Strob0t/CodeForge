import { createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type { BenchmarkRun, MultiCompareEntry } from "~/api/types";
import { useToast } from "~/components/Toast";
import { useI18n } from "~/i18n";
import { Badge, Button, Card, Checkbox, CostDisplay, EmptyState } from "~/ui";

interface MultiCompareViewProps {
  runs: BenchmarkRun[];
}

export function MultiCompareView(props: MultiCompareViewProps) {
  const { t } = useI18n();
  const { show: toast } = useToast();
  const [selected, setSelected] = createSignal<Set<string>>(new Set());
  const [result, setResult] = createSignal<MultiCompareEntry[] | null>(null);
  const [loading, setLoading] = createSignal(false);

  const toggle = (id: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const handleCompare = async () => {
    const ids = [...selected()];
    if (ids.length < 2) return;
    setLoading(true);
    try {
      const data = await api.benchmarks.compareMulti(ids);
      setResult(data);
      toast("success", t("benchmark.toast.compareReady"));
    } catch {
      toast("error", t("benchmark.toast.compareError"));
    } finally {
      setLoading(false);
    }
  };

  /** Extract all unique metric names across all entries */
  const metricNames = (): string[] => {
    const names = new Set<string>();
    for (const entry of result() ?? []) {
      for (const r of entry.results) {
        for (const key of Object.keys(r.scores)) {
          names.add(key);
        }
      }
    }
    return [...names].sort();
  };

  const avgScore = (entry: MultiCompareEntry, metric: string): string => {
    const vals = entry.results.map((r) => r.scores[metric]).filter((v) => v !== undefined);
    if (vals.length === 0) return "-";
    return (vals.reduce((a, b) => a + b, 0) / vals.length).toFixed(3);
  };

  const bestForMetric = (metric: string): string => {
    let best = "";
    let bestVal = -1;
    for (const entry of result() ?? []) {
      const val = parseFloat(avgScore(entry, metric));
      if (!isNaN(val) && val > bestVal) {
        bestVal = val;
        best = entry.run.id;
      }
    }
    return best;
  };

  return (
    <div class="space-y-4">
      {/* Run selector with checkboxes */}
      <Card class="p-4">
        <div class="mb-3 text-sm font-medium">{t("benchmark.multiCompare.selectRuns")}</div>
        <div class="max-h-48 space-y-1.5 overflow-y-auto">
          <For each={props.runs}>
            {(run: BenchmarkRun) => (
              <label class="flex cursor-pointer items-center gap-2 rounded px-2 py-1 text-sm hover:bg-gray-50 dark:hover:bg-gray-800">
                <Checkbox checked={selected().has(run.id)} onChange={() => toggle(run.id)} />
                <span class="font-medium">{run.dataset}</span>
                <span class="text-gray-500">/ {run.model}</span>
                <Badge variant="default" class="ml-auto">
                  {run.status}
                </Badge>
              </label>
            )}
          </For>
        </div>
        <div class="mt-3">
          <Button
            size="sm"
            variant="primary"
            disabled={selected().size < 2 || loading()}
            onClick={handleCompare}
          >
            {loading() ? "..." : t("benchmark.multiCompare.compareBtn")} ({selected().size})
          </Button>
        </div>
      </Card>

      {/* Results table */}
      <Show when={result()}>
        {(entries) => (
          <Card class="overflow-x-auto p-0">
            <table class="w-full text-left text-sm">
              <thead>
                <tr class="border-b text-xs text-gray-500 dark:border-gray-700">
                  <th class="px-4 py-3">{t("benchmark.metric")}</th>
                  <For each={entries()}>
                    {(entry) => (
                      <th class="px-4 py-3 text-right">
                        <div>{entry.run.model}</div>
                        <div class="font-normal text-gray-400">{entry.run.dataset}</div>
                      </th>
                    )}
                  </For>
                </tr>
              </thead>
              <tbody>
                {/* Cost row */}
                <tr class="border-b dark:border-gray-700">
                  <td class="px-4 py-2 font-medium">{t("benchmark.cost")}</td>
                  <For each={entries()}>
                    {(entry) => (
                      <td class="px-4 py-2 text-right font-mono text-xs">
                        <CostDisplay usd={entry.run.total_cost} />
                      </td>
                    )}
                  </For>
                </tr>
                {/* Duration row */}
                <tr class="border-b dark:border-gray-700">
                  <td class="px-4 py-2 font-medium">{t("benchmark.duration")}</td>
                  <For each={entries()}>
                    {(entry) => (
                      <td class="px-4 py-2 text-right text-xs">
                        {entry.run.total_duration_ms < 1000
                          ? `${entry.run.total_duration_ms}ms`
                          : `${(entry.run.total_duration_ms / 1000).toFixed(1)}s`}
                      </td>
                    )}
                  </For>
                </tr>
                {/* Metric rows */}
                <For each={metricNames()}>
                  {(metric) => {
                    const best = bestForMetric(metric);
                    return (
                      <tr class="border-b last:border-0 dark:border-gray-700">
                        <td class="px-4 py-2 font-medium">{metric}</td>
                        <For each={entries()}>
                          {(entry) => (
                            <td
                              class={`px-4 py-2 text-right font-mono ${
                                entry.run.id === best
                                  ? "font-bold text-green-600 dark:text-green-400"
                                  : ""
                              }`}
                            >
                              {avgScore(entry, metric)}
                            </td>
                          )}
                        </For>
                      </tr>
                    );
                  }}
                </For>
              </tbody>
            </table>
          </Card>
        )}
      </Show>

      <Show when={!result() && !loading()}>
        <EmptyState title={t("benchmark.multiCompare.noResults")} />
      </Show>
    </div>
  );
}
