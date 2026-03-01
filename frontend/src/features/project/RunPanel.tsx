import { createResource, createSignal, For, Show } from "solid-js";

import { api } from "~/api/client";
import type { Agent, DeliverMode, Run, RunStatus, Task, ToolCallEvent } from "~/api/types";
import { StepProgress } from "~/components/StepProgress";
import { useToast } from "~/components/Toast";
import { getVariant, runStatusVariant } from "~/config/statusVariants";
import { useI18n } from "~/i18n";
import { Badge, Button, Card, Select } from "~/ui";

import TrajectoryPanel from "./TrajectoryPanel";

interface RunPanelProps {
  projectId: string;
  tasks: Task[];
  agents: Agent[];
  onError: (msg: string) => void;
}

export default function RunPanel(props: RunPanelProps) {
  const { t, tp, fmt } = useI18n();
  const { show: toast } = useToast();

  const DELIVER_MODES = (): { value: DeliverMode; label: string }[] => [
    { value: "", label: t("run.deliver.none") },
    { value: "patch", label: t("run.deliver.patch") },
    { value: "commit-local", label: t("run.deliver.commitLocal") },
    { value: "branch", label: t("run.deliver.branch") },
    { value: "pr", label: t("run.deliver.pr") },
  ];
  const [selectedTaskId, setSelectedTaskId] = createSignal("");
  const [selectedAgentId, setSelectedAgentId] = createSignal("");
  const [selectedPolicy, setSelectedPolicy] = createSignal("");
  const [selectedDeliverMode, setSelectedDeliverMode] = createSignal<DeliverMode>("");
  const [starting, setStarting] = createSignal(false);
  const [activeRun, setActiveRun] = createSignal<Run | null>(null);
  const [runMaxSteps, setRunMaxSteps] = createSignal(0);
  const [toolCalls, setToolCalls] = createSignal<ToolCallEvent[]>([]);
  const [trajectoryRunId, setTrajectoryRunId] = createSignal<string | null>(null);

  const [policies] = createResource(() => api.policies.list());

  const [taskRuns, { refetch: refetchRuns }] = createResource(
    () => selectedTaskId(),
    (taskId) => (taskId ? api.runs.listByTask(taskId) : []),
  );

  const pendingTasks = () =>
    props.tasks.filter((task) => task.status === "pending" || task.status === "queued");
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
      // Fetch max_steps from the policy for progress indicator
      const policyName = run.policy_profile;
      if (policyName) {
        api.policies
          .get(policyName)
          .then((p) => {
            setRunMaxSteps(p.termination?.max_steps ?? 0);
          })
          .catch(() => setRunMaxSteps(0));
      } else {
        setRunMaxSteps(0);
      }
      refetchRuns();
      toast("success", t("run.toast.started"));
    } catch (e) {
      const msg = e instanceof Error ? e.message : t("run.toast.startFailed");
      props.onError(msg);
      toast("error", msg);
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
      toast("success", t("run.toast.cancelled"));
    } catch (e) {
      const msg = e instanceof Error ? e.message : t("run.toast.cancelFailed");
      props.onError(msg);
      toast("error", msg);
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
    <Card>
      <Card.Header>
        <h3 class="text-lg font-semibold">{t("run.title")}</h3>
      </Card.Header>

      <Card.Body>
        {/* Start Run Form */}
        <div class="mb-4 flex flex-wrap gap-2">
          <Select
            value={selectedTaskId()}
            aria-label="Select task for run"
            onChange={(e) => {
              setSelectedTaskId(e.currentTarget.value);
              refetchRuns();
            }}
          >
            <option value="">{t("run.selectTask")}</option>
            <For each={pendingTasks()}>
              {(task) => (
                <option value={task.id}>
                  {task.title.slice(0, 40)}
                  {task.title.length > 40 ? "..." : ""}
                </option>
              )}
            </For>
          </Select>

          <Select
            value={selectedAgentId()}
            aria-label="Select agent for run"
            onChange={(e) => setSelectedAgentId(e.currentTarget.value)}
          >
            <option value="">{t("run.selectAgent")}</option>
            <For each={idleAgents()}>
              {(a) => (
                <option value={a.id}>
                  {a.name} ({a.backend})
                </option>
              )}
            </For>
          </Select>

          <Select
            value={selectedPolicy()}
            aria-label="Select policy profile"
            onChange={(e) => setSelectedPolicy(e.currentTarget.value)}
          >
            <option value="">{t("run.defaultPolicy")}</option>
            <For each={policies()?.profiles ?? []}>{(p) => <option value={p}>{p}</option>}</For>
          </Select>

          <Select
            value={selectedDeliverMode()}
            aria-label="Select delivery mode"
            onChange={(e) => setSelectedDeliverMode(e.currentTarget.value as DeliverMode)}
          >
            <For each={DELIVER_MODES()}>{(m) => <option value={m.value}>{m.label}</option>}</For>
          </Select>

          <Button
            variant="primary"
            size="sm"
            onClick={handleStart}
            disabled={starting() || !selectedTaskId() || !selectedAgentId()}
            loading={starting()}
          >
            {t("run.startRun")}
          </Button>
        </div>

        {/* Active Run */}
        <Show when={activeRun()}>
          {(run) => (
            <div
              class="mb-4 rounded-cf-md border border-cf-accent/30 bg-cf-accent/5 p-3"
              aria-live="polite"
            >
              <div class="mb-2 flex items-center justify-between">
                <div class="flex items-center gap-2">
                  <Badge variant={getVariant(runStatusVariant, run().status)}>{run().status}</Badge>
                  <span class="text-xs text-cf-text-muted">
                    {t("run.runLabel")} {run().id.slice(0, 8)}
                  </span>
                </div>
                <Show when={run().status === "running" || run().status === "quality_gate"}>
                  <Button
                    variant="danger"
                    size="sm"
                    onClick={handleCancel}
                    aria-label={t("run.cancelAria")}
                  >
                    {t("run.cancel")}
                  </Button>
                </Show>
              </div>
              <Show when={run().status === "running" || run().status === "quality_gate"}>
                <div class="mb-2">
                  <StepProgress current={run().step_count} max={runMaxSteps() || undefined} />
                </div>
              </Show>
              <div class="flex flex-wrap gap-4 text-sm text-cf-text-tertiary">
                <span>{tp("run.steps", run().step_count)}</span>
                <span>
                  {t("run.cost")} {fmt.currency(run().cost_usd)}
                </span>
                <Show when={run().tokens_in > 0 || run().tokens_out > 0}>
                  <span>
                    {t("run.tokens")} {fmt.number(run().tokens_in)} in /{" "}
                    {fmt.number(run().tokens_out)} out
                  </span>
                </Show>
                <Show when={run().model}>
                  <span>
                    {t("run.model")} {run().model}
                  </span>
                </Show>
                <span>
                  {t("run.policy")} {run().policy_profile}
                </span>
                <Show when={run().deliver_mode}>
                  <span>
                    {t("run.deliver")} {run().deliver_mode}
                  </span>
                </Show>
              </div>

              {/* Tool Call Activity */}
              <Show when={toolCalls().length > 0}>
                <div class="mt-2 max-h-32 overflow-y-auto text-xs">
                  <For each={toolCalls()}>
                    {(tc) => (
                      <div class="flex gap-2 py-0.5 text-cf-text-muted">
                        <span class="font-mono">{tc.tool || "?"}</span>
                        <span
                          class={
                            tc.phase === "denied"
                              ? "text-red-500 dark:text-red-400"
                              : "text-green-600 dark:text-green-400"
                          }
                        >
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
            <h4 class="mb-2 text-sm font-medium text-cf-text-tertiary">{t("run.history")}</h4>
            <div class="space-y-1">
              <For each={taskRuns() ?? []}>
                {(r) => (
                  <div class="flex items-center justify-between rounded-cf-sm bg-cf-bg-inset px-3 py-2 text-sm">
                    <div class="flex items-center gap-2">
                      <Badge variant={getVariant(runStatusVariant, r.status)} pill>
                        {r.status}
                      </Badge>
                      <span class="font-mono text-xs text-cf-text-muted">{r.id.slice(0, 8)}</span>
                    </div>
                    <div class="flex gap-3 text-xs text-cf-text-tertiary">
                      <span>{tp("run.steps", r.step_count)}</span>
                      <span>{fmt.currency(r.cost_usd)}</span>
                      <Show when={r.tokens_in > 0 || r.tokens_out > 0}>
                        <span>
                          {fmt.number(r.tokens_in)}/{fmt.number(r.tokens_out)} tok
                        </span>
                      </Show>
                      <Show when={r.model}>
                        <span class="font-mono">{r.model}</span>
                      </Show>
                      <Show when={r.deliver_mode}>
                        <span>{r.deliver_mode}</span>
                      </Show>
                      <Show when={r.error}>
                        <span class="text-red-500 dark:text-red-400" title={r.error}>
                          {t("common.error")}
                        </span>
                      </Show>
                      <button
                        type="button"
                        class="text-cf-accent hover:underline"
                        onClick={(e) => {
                          e.stopPropagation();
                          setTrajectoryRunId((prev) => (prev === r.id ? null : r.id));
                        }}
                        aria-label={
                          trajectoryRunId() === r.id
                            ? t("run.hideTrajectoryAria")
                            : t("run.showTrajectoryAria")
                        }
                        aria-expanded={trajectoryRunId() === r.id}
                      >
                        {trajectoryRunId() === r.id
                          ? t("run.hideTrajectory")
                          : t("run.showTrajectory")}
                      </button>
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
