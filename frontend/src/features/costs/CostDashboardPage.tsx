import { A } from "@solidjs/router";
import { createResource, For, Show } from "solid-js";

import { api } from "~/api/client";
import type {
  DailyCost,
  ModelCostSummary,
  ProjectCostSummary,
  Run,
  ToolCostSummary,
} from "~/api/types";
import { useI18n } from "~/i18n";
import { Card, LoadingState, PageLayout, Table } from "~/ui";
import type { TableColumn } from "~/ui/composites/Table";

export default function CostDashboardPage() {
  const { t, fmt } = useI18n();
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

  const projectColumns: TableColumn<ProjectCostSummary>[] = [
    {
      key: "project_name",
      header: t("costs.table.project"),
      render: (p) => (
        <A href={`/projects/${p.project_id}`} class="text-cf-accent hover:underline">
          {p.project_name || p.project_id}
        </A>
      ),
    },
    {
      key: "total_cost_usd",
      header: t("costs.table.cost"),
      class: "text-right",
      render: (p) => <span class="font-mono">{fmt.currency(p.total_cost_usd)}</span>,
    },
    {
      key: "total_tokens_in",
      header: t("costs.table.tokensIn"),
      class: "text-right",
      render: (p) => <span class="font-mono">{fmt.compact(p.total_tokens_in)}</span>,
    },
    {
      key: "total_tokens_out",
      header: t("costs.table.tokensOut"),
      class: "text-right",
      render: (p) => <span class="font-mono">{fmt.compact(p.total_tokens_out)}</span>,
    },
    {
      key: "run_count",
      header: t("costs.table.runs"),
      class: "text-right",
      render: (p) => <span>{p.run_count}</span>,
    },
  ];

  return (
    <PageLayout title={t("costs.title")}>
      {/* Global Totals */}
      <div class="mb-6 grid grid-cols-4 gap-4">
        <Card>
          <Card.Body>
            <p class="text-sm text-cf-text-muted">{t("costs.totalCost")}</p>
            <p class="mt-1 text-2xl font-bold text-cf-text-primary">
              {fmt.currency(totals().cost)}
            </p>
          </Card.Body>
        </Card>
        <Card>
          <Card.Body>
            <p class="text-sm text-cf-text-muted">{t("costs.tokensIn")}</p>
            <p class="mt-1 text-2xl font-bold text-cf-text-primary">
              {fmt.compact(totals().tokensIn)}
            </p>
          </Card.Body>
        </Card>
        <Card>
          <Card.Body>
            <p class="text-sm text-cf-text-muted">{t("costs.tokensOut")}</p>
            <p class="mt-1 text-2xl font-bold text-cf-text-primary">
              {fmt.compact(totals().tokensOut)}
            </p>
          </Card.Body>
        </Card>
        <Card>
          <Card.Body>
            <p class="text-sm text-cf-text-muted">{t("costs.totalRuns")}</p>
            <p class="mt-1 text-2xl font-bold text-cf-text-primary">{totals().runs}</p>
          </Card.Body>
        </Card>
      </div>

      {/* Project Breakdown */}
      <Card class="mb-6">
        <Card.Header>
          <h3 class="text-lg font-semibold text-cf-text-primary">{t("costs.byProject")}</h3>
        </Card.Header>
        <Card.Body class="p-0">
          <Show when={!globalCosts.loading} fallback={<LoadingState />}>
            <Table<ProjectCostSummary>
              columns={projectColumns}
              data={globalCosts() ?? []}
              rowKey={(p) => p.project_id}
              emptyMessage={t("costs.empty")}
            />
          </Show>
        </Card.Body>
      </Card>
    </PageLayout>
  );
}

/** Reusable cost section for a specific project */
export function ProjectCostSection(props: { projectId: string }) {
  const { t, tp, fmt } = useI18n();
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
  const [byTool] = createResource(
    () => props.projectId,
    (id) => api.costs.byTool(id),
  );

  const maxDailyCost = () => {
    const items = daily() ?? [];
    return Math.max(...items.map((d) => d.cost_usd), 0.001);
  };

  return (
    <Card>
      <Card.Header>
        <h3 class="text-lg font-semibold text-cf-text-primary">{t("costs.overview")}</h3>
      </Card.Header>
      <Card.Body>
        {/* Summary Cards */}
        <Show when={summary()}>
          {(s) => (
            <div class="mb-4 grid grid-cols-4 gap-3">
              <div class="rounded-cf-md bg-cf-bg-surface-alt p-3">
                <p class="text-xs text-cf-text-muted">{t("costs.totalCost")}</p>
                <p class="text-lg font-bold text-cf-text-primary">
                  {fmt.currency(s().total_cost_usd)}
                </p>
              </div>
              <div class="rounded-cf-md bg-cf-bg-surface-alt p-3">
                <p class="text-xs text-cf-text-muted">{t("costs.tokensIn")}</p>
                <p class="text-lg font-bold text-cf-text-primary">
                  {fmt.compact(s().total_tokens_in)}
                </p>
              </div>
              <div class="rounded-cf-md bg-cf-bg-surface-alt p-3">
                <p class="text-xs text-cf-text-muted">{t("costs.tokensOut")}</p>
                <p class="text-lg font-bold text-cf-text-primary">
                  {fmt.compact(s().total_tokens_out)}
                </p>
              </div>
              <div class="rounded-cf-md bg-cf-bg-surface-alt p-3">
                <p class="text-xs text-cf-text-muted">{t("costs.table.runs")}</p>
                <p class="text-lg font-bold text-cf-text-primary">{s().run_count}</p>
              </div>
            </div>
          )}
        </Show>

        {/* Model Breakdown */}
        <Show when={(byModel() ?? []).length > 0}>
          <div class="mb-4">
            <h4 class="mb-2 text-sm font-medium text-cf-text-muted">{t("costs.byModel")}</h4>
            <div class="space-y-1">
              <For each={byModel() ?? []}>
                {(m: ModelCostSummary) => (
                  <div class="flex items-center justify-between rounded-cf-md bg-cf-bg-surface-alt px-3 py-2 text-sm">
                    <span class="font-mono text-xs">{m.model || t("costs.unknown")}</span>
                    <div class="flex gap-4 text-xs text-cf-text-muted">
                      <span>{fmt.currency(m.total_cost_usd)}</span>
                      <span>
                        {fmt.compact(m.total_tokens_in)} {t("costs.in")}
                      </span>
                      <span>
                        {fmt.compact(m.total_tokens_out)} {t("costs.out")}
                      </span>
                      <span>{tp("costs.runs", m.run_count)}</span>
                    </div>
                  </div>
                )}
              </For>
            </div>
          </div>
        </Show>

        {/* Tool Breakdown */}
        <Show when={(byTool() ?? []).length > 0}>
          <div class="mb-4">
            <h4 class="mb-2 text-sm font-medium text-cf-text-muted">{t("costs.byTool")}</h4>
            <div class="space-y-1">
              <For each={byTool() ?? []}>
                {(item: ToolCostSummary) => (
                  <div class="flex items-center justify-between rounded-cf-md bg-cf-bg-surface-alt px-3 py-2 text-sm">
                    <div class="flex items-center gap-2">
                      <span class="font-mono text-xs">{item.tool || t("costs.unknown")}</span>
                      <span class="text-xs text-cf-text-tertiary">{item.model || ""}</span>
                    </div>
                    <div class="flex gap-4 text-xs text-cf-text-muted">
                      <span>{fmt.currency(item.cost_usd)}</span>
                      <span>
                        {fmt.compact(item.tokens_in)} {t("costs.in")}
                      </span>
                      <span>
                        {fmt.compact(item.tokens_out)} {t("costs.out")}
                      </span>
                      <span>{tp("costs.calls", item.call_count)}</span>
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
            <h4 class="mb-2 text-sm font-medium text-cf-text-muted">{t("costs.dailyChart")}</h4>
            <div
              class="flex items-end gap-0.5"
              style={{ height: "80px" }}
              role="img"
              aria-label={t("costs.dailyChartAria")}
            >
              <For each={daily() ?? []}>
                {(d: DailyCost) => {
                  const pct = () => Math.max((d.cost_usd / maxDailyCost()) * 100, 2);
                  return (
                    <div
                      class="flex-1 rounded-t bg-cf-accent hover:opacity-80"
                      style={{ height: `${pct()}%` }}
                      title={`${d.date}: ${fmt.currency(d.cost_usd)} (${d.run_count} runs)`}
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
            <h4 class="mb-2 text-sm font-medium text-cf-text-muted">{t("costs.recentRuns")}</h4>
            <div class="space-y-1">
              <For each={recentRuns() ?? []}>
                {(r: Run) => (
                  <div class="flex items-center justify-between rounded-cf-md bg-cf-bg-surface-alt px-3 py-2 text-sm">
                    <div class="flex items-center gap-2">
                      <span class="font-mono text-xs text-cf-text-tertiary">
                        {r.id.slice(0, 8)}
                      </span>
                      <span class="text-xs text-cf-text-muted">{r.model || "-"}</span>
                    </div>
                    <div class="flex gap-3 text-xs text-cf-text-muted">
                      <span>{fmt.currency(r.cost_usd)}</span>
                      <span>
                        {fmt.compact(r.tokens_in)} {t("costs.in")}
                      </span>
                      <span>
                        {fmt.compact(r.tokens_out)} {t("costs.out")}
                      </span>
                      <span>{tp("costs.steps", r.step_count)}</span>
                    </div>
                  </div>
                )}
              </For>
            </div>
          </div>
        </Show>
      </Card.Body>
    </Card>
  );
}
