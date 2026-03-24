import { For, Match, onMount, Show, Switch } from "solid-js";

import { api } from "~/api/client";
import type { BenchmarkRun, BenchmarkType, RoutingReport as RoutingReportType } from "~/api/types";
import { benchmarkStatusVariant, getVariant } from "~/config/statusVariants";
import { useI18n } from "~/i18n";
import {
  Badge,
  Button,
  Card,
  Checkbox,
  CostDisplay,
  EmptyState,
  FormField,
  LoadingState,
  ModelCombobox,
  PageLayout,
  Select,
  Tabs,
} from "~/ui";
import { ChartTrophyIcon } from "~/ui/icons/EmptyStateIcons";

function isRoutingReport(v: unknown): v is RoutingReportType {
  return typeof v === "object" && v !== null && "models_used" in v;
}

import { BenchmarkCompare } from "./BenchmarkCompare";
import { BenchmarkLiveFeed } from "./BenchmarkLiveFeed";
import { BenchmarkRunDetail } from "./BenchmarkRunDetail";
import { CostAnalysisView } from "./CostAnalysisView";
import { LeaderboardView } from "./LeaderboardView";
import { emptyLiveFeedState } from "./liveFeedState";
import { MultiCompareView } from "./MultiCompareView";
import { PromptOptimizationPanel } from "./PromptOptimizationPanel";
import { RoutingReport } from "./RoutingReport";
import { SuiteManagement } from "./SuiteManagement";
import { TaskSettings } from "./TaskSettings";
import { useBenchmarkPage } from "./useBenchmarkPage";

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const METRIC_OPTIONS = [
  "correctness",
  "tool_correctness",
  "faithfulness",
  "answer_relevancy",
  "contextual_precision",
];

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export default function BenchmarkPage() {
  onMount(() => {
    document.title = "Benchmarks - CodeForge";
  });

  const { t } = useI18n();
  const state = useBenchmarkPage();

  return (
    <PageLayout title={t("benchmark.title")} description={t("benchmark.subtitle")}>
      {/* Tab navigation */}
      <Tabs
        items={state.tabItems()}
        value={state.activeTab()}
        onChange={state.setActiveTab}
        class="mb-6"
      />

      <Switch>
        {/* ============ RUNS TAB ============ */}
        <Match when={state.activeTab() === "runs"}>
          <div class="mb-4 flex gap-2">
            <Button onClick={() => state.setShowForm(!state.showForm())} size="sm">
              {state.showForm() ? t("common.cancel") : t("benchmark.newRun")}
            </Button>
          </div>

          {/* New Run Form */}
          <Show when={state.showForm()}>
            <Card class="mb-6 p-4">
              <form onSubmit={state.handleCreate} class="space-y-4">
                <FormField label={t("benchmark.suite")} id="benchmark-suite">
                  <Select
                    value={state.form.state.suiteId}
                    onChange={(e) => state.handleSuiteChange(e.currentTarget.value)}
                  >
                    <option value="">{t("common.select")}</option>
                    <Show when={state.localSuites().length}>
                      <optgroup label={t("benchmark.suiteLocal")}>
                        <For each={state.localSuites()}>
                          {(s) => (
                            <option value={s.id}>
                              {s.name} ({s.task_count} tasks)
                            </option>
                          )}
                        </For>
                      </optgroup>
                    </Show>
                    <Show when={state.externalSuites().length}>
                      <optgroup label={t("benchmark.suiteExternal")}>
                        <For each={state.externalSuites()}>
                          {(s) => (
                            <option value={s.id}>
                              {s.name} ({s.task_count} tasks)
                            </option>
                          )}
                        </For>
                      </optgroup>
                    </Show>
                  </Select>
                </FormField>

                <Show when={state.selectedSuite()}>
                  {(suite) => (
                    <TaskSettings
                      providerName={suite().provider_name}
                      config={state.form.state.providerConfig}
                      onChange={(c) => state.form.setState("providerConfig", c)}
                      taskCount={suite().task_count}
                    />
                  )}
                </Show>

                <FormField label={t("benchmark.model")} id="benchmark-model">
                  <div class="space-y-2">
                    <Checkbox
                      label={t("benchmark.modelAuto")}
                      checked={state.form.state.model === "auto"}
                      onChange={(v) => state.form.setState("model", v ? "auto" : "")}
                    />
                    <Show when={state.form.state.model !== "auto"}>
                      <ModelCombobox
                        id="benchmark-model"
                        value={state.form.state.model}
                        onInput={(v) => state.form.setState("model", v)}
                        required
                      />
                    </Show>
                  </div>
                </FormField>

                <FormField label={t("benchmark.benchmarkType")} id="benchmark-type">
                  <Select
                    value={state.form.state.benchmarkType}
                    onChange={(e) =>
                      state.form.setState("benchmarkType", e.currentTarget.value as BenchmarkType)
                    }
                  >
                    <option value="simple">{`Simple (prompt \u2192 output)`}</option>
                    <option value="tool_use">Tool Use (with tool calling)</option>
                    <option value="agent">Agent (multi-turn, exec modes)</option>
                  </Select>
                </FormField>

                <Show when={state.form.state.benchmarkType === "agent"}>
                  <FormField label={t("benchmark.execMode")} id="benchmark-exec-mode">
                    <Select
                      value={state.form.state.execMode}
                      onChange={(e) =>
                        state.form.setState(
                          "execMode",
                          e.currentTarget.value as "mount" | "sandbox" | "hybrid",
                        )
                      }
                    >
                      <option value="mount">Mount (direct file access)</option>
                      <option value="sandbox">Sandbox (isolated container)</option>
                      <option value="hybrid">Hybrid</option>
                    </Select>
                  </FormField>
                </Show>

                <FormField label={t("benchmark.metrics")} id="benchmark-metrics">
                  <div class="flex flex-wrap gap-2">
                    <For each={METRIC_OPTIONS}>
                      {(m) => (
                        <Button
                          variant="pill"
                          size="xs"
                          class={
                            state.form.state.metrics.includes(m)
                              ? "border-cf-accent bg-cf-accent/10 text-cf-accent"
                              : ""
                          }
                          onClick={() => state.toggleMetric(m)}
                        >
                          {m}
                        </Button>
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
          <Show when={!state.runs.loading} fallback={<LoadingState />}>
            <Show
              when={state.runs()?.length}
              fallback={
                <EmptyState
                  illustration={<ChartTrophyIcon />}
                  title={t("benchmark.empty")}
                  description={t("benchmark.emptyDescription")}
                />
              }
            >
              <div class="space-y-3">
                <For each={state.runs()}>
                  {(run: BenchmarkRun) => (
                    <div
                      class={`cursor-pointer transition hover:ring-1 hover:ring-blue-400 ${
                        state.selectedRun() === run.id ? "ring-2 ring-blue-500" : ""
                      }`}
                      onClick={(e: MouseEvent) => {
                        const target = e.target as HTMLElement;
                        if (
                          target.closest("table") ||
                          target.closest("button") ||
                          target.closest("a")
                        )
                          return;
                        state.setSelectedRun(state.selectedRun() === run.id ? null : run.id);
                      }}
                    >
                      <Card class="p-4">
                        <div class="flex items-center justify-between">
                          <div class="flex items-center gap-2">
                            <span class="font-medium">{run.dataset}</span>
                            <span class="text-sm text-gray-500">{run.model}</span>
                            <Show when={run.benchmark_type}>
                              <Badge
                                variant={
                                  run.benchmark_type === "agent"
                                    ? "info"
                                    : run.benchmark_type === "tool_use"
                                      ? "warning"
                                      : "default"
                                }
                              >
                                {run.benchmark_type}
                              </Badge>
                            </Show>
                            <Show when={run.exec_mode}>
                              <Badge variant="default">{run.exec_mode}</Badge>
                            </Show>
                          </div>
                          <div class="flex items-center gap-2">
                            <Badge
                              variant={getVariant(benchmarkStatusVariant, run.status, "warning")}
                            >
                              {run.status}
                            </Badge>
                            <span class="text-xs text-gray-400">
                              {state.formatDuration(run.total_duration_ms)}
                            </span>
                            <CostDisplay usd={run.total_cost} class="text-xs text-gray-400" />
                            <Show when={run.status === "running"}>
                              <Button
                                size="sm"
                                variant="danger"
                                onClick={(e: MouseEvent) => {
                                  e.stopPropagation();
                                  state.handleCancel(run.id);
                                }}
                              >
                                {t("common.cancel")}
                              </Button>
                            </Show>
                            <Show when={run.status === "completed"}>
                              <a
                                href={api.benchmarks.exportResultsUrl(run.id, "csv")}
                                target="_blank"
                                rel="noopener noreferrer"
                                class="rounded bg-gray-200 px-2 py-1 text-xs hover:bg-gray-300 dark:bg-gray-700 dark:hover:bg-gray-600"
                                onClick={(e: MouseEvent) => e.stopPropagation()}
                              >
                                CSV
                              </a>
                            </Show>
                            <Button
                              size="sm"
                              variant="danger"
                              onClick={(e: MouseEvent) => {
                                e.stopPropagation();
                                state.handleDelete(run.id);
                              }}
                            >
                              {t("common.delete")}
                            </Button>
                          </div>
                        </div>

                        <Show when={run.metrics?.length}>
                          <div class="mt-2 flex gap-1">
                            <For each={run.metrics}>
                              {(m) => <Badge variant="default">{m}</Badge>}
                            </For>
                          </div>
                        </Show>

                        {/* Minimal pulse bar for non-selected running runs */}
                        <Show when={run.status === "running" && state.selectedRun() !== run.id}>
                          <div class="mt-2">
                            <div class="h-1.5 w-full overflow-hidden rounded-full bg-cf-bg-secondary">
                              <div
                                class="h-1.5 animate-pulse rounded-full bg-cf-accent"
                                style={{ width: "100%" }}
                              />
                            </div>
                            <span class="mt-1 text-xs text-cf-text-muted">Running...</span>
                          </div>
                        </Show>

                        {/* Summary Scores */}
                        <Show
                          when={run.summary_scores && Object.keys(run.summary_scores).length > 0}
                        >
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
                        <Show when={state.selectedRun() === run.id}>
                          <Show when={run.status === "running"}>
                            <BenchmarkLiveFeed
                              state={state.liveFeedStates().get(run.id) ?? emptyLiveFeedState()}
                              startedAt={run.created_at}
                            />
                          </Show>
                          <BenchmarkRunDetail
                            results={state.results()}
                            loading={state.results.loading}
                            formatDuration={state.formatDuration}
                          />
                          <Show
                            when={(() => {
                              if (run.model !== "auto" || run.status !== "completed")
                                return undefined;
                              const candidate: unknown = run.summary_scores?.routing_report;
                              return isRoutingReport(candidate) ? candidate : undefined;
                            })()}
                          >
                            {(report) => <RoutingReport report={report()} />}
                          </Show>
                          <Show when={run.status === "completed"}>
                            <PromptOptimizationPanel runId={run.id} suiteId={run.suite_id ?? ""} />
                          </Show>
                        </Show>
                      </Card>
                    </div>
                  )}
                </For>
              </div>
            </Show>
          </Show>

          {/* Compare Section (2-run) */}
          <BenchmarkCompare runs={state.runs() ?? []} />
        </Match>

        {/* ============ LEADERBOARD TAB ============ */}
        <Match when={state.activeTab() === "leaderboard"}>
          <LeaderboardView />
        </Match>

        {/* ============ COST ANALYSIS TAB ============ */}
        <Match when={state.activeTab() === "costAnalysis"}>
          <CostAnalysisView runs={state.runs() ?? []} />
        </Match>

        {/* ============ MULTI-COMPARE TAB ============ */}
        <Match when={state.activeTab() === "multiCompare"}>
          <MultiCompareView runs={state.runs() ?? []} />
        </Match>

        {/* ============ SUITES TAB ============ */}
        <Match when={state.activeTab() === "suites"}>
          <SuiteManagement />
        </Match>
      </Switch>
    </PageLayout>
  );
}
