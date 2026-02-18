import { createResource, createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type { Agent, DeliverMode, Run, RunStatus, Task, ToolCallEvent } from "~/api/types";

import TrajectoryPanel from "./TrajectoryPanel";

interface RunPanelProps {
  projectId: string;
  tasks: Task[];
  agents: Agent[];
  onError: (msg: string) => void;
}

const STATUS_COLORS: Record<RunStatus, string> = {
  pending: "bg-gray-100 text-gray-700",
  running: "bg-blue-100 text-blue-700",
  completed: "bg-green-100 text-green-700",
  failed: "bg-red-100 text-red-700",
  cancelled: "bg-yellow-100 text-yellow-700",
  timeout: "bg-orange-100 text-orange-700",
  quality_gate: "bg-purple-100 text-purple-700",
};

const DELIVER_MODES: { value: DeliverMode; label: string }[] = [
  { value: "", label: "None" },
  { value: "patch", label: "Patch" },
  { value: "commit-local", label: "Commit (local)" },
  { value: "branch", label: "Branch" },
  { value: "pr", label: "Pull Request" },
];

export default function RunPanel(props: RunPanelProps) {
  const [selectedTaskId, setSelectedTaskId] = createSignal("");
  const [selectedAgentId, setSelectedAgentId] = createSignal("");
  const [selectedPolicy, setSelectedPolicy] = createSignal("");
  const [selectedDeliverMode, setSelectedDeliverMode] = createSignal<DeliverMode>("");
  const [starting, setStarting] = createSignal(false);
  const [activeRun, setActiveRun] = createSignal<Run | null>(null);
  const [toolCalls, setToolCalls] = createSignal<ToolCallEvent[]>([]);
  const [trajectoryRunId, setTrajectoryRunId] = createSignal<string | null>(null);

  const [policies] = createResource(() => api.policies.list());

  const [taskRuns, { refetch: refetchRuns }] = createResource(
    () => selectedTaskId(),
    (taskId) => (taskId ? api.runs.listByTask(taskId) : []),
  );

  const pendingTasks = () =>
    props.tasks.filter((t) => t.status === "pending" || t.status === "queued");
  const idleAgents = () => props.agents.filter((a) => a.status === "idle");

  const handleStart = async () => {
    const taskId = selectedTaskId();
    const agentId = selectedAgentId();
    if (!taskId || !agentId) return;

    setStarting(true);
    props.onError("");
    try {
      const run = await api.runs.start({
        task_id: taskId,
        agent_id: agentId,
        project_id: props.projectId,
        policy_profile: selectedPolicy() || undefined,
        deliver_mode: selectedDeliverMode() || undefined,
      });
      setActiveRun(run);
      setToolCalls([]);
      refetchRuns();
    } catch (e) {
      props.onError(e instanceof Error ? e.message : "Failed to start run");
    } finally {
      setStarting(false);
    }
  };

  const handleCancel = async () => {
    const run = activeRun();
    if (!run) return;
    try {
      await api.runs.cancel(run.id);
      setActiveRun(null);
      refetchRuns();
    } catch (e) {
      props.onError(e instanceof Error ? e.message : "Failed to cancel run");
    }
  };

  // Called from parent via ref or WS event forwarding
  const updateRunStatus = (
    runId: string,
    status: RunStatus,
    stepCount: number,
    costUsd: number,
    tokensIn?: number,
    tokensOut?: number,
    model?: string,
  ) => {
    const current = activeRun();
    if (current && current.id === runId) {
      setActiveRun({
        ...current,
        status,
        step_count: stepCount,
        cost_usd: costUsd,
        tokens_in: tokensIn ?? current.tokens_in,
        tokens_out: tokensOut ?? current.tokens_out,
        model: model ?? current.model,
      });
      if (
        status === "completed" ||
        status === "failed" ||
        status === "cancelled" ||
        status === "timeout"
      ) {
        refetchRuns();
      }
    }
  };

  const addToolCall = (ev: ToolCallEvent) => {
    const current = activeRun();
    if (current && current.id === ev.run_id) {
      setToolCalls((prev) => [...prev.slice(-49), ev]);
    }
  };

  // Expose methods for parent
  (RunPanel as unknown as { updateRunStatus: typeof updateRunStatus }).updateRunStatus =
    updateRunStatus;
  (RunPanel as unknown as { addToolCall: typeof addToolCall }).addToolCall = addToolCall;

  return (
    <div class="rounded-lg border border-gray-200 bg-white p-4">
      <h3 class="mb-3 text-lg font-semibold">Run Management</h3>

      {/* Start Run Form */}
      <div class="mb-4 flex flex-wrap gap-2">
        <select
          class="rounded border border-gray-300 px-2 py-1.5 text-sm"
          value={selectedTaskId()}
          onChange={(e) => {
            setSelectedTaskId(e.currentTarget.value);
            refetchRuns();
          }}
        >
          <option value="">Select task...</option>
          <For each={pendingTasks()}>
            {(t) => (
              <option value={t.id}>
                {t.title.slice(0, 40)}
                {t.title.length > 40 ? "..." : ""}
              </option>
            )}
          </For>
        </select>

        <select
          class="rounded border border-gray-300 px-2 py-1.5 text-sm"
          value={selectedAgentId()}
          onChange={(e) => setSelectedAgentId(e.currentTarget.value)}
        >
          <option value="">Select agent...</option>
          <For each={idleAgents()}>
            {(a) => (
              <option value={a.id}>
                {a.name} ({a.backend})
              </option>
            )}
          </For>
        </select>

        <select
          class="rounded border border-gray-300 px-2 py-1.5 text-sm"
          value={selectedPolicy()}
          onChange={(e) => setSelectedPolicy(e.currentTarget.value)}
        >
          <option value="">Default policy</option>
          <For each={policies()?.profiles ?? []}>{(p) => <option value={p}>{p}</option>}</For>
        </select>

        <select
          class="rounded border border-gray-300 px-2 py-1.5 text-sm"
          value={selectedDeliverMode()}
          onChange={(e) => setSelectedDeliverMode(e.currentTarget.value as DeliverMode)}
        >
          <For each={DELIVER_MODES}>{(m) => <option value={m.value}>{m.label}</option>}</For>
        </select>

        <button
          class="rounded bg-blue-600 px-4 py-1.5 text-sm text-white hover:bg-blue-700 disabled:opacity-50"
          onClick={handleStart}
          disabled={starting() || !selectedTaskId() || !selectedAgentId()}
        >
          {starting() ? "Starting..." : "Start Run"}
        </button>
      </div>

      {/* Active Run */}
      <Show when={activeRun()}>
        {(run) => (
          <div class="mb-4 rounded border border-blue-200 bg-blue-50 p-3">
            <div class="mb-2 flex items-center justify-between">
              <div class="flex items-center gap-2">
                <span
                  class={`rounded px-2 py-0.5 text-xs font-medium ${STATUS_COLORS[run().status]}`}
                >
                  {run().status}
                </span>
                <span class="text-xs text-gray-500">Run: {run().id.slice(0, 8)}</span>
              </div>
              <Show when={run().status === "running" || run().status === "quality_gate"}>
                <button
                  class="rounded bg-red-500 px-3 py-1 text-xs text-white hover:bg-red-600"
                  onClick={handleCancel}
                >
                  Cancel
                </button>
              </Show>
            </div>
            <div class="flex flex-wrap gap-4 text-sm text-gray-600">
              <span>Steps: {run().step_count}</span>
              <span>Cost: ${run().cost_usd.toFixed(4)}</span>
              <Show when={run().tokens_in > 0 || run().tokens_out > 0}>
                <span>
                  Tokens: {run().tokens_in.toLocaleString()} in /{" "}
                  {run().tokens_out.toLocaleString()} out
                </span>
              </Show>
              <Show when={run().model}>
                <span>Model: {run().model}</span>
              </Show>
              <span>Policy: {run().policy_profile}</span>
              <Show when={run().deliver_mode}>
                <span>Deliver: {run().deliver_mode}</span>
              </Show>
            </div>

            {/* Tool Call Activity */}
            <Show when={toolCalls().length > 0}>
              <div class="mt-2 max-h-32 overflow-y-auto text-xs">
                <For each={toolCalls()}>
                  {(tc) => (
                    <div class="flex gap-2 py-0.5 text-gray-500">
                      <span class="font-mono">{tc.tool || "?"}</span>
                      <span class={tc.phase === "denied" ? "text-red-500" : "text-green-600"}>
                        {tc.phase}
                      </span>
                    </div>
                  )}
                </For>
              </div>
            </Show>
          </div>
        )}
      </Show>

      {/* Trajectory Panel */}
      <Show when={trajectoryRunId()}>
        {(runId) => (
          <div class="mb-4">
            <TrajectoryPanel runId={runId()} />
          </div>
        )}
      </Show>

      {/* Run History */}
      <Show when={selectedTaskId() && (taskRuns() ?? []).length > 0}>
        <div>
          <h4 class="mb-2 text-sm font-medium text-gray-500">Run History</h4>
          <div class="space-y-1">
            <For each={taskRuns() ?? []}>
              {(r) => (
                <div class="flex items-center justify-between rounded bg-gray-50 px-3 py-2 text-sm">
                  <div class="flex items-center gap-2">
                    <span class={`rounded px-1.5 py-0.5 text-xs ${STATUS_COLORS[r.status]}`}>
                      {r.status}
                    </span>
                    <span class="font-mono text-xs text-gray-400">{r.id.slice(0, 8)}</span>
                  </div>
                  <div class="flex gap-3 text-xs text-gray-500">
                    <span>{r.step_count} steps</span>
                    <span>${r.cost_usd.toFixed(4)}</span>
                    <Show when={r.tokens_in > 0 || r.tokens_out > 0}>
                      <span>
                        {r.tokens_in.toLocaleString()}/{r.tokens_out.toLocaleString()} tok
                      </span>
                    </Show>
                    <Show when={r.model}>
                      <span class="font-mono">{r.model}</span>
                    </Show>
                    <Show when={r.deliver_mode}>
                      <span>{r.deliver_mode}</span>
                    </Show>
                    <Show when={r.error}>
                      <span class="text-red-500" title={r.error}>
                        error
                      </span>
                    </Show>
                    <button
                      class="text-blue-600 hover:text-blue-800"
                      onClick={(e) => {
                        e.stopPropagation();
                        setTrajectoryRunId((prev) => (prev === r.id ? null : r.id));
                      }}
                    >
                      {trajectoryRunId() === r.id ? "Hide Trajectory" : "Trajectory"}
                    </button>
                  </div>
                </div>
              )}
            </For>
          </div>
        </div>
      </Show>
    </div>
  );
}
