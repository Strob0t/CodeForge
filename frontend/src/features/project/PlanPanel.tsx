import { createResource, createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type {
  Agent,
  CreatePlanRequest,
  CreateStepRequest,
  DecomposeRequest,
  ExecutionPlan,
  PlanProtocol,
  PlanStatus,
  PlanStepStatus,
  Task,
} from "~/api/types";
import { StepProgress } from "~/components/StepProgress";
import { useI18n } from "~/i18n";
import { Badge, Button, Card, Checkbox, FormField, Input, Select, Textarea } from "~/ui";

interface PlanPanelProps {
  projectId: string;
  tasks: Task[];
  agents: Agent[];
  onError: (msg: string) => void;
}

function planStatusVariant(
  status: PlanStatus,
): "default" | "info" | "success" | "danger" | "warning" {
  switch (status) {
    case "pending":
      return "default";
    case "running":
      return "info";
    case "completed":
      return "success";
    case "failed":
      return "danger";
    case "cancelled":
      return "warning";
  }
}

function stepStatusVariant(
  status: PlanStepStatus,
): "default" | "info" | "success" | "danger" | "warning" {
  switch (status) {
    case "pending":
      return "default";
    case "running":
      return "info";
    case "completed":
      return "success";
    case "failed":
      return "danger";
    case "skipped":
      return "default";
    case "cancelled":
      return "warning";
  }
}

export default function PlanPanel(props: PlanPanelProps) {
  const { t } = useI18n();

  const PROTOCOL_OPTIONS = (): { value: PlanProtocol; label: string; description: string }[] => [
    {
      value: "sequential",
      label: t("plan.protocol.sequential"),
      description: t("plan.protocol.sequentialDesc"),
    },
    {
      value: "parallel",
      label: t("plan.protocol.parallel"),
      description: t("plan.protocol.parallelDesc"),
    },
    {
      value: "ping_pong",
      label: t("plan.protocol.pingPong"),
      description: t("plan.protocol.pingPongDesc"),
    },
    {
      value: "consensus",
      label: t("plan.protocol.consensus"),
      description: t("plan.protocol.consensusDesc"),
    },
  ];

  const [plans, { refetch }] = createResource(
    () => props.projectId,
    (id) => api.plans.list(id),
  );

  const [showForm, setShowForm] = createSignal(false);
  const [selectedPlanId, setSelectedPlanId] = createSignal<string | null>(null);
  const [selectedPlan] = createResource(
    () => selectedPlanId(),
    (id) => api.plans.get(id),
  );

  // Decompose form state
  const [showDecompose, setShowDecompose] = createSignal(false);
  const [feature, setFeature] = createSignal("");
  const [decomposeContext, setDecomposeContext] = createSignal("");
  const [decomposeModel, setDecomposeModel] = createSignal("");
  const [autoStart, setAutoStart] = createSignal(false);
  const [decomposing, setDecomposing] = createSignal(false);
  const [decomposeResult, setDecomposeResult] = createSignal<ExecutionPlan | null>(null);

  const handleDecompose = async () => {
    if (!feature().trim()) {
      props.onError(t("plan.toast.featureRequired"));
      return;
    }
    setDecomposing(true);
    try {
      const req: DecomposeRequest = {
        feature: feature().trim(),
      };
      if (decomposeContext().trim()) req.context = decomposeContext().trim();
      if (decomposeModel().trim()) req.model = decomposeModel().trim();
      if (autoStart()) req.auto_start = true;

      const plan = await api.plans.decompose(props.projectId, req);
      setDecomposeResult(plan);
      refetch();
    } catch (e) {
      props.onError(e instanceof Error ? e.message : t("plan.toast.decomposeFailed"));
    } finally {
      setDecomposing(false);
    }
  };

  const acceptDecompose = () => {
    setDecomposeResult(null);
    setShowDecompose(false);
    setFeature("");
    setDecomposeContext("");
    setDecomposeModel("");
    setAutoStart(false);
  };

  const discardDecompose = () => {
    setDecomposeResult(null);
  };

  // Manual plan form state
  const [name, setName] = createSignal("");
  const [description, setDescription] = createSignal("");
  const [protocol, setProtocol] = createSignal<PlanProtocol>("sequential");
  const [maxParallel, setMaxParallel] = createSignal(4);
  const [steps, setSteps] = createSignal<CreateStepRequest[]>([
    { task_id: "", agent_id: "" },
    { task_id: "", agent_id: "" },
  ]);
  const [creating, setCreating] = createSignal(false);

  const addStep = () => {
    setSteps((prev) => [...prev, { task_id: "", agent_id: "" }]);
  };

  const removeStep = (index: number) => {
    setSteps((prev) => prev.filter((_, i) => i !== index));
  };

  const updateStep = (index: number, field: keyof CreateStepRequest, value: string) => {
    setSteps((prev) => prev.map((s, i) => (i === index ? { ...s, [field]: value } : s)));
  };

  const handleCreate = async () => {
    if (!name().trim()) {
      props.onError(t("plan.toast.nameRequired"));
      return;
    }
    if (steps().some((s) => !s.task_id || !s.agent_id)) {
      props.onError(t("plan.toast.stepsIncomplete"));
      return;
    }

    setCreating(true);
    try {
      const req: CreatePlanRequest = {
        name: name().trim(),
        description: description().trim(),
        protocol: protocol(),
        max_parallel: maxParallel(),
        steps: steps(),
      };
      await api.plans.create(props.projectId, req);
      refetch();
      resetForm();
    } catch (e) {
      props.onError(e instanceof Error ? e.message : t("plan.toast.createFailed"));
    } finally {
      setCreating(false);
    }
  };

  const resetForm = () => {
    setShowForm(false);
    setName("");
    setDescription("");
    setProtocol("sequential");
    setMaxParallel(4);
    setSteps([
      { task_id: "", agent_id: "" },
      { task_id: "", agent_id: "" },
    ]);
  };

  const handleStart = async (planId: string) => {
    try {
      await api.plans.start(planId);
      refetch();
      if (selectedPlanId() === planId) {
        setSelectedPlanId(null);
        setSelectedPlanId(planId);
      }
    } catch (e) {
      props.onError(e instanceof Error ? e.message : t("plan.toast.startFailed"));
    }
  };

  const handleCancel = async (planId: string) => {
    try {
      await api.plans.cancel(planId);
      refetch();
      if (selectedPlanId() === planId) {
        setSelectedPlanId(null);
        setSelectedPlanId(planId);
      }
    } catch (e) {
      props.onError(e instanceof Error ? e.message : t("plan.toast.cancelFailed"));
    }
  };

  const taskName = (id: string) =>
    props.tasks.find((task) => task.id === id)?.title ?? id.slice(0, 8);
  const agentName = (id: string) => props.agents.find((a) => a.id === id)?.name ?? id.slice(0, 8);

  return (
    <Card>
      <Card.Header>
        <div class="flex items-center justify-between">
          <h3 class="text-lg font-semibold">{t("plan.title")}</h3>
          <div class="flex gap-2">
            <Button
              variant={showDecompose() ? "secondary" : "primary"}
              size="sm"
              onClick={() => {
                setShowDecompose(!showDecompose());
                if (showDecompose()) setShowForm(false);
              }}
            >
              {showDecompose() ? t("common.cancel") : t("plan.decompose")}
            </Button>
            <Button
              variant={showForm() ? "secondary" : "primary"}
              size="sm"
              onClick={() => {
                setShowForm(!showForm());
                if (showForm()) setShowDecompose(false);
              }}
            >
              {showForm() ? t("common.cancel") : t("plan.newPlan")}
            </Button>
          </div>
        </div>
      </Card.Header>

      <Card.Body>
        {/* Decompose Feature -- Split-Screen */}
        <Show when={showDecompose()}>
          <div
            class={`mb-4 grid gap-4 ${decomposeResult() ? "grid-cols-1 lg:grid-cols-2" : "grid-cols-1"}`}
          >
            {/* Left: Prompt Form */}
            <div class="rounded-cf-sm border border-purple-200 bg-purple-50 p-4 dark:border-purple-700 dark:bg-purple-900/20">
              <p class="mb-3 text-xs text-cf-text-tertiary">{t("plan.form.featureHint")}</p>
              <FormField id="decompose-feature" label={t("plan.form.featureDesc")} required>
                <Textarea
                  id="decompose-feature"
                  rows={3}
                  value={feature()}
                  onInput={(e) => setFeature(e.currentTarget.value)}
                  placeholder="Describe the feature to implement..."
                  aria-required="true"
                />
              </FormField>
              <div class="mt-3">
                <FormField id="decompose-context" label={t("plan.form.context")}>
                  <Textarea
                    id="decompose-context"
                    rows={2}
                    value={decomposeContext()}
                    onInput={(e) => setDecomposeContext(e.currentTarget.value)}
                    placeholder="Repository structure, existing patterns, constraints..."
                  />
                </FormField>
              </div>
              <div class="mt-3 flex items-center gap-4">
                <div class="flex-1">
                  <FormField id="decompose-model" label={t("plan.form.modelOverride")}>
                    <Input
                      id="decompose-model"
                      type="text"
                      value={decomposeModel()}
                      onInput={(e) => setDecomposeModel(e.currentTarget.value)}
                      placeholder="e.g. openai/gpt-4o"
                    />
                  </FormField>
                </div>
                <label class="flex items-center gap-1.5 pt-4 text-sm text-cf-text-secondary">
                  <Checkbox checked={autoStart()} onChange={(checked) => setAutoStart(checked)} />
                  {t("plan.form.autoStart")}
                </label>
              </div>
              <div class="mt-3">
                <Button
                  variant="primary"
                  size="sm"
                  onClick={handleDecompose}
                  disabled={decomposing()}
                  loading={decomposing()}
                >
                  {decomposing() ? t("plan.form.decomposing") : t("plan.form.decomposeBtn")}
                </Button>
              </div>
            </div>

            {/* Right: Plan Preview */}
            <Show when={decomposeResult()}>
              {(result) => (
                <div class="rounded-cf-sm border border-cf-success-border bg-cf-success-bg p-4">
                  <div class="mb-3 flex items-center justify-between">
                    <h4 class="text-sm font-semibold text-cf-success-fg">
                      {t("plan.preview.title")}
                    </h4>
                    <div class="flex gap-2">
                      <Button variant="primary" size="sm" onClick={acceptDecompose}>
                        {t("plan.preview.accept")}
                      </Button>
                      <Button variant="secondary" size="sm" onClick={discardDecompose}>
                        {t("plan.preview.discard")}
                      </Button>
                    </div>
                  </div>

                  {/* Plan summary */}
                  <div class="mb-3 space-y-1 text-xs text-cf-text-secondary">
                    <p>
                      <span class="font-medium">{t("plan.preview.name")}:</span> {result().name}
                    </p>
                    <Show when={result().description}>
                      <p>
                        <span class="font-medium">{t("plan.preview.description")}:</span>{" "}
                        {result().description}
                      </p>
                    </Show>
                    <p>
                      <span class="font-medium">{t("plan.preview.protocol")}:</span>{" "}
                      <Badge variant="default">{result().protocol}</Badge>
                    </p>
                    <p>
                      <span class="font-medium">{t("plan.preview.steps")}:</span>{" "}
                      {result().steps.length}
                    </p>
                  </div>

                  {/* Step list */}
                  <div class="max-h-64 space-y-1.5 overflow-y-auto">
                    <For each={result().steps}>
                      {(step, idx) => (
                        <div class="flex items-start gap-2 rounded-cf-sm bg-cf-bg-surface p-2 text-xs">
                          <span class="mt-0.5 flex h-5 w-5 flex-shrink-0 items-center justify-center rounded-full bg-cf-success-bg text-cf-success-fg">
                            {idx() + 1}
                          </span>
                          <div class="min-w-0 flex-1">
                            <div class="flex items-center gap-2">
                              <span class="font-medium text-cf-text-primary">
                                {taskName(step.task_id)}
                              </span>
                              <span class="text-cf-text-muted">/</span>
                              <span class="text-cf-text-tertiary">{agentName(step.agent_id)}</span>
                            </div>
                            <Show when={step.depends_on && step.depends_on.length > 0}>
                              <p class="mt-0.5 text-cf-text-muted">
                                {t("plan.preview.dependsOn")}:{" "}
                                {step.depends_on
                                  .map((depId) => {
                                    const depIdx = result().steps.findIndex((s) => s.id === depId);
                                    return depIdx >= 0 ? `#${depIdx + 1}` : depId.slice(0, 8);
                                  })
                                  .join(", ")}
                              </p>
                            </Show>
                          </div>
                          <Badge variant={stepStatusVariant(step.status)}>{step.status}</Badge>
                        </div>
                      )}
                    </For>
                  </div>
                </div>
              )}
            </Show>
          </div>
        </Show>

        {/* Create Plan Form */}
        <Show when={showForm()}>
          <div class="mb-4 rounded-cf-sm border border-indigo-200 bg-indigo-50 p-4 dark:border-indigo-800 dark:bg-indigo-900/20">
            <div class="mb-3 grid grid-cols-2 gap-3">
              <FormField id="plan-name" label={t("plan.form.name")} required>
                <Input
                  id="plan-name"
                  type="text"
                  value={name()}
                  onInput={(e) => setName(e.currentTarget.value)}
                  placeholder="Plan name"
                  aria-required="true"
                />
              </FormField>
              <FormField id="plan-protocol" label={t("plan.form.protocol")}>
                <Select
                  id="plan-protocol"
                  value={protocol()}
                  onChange={(e) => setProtocol(e.currentTarget.value as PlanProtocol)}
                >
                  <For each={PROTOCOL_OPTIONS()}>
                    {(opt) => <option value={opt.value}>{opt.label}</option>}
                  </For>
                </Select>
              </FormField>
            </div>

            <FormField id="plan-description" label={t("plan.form.description")}>
              <Input
                id="plan-description"
                type="text"
                value={description()}
                onInput={(e) => setDescription(e.currentTarget.value)}
                placeholder="Optional description"
              />
            </FormField>

            <Show when={protocol() === "parallel"}>
              <div class="mt-3">
                <FormField id="plan-max-parallel" label={t("plan.form.maxParallel")}>
                  <Input
                    id="plan-max-parallel"
                    type="number"
                    min="1"
                    max="20"
                    class="w-24"
                    value={maxParallel()}
                    onInput={(e) => setMaxParallel(parseInt(e.currentTarget.value) || 4)}
                  />
                </FormField>
              </div>
            </Show>

            <p class="mt-2 mb-2 text-xs text-cf-text-tertiary">
              {PROTOCOL_OPTIONS().find((o) => o.value === protocol())?.description}
            </p>

            {/* Steps */}
            <div class="mb-3">
              <div class="mb-2 flex items-center justify-between">
                <label class="text-xs font-medium text-cf-text-tertiary">
                  {t("plan.form.steps")}
                </label>
                <Button variant="ghost" size="sm" onClick={addStep}>
                  {t("plan.form.addStep")}
                </Button>
              </div>
              <For each={steps()}>
                {(step, idx) => (
                  <div class="mb-2 flex items-center gap-2">
                    <span class="w-6 text-center text-xs text-cf-text-muted">{idx() + 1}</span>
                    <Select
                      class="flex-1"
                      value={step.task_id}
                      onChange={(e) => updateStep(idx(), "task_id", e.currentTarget.value)}
                      aria-label={`Step ${idx() + 1} task`}
                    >
                      <option value="">{t("plan.form.selectTask")}</option>
                      <For each={props.tasks}>
                        {(task) => <option value={task.id}>{task.title}</option>}
                      </For>
                    </Select>
                    <Select
                      class="flex-1"
                      value={step.agent_id}
                      onChange={(e) => updateStep(idx(), "agent_id", e.currentTarget.value)}
                      aria-label={`Step ${idx() + 1} agent`}
                    >
                      <option value="">{t("plan.form.selectAgent")}</option>
                      <For each={props.agents}>
                        {(a) => (
                          <option value={a.id}>
                            {a.name} ({a.backend})
                          </option>
                        )}
                      </For>
                    </Select>
                    <Show when={steps().length > 2}>
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => removeStep(idx())}
                        aria-label={`Remove step ${idx() + 1}`}
                      >
                        x
                      </Button>
                    </Show>
                  </div>
                )}
              </For>
            </div>

            <Button
              variant="primary"
              size="sm"
              onClick={handleCreate}
              disabled={creating()}
              loading={creating()}
            >
              {creating() ? t("plan.form.creating") : t("plan.form.createPlan")}
            </Button>
          </div>
        </Show>

        {/* Plan List */}
        <Show
          when={(plans() ?? []).length > 0}
          fallback={<p class="text-sm text-cf-text-muted">{t("plan.empty")}</p>}
        >
          <div class="space-y-2">
            <For each={plans()}>
              {(p) => (
                <div
                  class={`cursor-pointer rounded-cf-sm border p-3 transition-colors ${
                    selectedPlanId() === p.id
                      ? "border-cf-accent bg-cf-bg-surface-alt"
                      : "border-cf-border-subtle hover:bg-cf-bg-surface-alt"
                  }`}
                  role="button"
                  tabIndex={0}
                  aria-expanded={selectedPlanId() === p.id}
                  aria-label={`Plan: ${p.name}, status: ${p.status}`}
                  onClick={() => setSelectedPlanId(selectedPlanId() === p.id ? null : p.id)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" || e.key === " ") {
                      e.preventDefault();
                      setSelectedPlanId(selectedPlanId() === p.id ? null : p.id);
                    }
                  }}
                >
                  <div class="flex items-center justify-between">
                    <div class="flex items-center gap-2">
                      <span class="font-medium text-sm">{p.name}</span>
                      <Badge variant={planStatusVariant(p.status)} pill>
                        {p.status}
                      </Badge>
                      <Badge variant="default">{p.protocol}</Badge>
                    </div>
                    <div class="flex gap-1">
                      <Show when={p.status === "pending"}>
                        <Button
                          variant="primary"
                          size="sm"
                          onClick={(e: MouseEvent) => {
                            e.stopPropagation();
                            handleStart(p.id);
                          }}
                          aria-label={`Start plan ${p.name}`}
                        >
                          {t("plan.start")}
                        </Button>
                      </Show>
                      <Show when={p.status === "running"}>
                        <Button
                          variant="danger"
                          size="sm"
                          onClick={(e: MouseEvent) => {
                            e.stopPropagation();
                            handleCancel(p.id);
                          }}
                          aria-label={`Cancel plan ${p.name}`}
                        >
                          {t("common.cancel")}
                        </Button>
                      </Show>
                    </div>
                  </div>
                  <Show when={p.status === "running" && p.steps.length > 0}>
                    <div class="mt-1">
                      <StepProgress
                        current={p.steps.filter((s) => s.status === "completed").length}
                        max={p.steps.length}
                        label={t("progress.planSteps")}
                      />
                    </div>
                  </Show>
                  <Show when={p.description}>
                    <p class="mt-1 text-xs text-cf-text-tertiary">{p.description}</p>
                  </Show>
                </div>
              )}
            </For>
          </div>
        </Show>

        {/* Selected Plan Detail */}
        <Show when={selectedPlan()}>
          {(detail) => (
            <div class="mt-4 rounded-cf-sm border border-indigo-200 bg-indigo-50 p-4 dark:border-indigo-800 dark:bg-indigo-900/20">
              <h4 class="mb-2 text-sm font-semibold">{detail().name} - Steps</h4>
              <Show when={detail().steps.length > 0}>
                <div class="mb-3">
                  <StepProgress
                    current={detail().steps.filter((s) => s.status === "completed").length}
                    max={detail().steps.length}
                    label={t("progress.planSteps")}
                  />
                </div>
              </Show>
              <div class="space-y-2">
                <For each={detail().steps}>
                  {(step, idx) => (
                    <div class="flex items-center gap-3 rounded-cf-sm bg-cf-bg-surface p-2 text-sm">
                      <span class="w-6 text-center text-xs text-cf-text-muted">{idx() + 1}</span>
                      <Badge variant={stepStatusVariant(step.status)}>{step.status}</Badge>
                      <span class="text-cf-text-secondary">
                        {taskName(step.task_id)} / {agentName(step.agent_id)}
                      </span>
                      <Show when={step.run_id}>
                        <span class="font-mono text-xs text-cf-text-muted">
                          run: {step.run_id.slice(0, 8)}
                        </span>
                      </Show>
                      <Show when={step.round > 0}>
                        <span class="text-xs text-cf-text-muted">round {step.round}</span>
                      </Show>
                      <Show when={step.error}>
                        <span class="text-xs text-cf-danger-fg">{step.error}</span>
                      </Show>
                    </div>
                  )}
                </For>
              </div>
            </div>
          )}
        </Show>
      </Card.Body>
    </Card>
  );
}
