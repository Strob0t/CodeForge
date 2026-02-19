import { createResource, createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type {
  Agent,
  CreatePlanRequest,
  CreateStepRequest,
  DecomposeRequest,
  PlanProtocol,
  PlanStatus,
  PlanStepStatus,
  Task,
} from "~/api/types";

interface PlanPanelProps {
  projectId: string;
  tasks: Task[];
  agents: Agent[];
  onError: (msg: string) => void;
}

const PROTOCOL_OPTIONS: { value: PlanProtocol; label: string; description: string }[] = [
  { value: "sequential", label: "Sequential", description: "Steps run one at a time in order" },
  { value: "parallel", label: "Parallel", description: "All ready steps run concurrently" },
  {
    value: "ping_pong",
    label: "Ping-Pong",
    description: "Two agents alternate on each other's output",
  },
  {
    value: "consensus",
    label: "Consensus",
    description: "Same task to multiple agents, majority vote",
  },
];

const PLAN_STATUS_COLORS: Record<PlanStatus, string> = {
  pending: "bg-gray-100 text-gray-700 dark:bg-gray-700 dark:text-gray-300",
  running: "bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400",
  completed: "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400",
  failed: "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400",
  cancelled: "bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400",
};

const STEP_STATUS_COLORS: Record<PlanStepStatus, string> = {
  pending: "bg-gray-100 text-gray-700 dark:bg-gray-700 dark:text-gray-300",
  running: "bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400",
  completed: "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400",
  failed: "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400",
  skipped: "bg-gray-100 text-gray-500 dark:bg-gray-700 dark:text-gray-400",
  cancelled: "bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400",
};

export default function PlanPanel(props: PlanPanelProps) {
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

  const handleDecompose = async () => {
    if (!feature().trim()) {
      props.onError("Feature description is required");
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

      await api.plans.decompose(props.projectId, req);
      refetch();
      setShowDecompose(false);
      setFeature("");
      setDecomposeContext("");
      setDecomposeModel("");
      setAutoStart(false);
    } catch (e) {
      props.onError(e instanceof Error ? e.message : "Decomposition failed");
    } finally {
      setDecomposing(false);
    }
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
      props.onError("Plan name is required");
      return;
    }
    if (steps().some((s) => !s.task_id || !s.agent_id)) {
      props.onError("All steps must have a task and agent selected");
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
      props.onError(e instanceof Error ? e.message : "Failed to create plan");
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
      props.onError(e instanceof Error ? e.message : "Failed to start plan");
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
      props.onError(e instanceof Error ? e.message : "Failed to cancel plan");
    }
  };

  const taskName = (id: string) => props.tasks.find((t) => t.id === id)?.title ?? id.slice(0, 8);
  const agentName = (id: string) => props.agents.find((a) => a.id === id)?.name ?? id.slice(0, 8);

  return (
    <div class="rounded-lg border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-gray-800">
      <div class="mb-3 flex items-center justify-between">
        <h3 class="text-lg font-semibold">Execution Plans</h3>
        <div class="flex gap-2">
          <button
            class="rounded bg-purple-600 px-3 py-1.5 text-sm text-white hover:bg-purple-700"
            onClick={() => {
              setShowDecompose(!showDecompose());
              if (showDecompose()) setShowForm(false);
            }}
          >
            {showDecompose() ? "Cancel" : "Decompose Feature"}
          </button>
          <button
            class="rounded bg-indigo-600 px-3 py-1.5 text-sm text-white hover:bg-indigo-700"
            onClick={() => {
              setShowForm(!showForm());
              if (showForm()) setShowDecompose(false);
            }}
          >
            {showForm() ? "Cancel" : "New Plan"}
          </button>
        </div>
      </div>

      {/* Decompose Feature Form */}
      <Show when={showDecompose()}>
        <div class="mb-4 rounded border border-purple-200 bg-purple-50 p-4 dark:border-purple-700 dark:bg-purple-900/20">
          <p class="mb-3 text-xs text-gray-600 dark:text-gray-400">
            Describe a feature and let the meta-agent decompose it into subtasks with an execution
            plan.
          </p>
          <div class="mb-3">
            <label
              for="decompose-feature"
              class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400"
            >
              Feature Description <span aria-hidden="true">*</span>
              <span class="sr-only">(required)</span>
            </label>
            <textarea
              id="decompose-feature"
              class="w-full rounded border border-gray-300 px-2 py-1.5 text-sm dark:border-gray-600 dark:bg-gray-700"
              rows={3}
              value={feature()}
              onInput={(e) => setFeature(e.currentTarget.value)}
              placeholder="Describe the feature to implement..."
              aria-required="true"
            />
          </div>
          <div class="mb-3">
            <label for="decompose-context" class="mb-1 block text-xs font-medium text-gray-600">
              Additional Context (optional)
            </label>
            <textarea
              id="decompose-context"
              class="w-full rounded border border-gray-300 px-2 py-1.5 text-sm"
              rows={2}
              value={decomposeContext()}
              onInput={(e) => setDecomposeContext(e.currentTarget.value)}
              placeholder="Repository structure, existing patterns, constraints..."
            />
          </div>
          <div class="mb-3 flex items-center gap-4">
            <div class="flex-1">
              <label for="decompose-model" class="mb-1 block text-xs font-medium text-gray-600">
                Model Override (optional)
              </label>
              <input
                id="decompose-model"
                type="text"
                class="w-full rounded border border-gray-300 px-2 py-1.5 text-sm"
                value={decomposeModel()}
                onInput={(e) => setDecomposeModel(e.currentTarget.value)}
                placeholder="e.g. openai/gpt-4o"
              />
            </div>
            <label class="flex items-center gap-1.5 pt-4 text-sm">
              <input
                type="checkbox"
                checked={autoStart()}
                onChange={(e) => setAutoStart(e.currentTarget.checked)}
              />
              Auto-start
            </label>
          </div>
          <button
            class="rounded bg-purple-600 px-4 py-1.5 text-sm text-white hover:bg-purple-700 disabled:opacity-50"
            onClick={handleDecompose}
            disabled={decomposing()}
          >
            {decomposing() ? "Decomposing..." : "Decompose"}
          </button>
        </div>
      </Show>

      {/* Create Plan Form */}
      <Show when={showForm()}>
        <div class="mb-4 rounded border border-indigo-200 bg-indigo-50 p-4">
          <div class="mb-3 grid grid-cols-2 gap-3">
            <div>
              <label for="plan-name" class="mb-1 block text-xs font-medium text-gray-600">
                Name <span aria-hidden="true">*</span>
                <span class="sr-only">(required)</span>
              </label>
              <input
                id="plan-name"
                type="text"
                class="w-full rounded border border-gray-300 px-2 py-1.5 text-sm"
                value={name()}
                onInput={(e) => setName(e.currentTarget.value)}
                placeholder="Plan name"
                aria-required="true"
              />
            </div>
            <div>
              <label for="plan-protocol" class="mb-1 block text-xs font-medium text-gray-600">
                Protocol
              </label>
              <select
                id="plan-protocol"
                class="w-full rounded border border-gray-300 px-2 py-1.5 text-sm"
                value={protocol()}
                onChange={(e) => setProtocol(e.currentTarget.value as PlanProtocol)}
              >
                <For each={PROTOCOL_OPTIONS}>
                  {(opt) => <option value={opt.value}>{opt.label}</option>}
                </For>
              </select>
            </div>
          </div>

          <div class="mb-3">
            <label for="plan-description" class="mb-1 block text-xs font-medium text-gray-600">
              Description
            </label>
            <input
              id="plan-description"
              type="text"
              class="w-full rounded border border-gray-300 px-2 py-1.5 text-sm"
              value={description()}
              onInput={(e) => setDescription(e.currentTarget.value)}
              placeholder="Optional description"
            />
          </div>

          <Show when={protocol() === "parallel"}>
            <div class="mb-3">
              <label for="plan-max-parallel" class="mb-1 block text-xs font-medium text-gray-600">
                Max Parallel
              </label>
              <input
                id="plan-max-parallel"
                type="number"
                min="1"
                max="20"
                class="w-24 rounded border border-gray-300 px-2 py-1.5 text-sm"
                value={maxParallel()}
                onInput={(e) => setMaxParallel(parseInt(e.currentTarget.value) || 4)}
              />
            </div>
          </Show>

          <p class="mb-2 text-xs text-gray-500">
            {PROTOCOL_OPTIONS.find((o) => o.value === protocol())?.description}
          </p>

          {/* Steps */}
          <div class="mb-3">
            <div class="mb-2 flex items-center justify-between">
              <label class="text-xs font-medium text-gray-600">Steps</label>
              <button
                class="rounded bg-gray-200 px-2 py-0.5 text-xs hover:bg-gray-300"
                onClick={addStep}
              >
                + Add Step
              </button>
            </div>
            <For each={steps()}>
              {(step, idx) => (
                <div class="mb-2 flex items-center gap-2">
                  <span class="w-6 text-center text-xs text-gray-400">{idx() + 1}</span>
                  <select
                    class="flex-1 rounded border border-gray-300 px-2 py-1 text-sm"
                    value={step.task_id}
                    onChange={(e) => updateStep(idx(), "task_id", e.currentTarget.value)}
                    aria-label={`Step ${idx() + 1} task`}
                  >
                    <option value="">Select Task</option>
                    <For each={props.tasks}>{(t) => <option value={t.id}>{t.title}</option>}</For>
                  </select>
                  <select
                    class="flex-1 rounded border border-gray-300 px-2 py-1 text-sm"
                    value={step.agent_id}
                    onChange={(e) => updateStep(idx(), "agent_id", e.currentTarget.value)}
                    aria-label={`Step ${idx() + 1} agent`}
                  >
                    <option value="">Select Agent</option>
                    <For each={props.agents}>
                      {(a) => (
                        <option value={a.id}>
                          {a.name} ({a.backend})
                        </option>
                      )}
                    </For>
                  </select>
                  <Show when={steps().length > 2}>
                    <button
                      type="button"
                      class="text-xs text-red-500 hover:text-red-700"
                      onClick={() => removeStep(idx())}
                      aria-label={`Remove step ${idx() + 1}`}
                    >
                      x
                    </button>
                  </Show>
                </div>
              )}
            </For>
          </div>

          <button
            class="rounded bg-indigo-600 px-4 py-1.5 text-sm text-white hover:bg-indigo-700 disabled:opacity-50"
            onClick={handleCreate}
            disabled={creating()}
          >
            {creating() ? "Creating..." : "Create Plan"}
          </button>
        </div>
      </Show>

      {/* Plan List */}
      <Show
        when={(plans() ?? []).length > 0}
        fallback={<p class="text-sm text-gray-400">No execution plans yet.</p>}
      >
        <div class="space-y-2">
          <For each={plans()}>
            {(p) => (
              <div
                class={`cursor-pointer rounded border p-3 transition-colors ${
                  selectedPlanId() === p.id
                    ? "border-indigo-300 bg-indigo-50"
                    : "border-gray-200 hover:bg-gray-50"
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
                    <span class={`rounded px-1.5 py-0.5 text-xs ${PLAN_STATUS_COLORS[p.status]}`}>
                      {p.status}
                    </span>
                    <span class="rounded bg-gray-50 px-1.5 py-0.5 text-xs text-gray-500">
                      {p.protocol}
                    </span>
                  </div>
                  <div class="flex gap-1">
                    <Show when={p.status === "pending"}>
                      <button
                        type="button"
                        class="rounded bg-green-600 px-2 py-0.5 text-xs text-white hover:bg-green-700"
                        onClick={(e) => {
                          e.stopPropagation();
                          handleStart(p.id);
                        }}
                        aria-label={`Start plan ${p.name}`}
                      >
                        Start
                      </button>
                    </Show>
                    <Show when={p.status === "running"}>
                      <button
                        type="button"
                        class="rounded bg-red-600 px-2 py-0.5 text-xs text-white hover:bg-red-700"
                        onClick={(e) => {
                          e.stopPropagation();
                          handleCancel(p.id);
                        }}
                        aria-label={`Cancel plan ${p.name}`}
                      >
                        Cancel
                      </button>
                    </Show>
                  </div>
                </div>
                <Show when={p.description}>
                  <p class="mt-1 text-xs text-gray-500">{p.description}</p>
                </Show>
              </div>
            )}
          </For>
        </div>
      </Show>

      {/* Selected Plan Detail */}
      <Show when={selectedPlan()}>
        {(detail) => (
          <div class="mt-4 rounded border border-indigo-200 bg-indigo-50 p-4">
            <h4 class="mb-2 text-sm font-semibold">{detail().name} - Steps</h4>
            <div class="space-y-2">
              <For each={detail().steps}>
                {(step, idx) => (
                  <div class="flex items-center gap-3 rounded bg-white p-2 text-sm">
                    <span class="w-6 text-center text-xs text-gray-400">{idx() + 1}</span>
                    <span
                      class={`rounded px-1.5 py-0.5 text-xs ${STEP_STATUS_COLORS[step.status]}`}
                    >
                      {step.status}
                    </span>
                    <span class="text-gray-700">
                      {taskName(step.task_id)} / {agentName(step.agent_id)}
                    </span>
                    <Show when={step.run_id}>
                      <span class="font-mono text-xs text-gray-400">
                        run: {step.run_id.slice(0, 8)}
                      </span>
                    </Show>
                    <Show when={step.round > 0}>
                      <span class="text-xs text-gray-400">round {step.round}</span>
                    </Show>
                    <Show when={step.error}>
                      <span class="text-xs text-red-500">{step.error}</span>
                    </Show>
                  </div>
                )}
              </For>
            </div>
          </div>
        )}
      </Show>
    </div>
  );
}
