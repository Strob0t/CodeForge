import { createResource, createSignal, For, onMount, Show } from "solid-js";

import { api } from "~/api/client";
import type { ModelPerformanceStats, RoutingOutcome } from "~/api/types";
import { useToast } from "~/components/Toast";
import { useAsyncAction } from "~/hooks";
import { useFormState } from "~/hooks/useFormState";
import { useI18n } from "~/i18n";
import {
  Badge,
  Button,
  Card,
  Checkbox,
  EmptyState,
  ErrorBanner,
  FormField,
  Input,
  LoadingState,
  PageLayout,
  Select,
  Table,
} from "~/ui";
import type { TableColumn } from "~/ui/composites/Table";

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const TASK_TYPES = ["code", "review", "plan", "qa", "chat", "debug", "refactor"] as const;
const TIERS = ["simple", "medium", "complex", "reasoning"] as const;

// ---------------------------------------------------------------------------
// Routing Stats Page
// ---------------------------------------------------------------------------

export default function RoutingStatsPage() {
  onMount(() => {
    document.title = "Routing - CodeForge";
  });
  const { t } = useI18n();
  const { show: toast } = useToast();

  // ---- Stats section ----
  const [taskType, setTaskType] = createSignal("");
  const [tier, setTier] = createSignal("");
  const [stats, { refetch: refetchStats }] = createResource(
    () => ({ taskType: taskType(), tier: tier() }),
    (opts) => api.routing.stats(opts.taskType || undefined, opts.tier || undefined),
  );

  // ---- Outcomes section ----
  const [outcomes, { refetch: refetchOutcomes }] = createResource(() => api.routing.outcomes(50));

  // ---- Actions ----
  const {
    run: handleRefresh,
    loading: refreshing,
    error: refreshError,
    clearError: clearRefreshError,
  } = useAsyncAction(
    async () => {
      await api.routing.refreshStats();
      toast("success", t("routing.stats.refreshed"));
      refetchStats();
    },
    { onError: () => toast("error", t("routing.error.refreshFailed")) },
  );

  const {
    run: handleSeed,
    loading: seeding,
    error: seedError,
    clearError: clearSeedError,
  } = useAsyncAction(
    async () => {
      const result = await api.routing.seedFromBenchmarks();
      toast("success", t("routing.stats.seeded", { count: String(result.outcomes_created) }));
      refetchStats();
      refetchOutcomes();
    },
    { onError: () => toast("error", t("routing.error.seedFailed")) },
  );

  // ---- Record outcome form ----
  const [showRecordForm, setShowRecordForm] = createSignal(false);

  // ---- Stats table columns ----
  const statsColumns: TableColumn<ModelPerformanceStats>[] = [
    {
      key: "model_name",
      header: t("routing.field.modelName"),
      render: (row) => <span class="font-mono text-sm">{row.model_name}</span>,
    },
    { key: "task_type", header: t("routing.field.taskType") },
    {
      key: "complexity_tier",
      header: t("routing.field.tier"),
      render: (row) => <Badge>{row.complexity_tier}</Badge>,
    },
    {
      key: "trial_count",
      header: t("routing.field.trials"),
      render: (row) => <span class="font-mono">{row.trial_count}</span>,
    },
    {
      key: "avg_reward",
      header: t("routing.field.avgReward"),
      render: (row) => <span class="font-mono">{row.avg_reward.toFixed(3)}</span>,
    },
    {
      key: "avg_quality",
      header: t("routing.field.qualityScore"),
      render: (row) => <span class="font-mono">{row.avg_quality.toFixed(3)}</span>,
    },
    {
      key: "avg_cost_usd",
      header: t("routing.field.costUsd"),
      render: (row) => <span class="font-mono">${row.avg_cost_usd.toFixed(4)}</span>,
    },
    {
      key: "avg_latency_ms",
      header: t("routing.field.latencyMs"),
      render: (row) => <span class="font-mono">{row.avg_latency_ms.toFixed(0)}ms</span>,
    },
    {
      key: "supports_tools",
      header: "Tools",
      render: (row) => (
        <Badge variant={row.supports_tools ? "success" : "default"}>
          {row.supports_tools ? "Yes" : "No"}
        </Badge>
      ),
    },
    {
      key: "supports_vision",
      header: "Vision",
      render: (row) => (
        <Badge variant={row.supports_vision ? "success" : "default"}>
          {row.supports_vision ? "Yes" : "No"}
        </Badge>
      ),
    },
    {
      key: "max_context",
      header: "Max Context",
      render: (row) => (
        <span class="font-mono">
          {row.max_context > 0 ? `${(row.max_context / 1000).toFixed(0)}K` : "-"}
        </span>
      ),
    },
  ];

  // ---- Outcomes table columns ----
  const outcomeColumns: TableColumn<RoutingOutcome>[] = [
    {
      key: "model_name",
      header: t("routing.field.modelName"),
      render: (row) => <span class="font-mono text-sm">{row.model_name}</span>,
    },
    { key: "task_type", header: t("routing.field.taskType") },
    {
      key: "complexity_tier",
      header: t("routing.field.tier"),
      render: (row) => <Badge>{row.complexity_tier}</Badge>,
    },
    {
      key: "success",
      header: t("routing.field.success"),
      render: (row) => (
        <Badge variant={row.success ? "success" : "danger"}>{row.success ? "Pass" : "Fail"}</Badge>
      ),
    },
    {
      key: "quality_score",
      header: t("routing.field.qualityScore"),
      render: (row) => <span class="font-mono">{row.quality_score.toFixed(3)}</span>,
    },
    {
      key: "cost_usd",
      header: t("routing.field.costUsd"),
      render: (row) => <span class="font-mono">${row.cost_usd.toFixed(4)}</span>,
    },
    {
      key: "latency_ms",
      header: t("routing.field.latencyMs"),
      render: (row) => <span class="font-mono">{row.latency_ms.toFixed(0)}ms</span>,
    },
    {
      key: "routing_layer",
      header: t("routing.field.routingLayer"),
      render: (row) => <Badge variant="info">{row.routing_layer}</Badge>,
    },
    {
      key: "created_at",
      header: "Time",
      render: (row) => (
        <span class="text-xs text-cf-text-muted">{new Date(row.created_at).toLocaleString()}</span>
      ),
    },
  ];

  return (
    <PageLayout
      title={t("routing.title")}
      description={t("routing.subtitle")}
      action={
        <div class="flex gap-2">
          <Button
            variant="secondary"
            size="sm"
            onClick={() => void handleRefresh()}
            disabled={refreshing()}
            loading={refreshing()}
          >
            {t("routing.stats.refresh")}
          </Button>
          <Button
            variant="secondary"
            size="sm"
            onClick={() => void handleSeed()}
            disabled={seeding()}
            loading={seeding()}
          >
            {t("routing.stats.seed")}
          </Button>
        </div>
      }
    >
      <ErrorBanner error={refreshError} onDismiss={clearRefreshError} />
      <ErrorBanner error={seedError} onDismiss={clearSeedError} />

      {/* ---- Section 1: Stats Table ---- */}
      <section class="mb-8">
        <h2 class="mb-3 text-lg font-semibold text-cf-text-primary">{t("routing.stats")}</h2>

        {/* Filters */}
        <div class="mb-4 flex flex-wrap gap-3">
          <FormField label={t("routing.filter.taskType")} id="filter-task-type">
            <Select
              id="filter-task-type"
              value={taskType()}
              onChange={(e) => setTaskType(e.currentTarget.value)}
            >
              <option value="">{t("routing.filter.all")}</option>
              <For each={[...TASK_TYPES]}>{(tt) => <option value={tt}>{tt}</option>}</For>
            </Select>
          </FormField>
          <FormField label={t("routing.filter.tier")} id="filter-tier">
            <Select
              id="filter-tier"
              value={tier()}
              onChange={(e) => setTier(e.currentTarget.value)}
            >
              <option value="">{t("routing.filter.all")}</option>
              <For each={[...TIERS]}>{(t) => <option value={t}>{t}</option>}</For>
            </Select>
          </FormField>
        </div>

        {/* Stats data */}
        <Show when={stats.loading}>
          <LoadingState message={t("common.loading")} />
        </Show>
        <Show when={stats.error}>
          <ErrorBanner error={stats.error} />
        </Show>
        <Show when={!stats.loading && !stats.error}>
          <Show
            when={(stats() ?? []).length > 0}
            fallback={
              <EmptyState
                title={t("routing.stats.empty")}
                description={t("routing.stats.emptyDescription")}
              />
            }
          >
            <div class="overflow-x-auto">
              <Table<ModelPerformanceStats>
                columns={statsColumns}
                data={stats() ?? []}
                rowKey={(s) => s.id}
              />
            </div>
          </Show>
        </Show>
      </section>

      {/* ---- Section 2: Outcomes Table ---- */}
      <section class="mb-8">
        <h2 class="mb-3 text-lg font-semibold text-cf-text-primary">{t("routing.outcomes")}</h2>

        <Show when={outcomes.loading}>
          <LoadingState message={t("common.loading")} />
        </Show>
        <Show when={outcomes.error}>
          <ErrorBanner error={outcomes.error} />
        </Show>
        <Show when={!outcomes.loading && !outcomes.error}>
          <Show
            when={(outcomes() ?? []).length > 0}
            fallback={<EmptyState title={t("routing.outcomes.empty")} />}
          >
            <div class="overflow-x-auto">
              <Table<RoutingOutcome>
                columns={outcomeColumns}
                data={outcomes() ?? []}
                rowKey={(o) => o.id}
              />
            </div>
          </Show>
        </Show>
      </section>

      {/* ---- Section 3: Record Outcome Form ---- */}
      <section>
        <Button
          variant={showRecordForm() ? "secondary" : "primary"}
          size="sm"
          onClick={() => setShowRecordForm(!showRecordForm())}
        >
          {showRecordForm() ? t("common.cancel") : t("routing.outcomes.record")}
        </Button>

        <Show when={showRecordForm()}>
          <RecordOutcomeForm
            onSuccess={() => {
              setShowRecordForm(false);
              refetchOutcomes();
              refetchStats();
            }}
          />
        </Show>
      </section>
    </PageLayout>
  );
}

// ---------------------------------------------------------------------------
// Record Outcome Form
// ---------------------------------------------------------------------------

interface RecordOutcomeFormDefaults {
  model_name: string;
  task_type: string;
  complexity_tier: string;
  success: boolean;
  quality_score: string;
  cost_usd: string;
  latency_ms: string;
  tokens_in: string;
  tokens_out: string;
  routing_layer: string;
}

const FORM_DEFAULTS: RecordOutcomeFormDefaults = {
  model_name: "",
  task_type: "code",
  complexity_tier: "simple",
  success: true,
  quality_score: "0",
  cost_usd: "0",
  latency_ms: "0",
  tokens_in: "0",
  tokens_out: "0",
  routing_layer: "manual",
};

function RecordOutcomeForm(props: { onSuccess: () => void }) {
  const { t } = useI18n();
  const { show: toast } = useToast();
  const form = useFormState(FORM_DEFAULTS);

  const {
    run: handleSubmit,
    loading: submitting,
    error,
    clearError,
  } = useAsyncAction(
    async () => {
      if (!form.state.model_name.trim()) {
        toast("error", t("routing.error.recordFailed"));
        return;
      }
      await api.routing.recordOutcome({
        model_name: form.state.model_name.trim(),
        task_type: form.state.task_type,
        complexity_tier: form.state.complexity_tier,
        success: form.state.success,
        quality_score: parseFloat(form.state.quality_score) || 0,
        cost_usd: parseFloat(form.state.cost_usd) || 0,
        latency_ms: parseFloat(form.state.latency_ms) || 0,
        tokens_in: parseInt(form.state.tokens_in, 10) || 0,
        tokens_out: parseInt(form.state.tokens_out, 10) || 0,
        reward: 0,
        routing_layer: form.state.routing_layer.trim() || "manual",
      });
      toast("success", t("routing.recorded"));
      form.reset();
      props.onSuccess();
    },
    { onError: () => toast("error", t("routing.error.recordFailed")) },
  );

  return (
    <Card class="mt-4">
      <Card.Body>
        <ErrorBanner error={error} onDismiss={clearError} />
        <form
          onSubmit={(e) => {
            e.preventDefault();
            void handleSubmit();
          }}
          aria-label={t("routing.outcomes.record")}
        >
          <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
            <FormField label={t("routing.field.modelName")} id="outcome-model" required>
              <Input
                id="outcome-model"
                type="text"
                value={form.state.model_name}
                onInput={(e) => form.setState("model_name", e.currentTarget.value)}
                placeholder="openai/gpt-4o"
                mono
                aria-required="true"
              />
            </FormField>

            <FormField label={t("routing.field.taskType")} id="outcome-task-type">
              <Select
                id="outcome-task-type"
                value={form.state.task_type}
                onChange={(e) => form.setState("task_type", e.currentTarget.value)}
              >
                <For each={[...TASK_TYPES]}>{(tt) => <option value={tt}>{tt}</option>}</For>
              </Select>
            </FormField>

            <FormField label={t("routing.field.tier")} id="outcome-tier">
              <Select
                id="outcome-tier"
                value={form.state.complexity_tier}
                onChange={(e) => form.setState("complexity_tier", e.currentTarget.value)}
              >
                <For each={[...TIERS]}>{(t) => <option value={t}>{t}</option>}</For>
              </Select>
            </FormField>

            <FormField label={t("routing.field.qualityScore")} id="outcome-quality">
              <Input
                id="outcome-quality"
                type="number"
                step="0.01"
                value={form.state.quality_score}
                onInput={(e) => form.setState("quality_score", e.currentTarget.value)}
              />
            </FormField>

            <FormField label={t("routing.field.costUsd")} id="outcome-cost">
              <Input
                id="outcome-cost"
                type="number"
                step="0.0001"
                value={form.state.cost_usd}
                onInput={(e) => form.setState("cost_usd", e.currentTarget.value)}
              />
            </FormField>

            <FormField label={t("routing.field.latencyMs")} id="outcome-latency">
              <Input
                id="outcome-latency"
                type="number"
                value={form.state.latency_ms}
                onInput={(e) => form.setState("latency_ms", e.currentTarget.value)}
              />
            </FormField>

            <FormField label={t("routing.field.tokensIn")} id="outcome-tokens-in">
              <Input
                id="outcome-tokens-in"
                type="number"
                value={form.state.tokens_in}
                onInput={(e) => form.setState("tokens_in", e.currentTarget.value)}
              />
            </FormField>

            <FormField label={t("routing.field.tokensOut")} id="outcome-tokens-out">
              <Input
                id="outcome-tokens-out"
                type="number"
                value={form.state.tokens_out}
                onInput={(e) => form.setState("tokens_out", e.currentTarget.value)}
              />
            </FormField>

            <FormField label={t("routing.field.routingLayer")} id="outcome-layer">
              <Input
                id="outcome-layer"
                type="text"
                value={form.state.routing_layer}
                onInput={(e) => form.setState("routing_layer", e.currentTarget.value)}
                placeholder="manual"
                mono
              />
            </FormField>

            <div class="flex items-center gap-3 sm:col-span-2 lg:col-span-3">
              <Checkbox
                id="outcome-success"
                checked={form.state.success}
                onChange={(checked) => form.setState("success", checked)}
              />
              <label for="outcome-success" class="text-sm font-medium text-cf-text-secondary">
                {t("routing.field.success")}
              </label>
            </div>
          </div>

          <div class="mt-4 flex justify-end gap-2">
            <Button type="submit" disabled={submitting()} loading={submitting()}>
              {t("routing.outcomes.record")}
            </Button>
          </div>
        </form>
      </Card.Body>
    </Card>
  );
}
