import { createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type { BenchmarkRun, MultiCompareEntry } from "~/api/types";
import { useToast } from "~/components/Toast";
import { useI18n } from "~/i18n";
import { Badge, Button, Card, Checkbox, CostDisplay, EmptyState } from "~/ui";

interface MultiCompareViewProps {
  runs: BenchmarkRun[];
}

const CHART_COLORS = [
  "#3B82F6", // blue
  "#EF4444", // red
  "#10B981", // green
  "#F59E0B", // amber
  "#8B5CF6", // violet
  "#EC4899", // pink
  "#06B6D4", // cyan
  "#F97316", // orange
];

/** Render an SVG radar/spider chart for multi-model metric comparison. */
function RadarChart(props: {
  entries: MultiCompareEntry[];
  metrics: string[];
  avgScore: (entry: MultiCompareEntry, metric: string) => string;
}) {
  const cx = 150;
  const cy = 150;
  const radius = 120;
  const levels = 5;

  const angleStep = () => (2 * Math.PI) / Math.max(props.metrics.length, 1);

  const pointOnAxis = (axisIdx: number, value: number) => {
    const angle = axisIdx * angleStep() - Math.PI / 2;
    const r = radius * Math.min(Math.max(value, 0), 1);
    return { x: cx + r * Math.cos(angle), y: cy + r * Math.sin(angle) };
  };

  const polygonPoints = (entry: MultiCompareEntry): string =>
    props.metrics
      .map((metric, i) => {
        const val = parseFloat(props.avgScore(entry, metric));
        const p = pointOnAxis(i, isNaN(val) ? 0 : val);
        return `${p.x},${p.y}`;
      })
      .join(" ");

  return (
    <div class="flex items-center gap-4">
      <svg viewBox="0 0 300 300" class="h-64 w-64 flex-shrink-0">
        {/* Grid levels */}
        <For each={Array.from({ length: levels }, (_, i) => (i + 1) / levels)}>
          {(level) => (
            <polygon
              points={props.metrics
                .map((_, i) => {
                  const p = pointOnAxis(i, level);
                  return `${p.x},${p.y}`;
                })
                .join(" ")}
              fill="none"
              stroke="currentColor"
              stroke-opacity="0.15"
              class="text-gray-500"
            />
          )}
        </For>

        {/* Axis lines */}
        <For each={props.metrics}>
          {(_, i) => {
            const p = pointOnAxis(i(), 1);
            return (
              <line
                x1={cx}
                y1={cy}
                x2={p.x}
                y2={p.y}
                stroke="currentColor"
                stroke-opacity="0.2"
                class="text-gray-500"
              />
            );
          }}
        </For>

        {/* Axis labels */}
        <For each={props.metrics}>
          {(metric, i) => {
            const p = pointOnAxis(i(), 1.15);
            return (
              <text
                x={p.x}
                y={p.y}
                text-anchor="middle"
                dominant-baseline="central"
                font-size="9"
                fill="currentColor"
                class="text-gray-600 dark:text-gray-400"
              >
                {metric.length > 12 ? metric.slice(0, 10) + ".." : metric}
              </text>
            );
          }}
        </For>

        {/* Data polygons */}
        <For each={props.entries}>
          {(entry, idx) => (
            <polygon
              points={polygonPoints(entry)}
              fill={CHART_COLORS[idx() % CHART_COLORS.length]}
              fill-opacity="0.15"
              stroke={CHART_COLORS[idx() % CHART_COLORS.length]}
              stroke-width="2"
            />
          )}
        </For>

        {/* Data points */}
        <For each={props.entries}>
          {(entry, idx) => (
            <For each={props.metrics}>
              {(metric, mi) => {
                const val = parseFloat(props.avgScore(entry, metric));
                const p = pointOnAxis(mi(), isNaN(val) ? 0 : val);
                return (
                  <circle
                    cx={p.x}
                    cy={p.y}
                    r="3"
                    fill={CHART_COLORS[idx() % CHART_COLORS.length]}
                  />
                );
              }}
            </For>
          )}
        </For>
      </svg>

      {/* Legend */}
      <div class="space-y-1.5">
        <For each={props.entries}>
          {(entry, idx) => (
            <div class="flex items-center gap-2 text-xs">
              <span
                class="inline-block h-3 w-3 rounded-full"
                style={{ background: CHART_COLORS[idx() % CHART_COLORS.length] }}
              />
              <span class="font-medium">{entry.run.model}</span>
              <span class="text-gray-500">{entry.run.dataset}</span>
            </div>
          )}
        </For>
      </div>
    </div>
  );
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

      {/* Results: Radar chart + table */}
      <Show when={result()}>
        {(entries) => (
          <>
            {/* SVG Radar chart */}
            <Show when={metricNames().length >= 3}>
              <Card class="p-4">
                <RadarChart entries={entries()} metrics={metricNames()} avgScore={avgScore} />
              </Card>
            </Show>

            {/* Table */}
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
          </>
        )}
      </Show>

      <Show when={!result() && !loading()}>
        <EmptyState title={t("benchmark.multiCompare.noResults")} />
      </Show>
    </div>
  );
}
