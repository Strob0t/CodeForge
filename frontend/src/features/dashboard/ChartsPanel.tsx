import { type Component, createResource, createSignal, Show } from "solid-js";

import { api } from "~/api/client";
import { useI18n } from "~/i18n";
import { Tabs } from "~/ui";

import AgentPerformanceBars from "./charts/AgentPerformanceBars";
import CostByProjectBars from "./charts/CostByProjectBars";
import CostTrendChart from "./charts/CostTrendChart";
import ModelUsagePie from "./charts/ModelUsagePie";
import RunOutcomesDonut from "./charts/RunOutcomesDonut";

type ChartTab = "cost-trend" | "run-outcomes" | "agents" | "models" | "cost-project";

const ChartsPanel: Component = () => {
  const { t } = useI18n();
  const [activeTab, setActiveTab] = createSignal<ChartTab>("cost-trend");
  const [trendDays, setTrendDays] = createSignal(30);

  const [costTrend] = createResource(trendDays, (days) => api.dashboard.costTrend(days));
  const [runOutcomes] = createResource(() => api.dashboard.runOutcomes(7));
  const [agentPerf] = createResource(() => api.dashboard.agentPerformance());
  const [modelUsage] = createResource(() => api.dashboard.modelUsage());
  const [costByProject] = createResource(() => api.dashboard.costByProject());

  const tabs = () => [
    { value: "cost-trend", label: t("dashboard.charts.costTrend") },
    { value: "run-outcomes", label: t("dashboard.charts.runOutcomes") },
    { value: "agents", label: t("dashboard.charts.agentPerf") },
    { value: "models", label: t("dashboard.charts.modelUsage") },
    { value: "cost-project", label: t("dashboard.charts.costByProject") },
  ];

  return (
    <div>
      <div class="mb-3 flex items-center justify-between">
        <h3 class="text-sm font-semibold text-[var(--cf-text-primary)]">
          {t("dashboard.charts.title")}
        </h3>
        <Show when={activeTab() === "cost-trend"}>
          <div class="flex gap-1">
            <button
              class="rounded px-2 py-0.5 text-xs"
              classList={{
                "bg-[var(--cf-accent)] text-[var(--cf-accent-fg)]": trendDays() === 7,
                "text-[var(--cf-text-muted)] hover:text-[var(--cf-text-primary)]":
                  trendDays() !== 7,
              }}
              onClick={() => setTrendDays(7)}
            >
              7d
            </button>
            <button
              class="rounded px-2 py-0.5 text-xs"
              classList={{
                "bg-[var(--cf-accent)] text-[var(--cf-accent-fg)]": trendDays() === 30,
                "text-[var(--cf-text-muted)] hover:text-[var(--cf-text-primary)]":
                  trendDays() !== 30,
              }}
              onClick={() => setTrendDays(30)}
            >
              30d
            </button>
          </div>
        </Show>
      </div>

      <Tabs
        items={tabs()}
        value={activeTab()}
        onChange={(v) => setActiveTab(v as ChartTab)}
        variant="pills"
      />

      <div class="mt-3">
        <Show when={activeTab() === "cost-trend"}>
          <Show when={costTrend()} fallback={<ChartPlaceholder />}>
            {(data) => <CostTrendChart data={data()} />}
          </Show>
        </Show>

        <Show when={activeTab() === "run-outcomes"}>
          <Show when={runOutcomes()} fallback={<ChartPlaceholder />}>
            {(data) => <RunOutcomesDonut data={data()} />}
          </Show>
        </Show>

        <Show when={activeTab() === "agents"}>
          <Show when={agentPerf()} fallback={<ChartPlaceholder />}>
            {(data) => <AgentPerformanceBars data={data()} />}
          </Show>
        </Show>

        <Show when={activeTab() === "models"}>
          <Show when={modelUsage()} fallback={<ChartPlaceholder />}>
            {(data) => <ModelUsagePie data={data()} />}
          </Show>
        </Show>

        <Show when={activeTab() === "cost-project"}>
          <Show when={costByProject()} fallback={<ChartPlaceholder />}>
            {(data) => <CostByProjectBars data={data()} />}
          </Show>
        </Show>
      </div>
    </div>
  );
};

const ChartPlaceholder: Component = () => (
  <div class="flex h-64 items-center justify-center text-sm text-[var(--cf-text-muted)]">
    Loading...
  </div>
);

export default ChartsPanel;
