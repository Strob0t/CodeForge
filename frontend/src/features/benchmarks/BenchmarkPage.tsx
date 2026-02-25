import { createResource, createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type {
  BenchmarkDatasetInfo,
  BenchmarkResult,
  BenchmarkRun,
  CreateBenchmarkRunRequest,
} from "~/api/types";
import { useToast } from "~/components/Toast";
import { useI18n } from "~/i18n";
import {
  Badge,
  Button,
  Card,
  EmptyState,
  FormField,
  Input,
  LoadingState,
  PageLayout,
  Select,
} from "~/ui";

const METRIC_OPTIONS = [
  "correctness",
  "tool_correctness",
  "faithfulness",
  "answer_relevancy",
  "contextual_precision",
];

export default function BenchmarkPage() {
  const { t } = useI18n();
  const { show: toast } = useToast();
  const [runs, { refetch }] = createResource(() => api.benchmarks.listRuns());
  const [datasets] = createResource(() => api.benchmarks.listDatasets());

  // New run form
  const [showForm, setShowForm] = createSignal(false);
  const [dataset, setDataset] = createSignal("");
  const [model, setModel] = createSignal("");
  const [metrics, setMetrics] = createSignal<string[]>(["correctness"]);

  // Run detail
  const [selectedRun, setSelectedRun] = createSignal<string | null>(null);
  const [results] = createResource(selectedRun, (id) =>
    id ? api.benchmarks.listResults(id) : undefined,
  );

  // Compare
  const [compareA, setCompareA] = createSignal<string>("");
  const [compareB, setCompareB] = createSignal<string>("");

  const resetForm = () => {
    setDataset("");
    setModel("");
    setMetrics(["correctness"]);
  };

  const handleCreate = async (e: SubmitEvent) => {
    e.preventDefault();
    const req: CreateBenchmarkRunRequest = {
      dataset: dataset(),
      model: model(),
      metrics: metrics(),
    };
    try {
      await api.benchmarks.createRun(req);
      toast("success", t("benchmark.toast.created"));
      setShowForm(false);
      resetForm();
      refetch();
    } catch {
      toast("error", t("benchmark.toast.createError"));
    }
  };

  const handleDelete = async (id: string) => {
    try {
      await api.benchmarks.deleteRun(id);
      toast("success", t("benchmark.toast.deleted"));
      if (selectedRun() === id) setSelectedRun(null);
      refetch();
    } catch {
      toast("error", t("benchmark.toast.deleteError"));
    }
  };

  const statusVariant = (s: BenchmarkRun["status"]) => {
    switch (s) {
      case "completed":
        return "success" as const;
      case "failed":
        return "danger" as const;
      default:
        return "warning" as const;
    }
  };

  const formatDuration = (ms: number) => {
    if (ms < 1000) return `${ms}ms`;
    return `${(ms / 1000).toFixed(1)}s`;
  };

  const toggleMetric = (m: string) => {
    setMetrics((prev) => (prev.includes(m) ? prev.filter((x) => x !== m) : [...prev, m]));
  };

  return (
    <PageLayout title={t("benchmark.title")} description={t("benchmark.subtitle")}>
      <div class="mb-4 flex gap-2">
        <Button onClick={() => setShowForm(!showForm())} size="sm">
          {showForm() ? t("common.cancel") : t("benchmark.newRun")}
        </Button>
      </div>

      {/* New Run Form */}
      <Show when={showForm()}>
        <Card class="mb-6 p-4">
          <form onSubmit={handleCreate} class="space-y-4">
            <FormField label={t("benchmark.dataset")} id="benchmark-dataset">
              <Show
                when={datasets()?.length}
                fallback={
                  <Input
                    value={dataset()}
                    onInput={(e) => setDataset(e.currentTarget.value)}
                    placeholder="basic-coding"
                    required
                  />
                }
              >
                <Select value={dataset()} onChange={(e) => setDataset(e.currentTarget.value)}>
                  <option value="">{t("common.select")}</option>
                  <For each={datasets()}>
                    {(d: BenchmarkDatasetInfo) => (
                      <option value={d.path}>
                        {d.name} ({d.task_count} tasks)
                      </option>
                    )}
                  </For>
                </Select>
              </Show>
            </FormField>

            <FormField label={t("benchmark.model")} id="benchmark-model">
              <Input
                value={model()}
                onInput={(e) => setModel(e.currentTarget.value)}
                placeholder="openai/gpt-4o"
                required
              />
            </FormField>

            <FormField label={t("benchmark.metrics")} id="benchmark-metrics">
              <div class="flex flex-wrap gap-2">
                <For each={METRIC_OPTIONS}>
                  {(m) => (
                    <button
                      type="button"
                      class={`rounded px-2 py-1 text-xs font-medium transition ${
                        metrics().includes(m)
                          ? "bg-blue-600 text-white"
                          : "bg-gray-200 text-gray-700 dark:bg-gray-700 dark:text-gray-300"
                      }`}
                      onClick={() => toggleMetric(m)}
                    >
                      {m}
                    </button>
                  )}
                </For>
              </div>
            </FormField>

            <Button type="submit" variant="primary" size="sm">
              {t("benchmark.startRun")}
            </Button>
          </form>
        </Card>
      </Show>

      {/* Run List */}
      <Show when={!runs.loading} fallback={<LoadingState />}>
        <Show when={runs()?.length} fallback={<EmptyState title={t("benchmark.empty")} />}>
          <div class="space-y-3">
            <For each={runs()}>
              {(run: BenchmarkRun) => (
                <div
                  class={`cursor-pointer transition hover:ring-1 hover:ring-blue-400 ${
                    selectedRun() === run.id ? "ring-2 ring-blue-500" : ""
                  }`}
                  onClick={() => setSelectedRun(selectedRun() === run.id ? null : run.id)}
                >
                  <Card class="p-4">
                    <div class="flex items-center justify-between">
                      <div>
                        <span class="font-medium">{run.dataset}</span>
                        <span class="ml-2 text-sm text-gray-500">{run.model}</span>
                      </div>
                      <div class="flex items-center gap-2">
                        <Badge variant={statusVariant(run.status)}>{run.status}</Badge>
                        <span class="text-xs text-gray-400">
                          {formatDuration(run.total_duration_ms)}
                        </span>
                        <span class="text-xs text-gray-400">${run.total_cost.toFixed(4)}</span>
                        <Button
                          size="sm"
                          variant="danger"
                          onClick={(e: MouseEvent) => {
                            e.stopPropagation();
                            handleDelete(run.id);
                          }}
                        >
                          {t("common.delete")}
                        </Button>
                      </div>
                    </div>

                    <Show when={run.metrics?.length}>
                      <div class="mt-2 flex gap-1">
                        <For each={run.metrics}>{(m) => <Badge variant="default">{m}</Badge>}</For>
                      </div>
                    </Show>

                    {/* Summary Scores */}
                    <Show when={run.summary_scores && Object.keys(run.summary_scores).length > 0}>
                      <div class="mt-2 flex gap-3 text-sm">
                        <For each={Object.entries(run.summary_scores)}>
                          {([key, val]) => (
                            <span>
                              <span class="text-gray-500">{key}:</span>{" "}
                              <span class="font-mono">{(val as number).toFixed(3)}</span>
                            </span>
                          )}
                        </For>
                      </div>
                    </Show>

                    {/* Expanded Results */}
                    <Show when={selectedRun() === run.id}>
                      <div class="mt-4 border-t pt-4 dark:border-gray-700">
                        <Show when={!results.loading} fallback={<LoadingState />}>
                          <Show
                            when={results()?.length}
                            fallback={
                              <p class="text-sm text-gray-500">{t("benchmark.noResults")}</p>
                            }
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
                                <For each={results()}>
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
                                      <td class="py-2 font-mono text-xs">
                                        ${res.cost_usd.toFixed(4)}
                                      </td>
                                      <td class="py-2 text-xs">
                                        {formatDuration(res.duration_ms)}
                                      </td>
                                    </tr>
                                  )}
                                </For>
                              </tbody>
                            </table>
                          </Show>
                        </Show>
                      </div>
                    </Show>
                  </Card>
                </div>
              )}
            </For>
          </div>
        </Show>
      </Show>

      {/* Compare Section */}
      <Show when={(runs()?.length ?? 0) >= 2}>
        <Card class="mt-6 p-4">
          <h3 class="mb-3 text-sm font-semibold">{t("benchmark.compare")}</h3>
          <div class="flex items-end gap-3">
            <FormField label={t("benchmark.runA")} id="benchmark-compare-a">
              <Select value={compareA()} onChange={(e) => setCompareA(e.currentTarget.value)}>
                <option value="">{t("common.select")}</option>
                <For each={runs()}>
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
                <For each={runs()}>
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
                  console.log("Compare result:", result);
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
      </Show>

      {/* Datasets Info */}
      <Show when={datasets()?.length}>
        <Card class="mt-6 p-4">
          <h3 class="mb-3 text-sm font-semibold">{t("benchmark.datasets")}</h3>
          <div class="space-y-2">
            <For each={datasets()}>
              {(d: BenchmarkDatasetInfo) => (
                <div class="flex items-center justify-between text-sm">
                  <div>
                    <span class="font-medium">{d.name}</span>
                    <Show when={d.description}>
                      <span class="ml-2 text-gray-500">{d.description}</span>
                    </Show>
                  </div>
                  <Badge variant="default">
                    {d.task_count} {t("benchmark.tasks")}
                  </Badge>
                </div>
              )}
            </For>
          </div>
        </Card>
      </Show>
    </PageLayout>
  );
}
