import { createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type { BenchmarkCompareResult, BenchmarkResult, BenchmarkRun } from "~/api/types";
import { useToast } from "~/components/Toast";
import { useI18n } from "~/i18n";
import { Button, Card, FormField, Select } from "~/ui";

interface BenchmarkCompareProps {
  runs: BenchmarkRun[];
}

export function BenchmarkCompare(props: BenchmarkCompareProps) {
  const { t } = useI18n();
  const { show: toast } = useToast();
  const [compareA, setCompareA] = createSignal<string>("");
  const [compareB, setCompareB] = createSignal<string>("");
  const [compareResult, setCompareResult] = createSignal<BenchmarkCompareResult | null>(null);

  return (
    <Show when={props.runs.length >= 2}>
      <Card class="mt-6 p-4">
        <h3 class="mb-3 text-sm font-semibold">{t("benchmark.compare")}</h3>
        <div class="flex items-end gap-3">
          <FormField label={t("benchmark.runA")} id="benchmark-compare-a">
            <Select value={compareA()} onChange={(e) => setCompareA(e.currentTarget.value)}>
              <option value="">{t("common.select")}</option>
              <For each={props.runs}>
                {(r: BenchmarkRun) => (
                  <option value={r.id}>
                    {r.dataset} / {r.model} ({r.status})
                  </option>
                )}
              </For>
            </Select>
          </FormField>
          <FormField label={t("benchmark.runB")} id="benchmark-compare-b">
            <Select value={compareB()} onChange={(e) => setCompareB(e.currentTarget.value)}>
              <option value="">{t("common.select")}</option>
              <For each={props.runs}>
                {(r: BenchmarkRun) => (
                  <option value={r.id}>
                    {r.dataset} / {r.model} ({r.status})
                  </option>
                )}
              </For>
            </Select>
          </FormField>
          <Button
            size="sm"
            variant="primary"
            disabled={!compareA() || !compareB() || compareA() === compareB()}
            onClick={async () => {
              try {
                const result = await api.benchmarks.compare(compareA(), compareB());
                setCompareResult(result);
                toast("success", t("benchmark.toast.compareReady"));
              } catch {
                toast("error", t("benchmark.toast.compareError"));
              }
            }}
          >
            {t("benchmark.compareBtn")}
          </Button>
        </div>
      </Card>

      <Show when={compareResult()}>{(cr) => <CompareResultsTable result={cr()} />}</Show>
    </Show>
  );
}

function CompareResultsTable(props: { result: BenchmarkCompareResult }) {
  const { t } = useI18n();

  const metricNames = () => {
    const names = new Set<string>();
    for (const r of [...props.result.results_a, ...props.result.results_b]) {
      for (const key of Object.keys(r.scores)) {
        names.add(key);
      }
    }
    return [...names].sort();
  };

  const avgScore = (results: BenchmarkResult[], metric: string): string => {
    const vals = results.map((r) => r.scores[metric]).filter((v) => v !== undefined);
    if (vals.length === 0) return "-";
    return (vals.reduce((a, b) => a + b, 0) / vals.length).toFixed(3);
  };

  return (
    <Card class="mt-4 overflow-x-auto p-4">
      <h4 class="mb-3 text-sm font-semibold">{t("benchmark.compareResults")}</h4>
      <table class="w-full text-left text-sm">
        <thead>
          <tr class="border-b text-xs text-gray-500">
            <th class="pb-2 pr-4">{t("benchmark.metric")}</th>
            <th class="pb-2 pr-4">
              {props.result.run_a.dataset} / {props.result.run_a.model}
            </th>
            <th class="pb-2">
              {props.result.run_b.dataset} / {props.result.run_b.model}
            </th>
          </tr>
        </thead>
        <tbody>
          <For each={metricNames()}>
            {(metric) => (
              <tr class="border-b last:border-0">
                <td class="py-1.5 pr-4 font-medium">{metric}</td>
                <td class="py-1.5 pr-4 font-mono">{avgScore(props.result.results_a, metric)}</td>
                <td class="py-1.5 font-mono">{avgScore(props.result.results_b, metric)}</td>
              </tr>
            )}
          </For>
        </tbody>
      </table>
    </Card>
  );
}
