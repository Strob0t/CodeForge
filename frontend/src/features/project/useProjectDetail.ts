import { createEffect, createResource, createSignal, onCleanup } from "solid-js";

import { api } from "~/api/client";
import type { AutoAgentStatus, BudgetAlertEvent, GitStatus } from "~/api/types";
import { useToast } from "~/components/Toast";
import { useWebSocket } from "~/components/WebSocketProvider";
import { useI18n } from "~/i18n";
import { coalesce } from "~/lib/coalesce";
import { extractErrorMessage } from "~/lib/errorUtils";

import { createProjectTaskIndex, parseTaskOutput } from "./liveEvents";
import type { OutputLine } from "./LiveOutput";
import type { AgentTerminal } from "./MultiTerminal";

function isBudgetAlertEvent(p: unknown): p is BudgetAlertEvent {
  return (
    typeof p === "object" && p !== null && "run_id" in p && "percentage" in p && "cost_usd" in p
  );
}

function isAutoAgentStatus(p: unknown): p is AutoAgentStatus {
  return typeof p === "object" && p !== null && "id" in p && "project_id" in p && "status" in p;
}

/** The run statuses after which the run no longer changes the workspace. */
const RUN_ENDED = new Set(["completed", "failed", "cancelled", "timeout"]);

export interface RunCostState {
  costUsd: number;
  tokensIn: number;
  tokensOut: number;
  steps: number;
  model?: string;
}

/**
 * Custom hook that encapsulates data-fetching resources, WebSocket event
 * handling, and related state for the ProjectDetailPage.
 *
 * Extracted to reduce ProjectDetailPage from ~1036 LOC to ~800 LOC (render-focused).
 */
export function useProjectDetail(projectId: () => string) {
  const { t, fmt } = useI18n();
  const { show: toast } = useToast();
  const { onMessage } = useWebSocket();

  // ---- Data resources ----

  const [project, { refetch: refetchProject }] = createResource(projectId, (id) =>
    api.projects.get(id),
  );
  // Reading a resource that failed throws; outside JSX (event handlers,
  // resource sources) and for the branch badge, a failure reads as "none".
  const loadedProject = () => (project.error ? undefined : project());
  const [tasks, { refetch: refetchTasks }] = createResource(projectId, (id) => api.tasks.list(id));
  const [gitStatusResource, { refetch: refetchGitStatus }] = createResource(
    () => (loadedProject()?.workspace_path ? projectId() : undefined),
    (id: string) => api.projects.gitStatus(id),
  );
  // The badge reads the status; a failed refresh hides the badge instead of
  // throwing to the page's error boundary.
  const gitStatus = (): GitStatus | undefined =>
    gitStatusResource.error ? undefined : gitStatusResource();
  const [, { refetch: refetchBranches }] = createResource(
    () => (loadedProject()?.workspace_path ? projectId() : undefined),
    (id: string) => api.projects.branches(id),
  );
  const [agents, { refetch: refetchAgents }] = createResource(projectId, (id) =>
    api.agents.list(id),
  );

  // What agents do in the workspace (a run ends or delivers, a chat tool
  // call) refreshes the branch badge (KI-129): one git status request at a
  // time, and one more after it when activity came in meanwhile.
  const refetchGitStatusCoalesced = coalesce(refetchGitStatus);
  const refreshGitStatus = (): void => {
    if (loadedProject()?.workspace_path) refetchGitStatusCoalesced();
  };

  // Onboarding data
  const [onboardGoals] = createResource(projectId, (pid) => api.goals.list(pid).catch(() => []));
  const [onboardRoadmap] = createResource(projectId, (pid) =>
    api.roadmap.get(pid).catch(() => null),
  );
  const [onboardSessions] = createResource(projectId, (pid) =>
    api.sessions.list(pid).catch(() => []),
  );

  // ---- UI state ----

  const [cloning, setCloning] = createSignal(false);
  const [pulling, setPulling] = createSignal(false);
  const [error, setError] = createSignal("");
  const [budgetAlert, setBudgetAlert] = createSignal<BudgetAlertEvent | null>(null);
  const [settingsOpen, setSettingsOpen] = createSignal(false);
  const [showCanvas, setShowCanvas] = createSignal(false);
  const [autoAgentStatus, setAutoAgentStatus] = createSignal<AutoAgentStatus | undefined>();

  // WS-driven state for LiveOutput, MultiTerminal, CostBreakdown panels
  const [liveOutputTaskId, setLiveOutputTaskId] = createSignal<string | null>(null);
  const [liveOutputLines, setLiveOutputLines] = createSignal<OutputLine[]>([]);
  const [agentTerminals, setAgentTerminals] = createSignal<AgentTerminal[]>([]);
  const [activeRunCost, setActiveRunCost] = createSignal<RunCostState | null>(null);

  // ---- WS event handling ----

  // task.output names only its task: attribute it through the project's tasks.
  let taskIndex = createProjectTaskIndex(projectId());
  createEffect(() => {
    const id = projectId();
    if (taskIndex.projectId !== id) taskIndex = createProjectTaskIndex(id);
    if (!tasks.error) taskIndex.addTasks(tasks() ?? []);
  });
  const agentName = (agentId: string): string =>
    (agents.error ? undefined : agents())?.find((a) => a.id === agentId)?.name ?? agentId;

  // eslint-disable-next-line solid/reactivity -- subscription callback, not a reactive computation
  const cleanup = onMessage((msg) => {
    const payload = msg.payload;
    const pid = projectId();
    taskIndex.observe(msg);

    switch (msg.type) {
      case "task.status": {
        if ((payload.project_id as string) === pid) refetchTasks();
        break;
      }
      case "agent.status": {
        if ((payload.project_id as string) === pid) refetchAgents();
        break;
      }
      case "run.status": {
        // The run's task and agent announce their own status (task.status,
        // agent.status), so the lists are not refetched here.
        if ((payload.project_id as string) === pid) {
          const status = payload.status as string;
          if (status === "completed") toast("info", t("detail.toast.runCompleted"));
          else if (status === "failed") toast("error", t("detail.toast.runFailed"));
          else if (status === "cancelled") toast("info", t("detail.toast.runCancelled"));
          if (RUN_ENDED.has(status)) refreshGitStatus();

          const costUsd = payload.cost_usd as number | undefined;
          if (costUsd !== undefined) {
            setActiveRunCost({
              costUsd,
              tokensIn: (payload.tokens_in as number) ?? 0,
              tokensOut: (payload.tokens_out as number) ?? 0,
              steps: (payload.step_count as number) ?? 0,
              model: payload.model as string | undefined,
            });
          }
        }
        break;
      }
      case "run.toolcall":
        break;
      case "run.delivery": {
        // A delivery commits, and may create and check out a branch.
        if ((payload.project_id as string) === pid && payload.status === "completed") {
          refreshGitStatus();
        }
        break;
      }
      case "run.qualitygate":
      case "plan.step.status":
      case "repomap.status":
      case "retrieval.status":
      case "roadmap.status":
        break;
      case "plan.status": {
        if ((payload.project_id as string) === pid) {
          const status = payload.status as string;
          if (status === "completed") toast("info", t("detail.toast.planCompleted"));
          else if (status === "failed") toast("error", t("detail.toast.planFailed"));
        }
        break;
      }
      case "run.budget_alert": {
        if ((payload.project_id as string) === pid && isBudgetAlertEvent(payload)) {
          setBudgetAlert(payload);
          toast("warning", t("detail.toast.budgetAlert", { pct: fmt.percent(payload.percentage) }));
        }
        break;
      }
      case "task.output": {
        const output = parseTaskOutput(msg);
        if (!output || !taskIndex.owns(output.taskId)) break;
        const { taskId, line, stream } = output;
        const agentId = taskIndex.agentOf(taskId);

        setLiveOutputTaskId(taskId);
        setLiveOutputLines((prev) => [...prev, { line, stream, timestamp: Date.now() }]);

        if (agentId) {
          setAgentTerminals((prev) => {
            const idx = prev.findIndex((at) => at.agentId === agentId);
            const entry: AgentTerminal =
              idx >= 0
                ? {
                    ...prev[idx],
                    lines: [...prev[idx].lines, { line, stream, timestamp: Date.now() }],
                  }
                : {
                    agentId,
                    agentName: agentName(agentId),
                    lines: [{ line, stream, timestamp: Date.now() }],
                  };
            if (idx >= 0) {
              const next = [...prev];
              next[idx] = entry;
              return next;
            }
            return [...prev, entry];
          });
        }
        break;
      }
      case "autoagent.status": {
        if ((payload.project_id as string) === pid && isAutoAgentStatus(payload)) {
          setAutoAgentStatus(payload);
        }
        break;
      }
      case "activework.claimed":
      case "activework.released":
        break;
    }
  });
  onCleanup(cleanup);

  // ---- Action handlers ----

  const handleClone = async () => {
    setCloning(true);
    setError("");
    try {
      await api.projects.clone(projectId());
      refetchProject();
      refetchGitStatus();
      refetchBranches();
      toast("success", t("detail.toast.cloned"));
    } catch (e) {
      const msg = extractErrorMessage(e, t("detail.toast.cloneFailed"));
      setError(msg);
      toast("error", msg);
    } finally {
      setCloning(false);
    }
  };

  const handlePull = async () => {
    setPulling(true);
    setError("");
    try {
      await api.projects.pull(projectId());
      refetchGitStatus();
      toast("success", t("detail.toast.pulled"));
    } catch (e) {
      const msg = extractErrorMessage(e, t("detail.toast.pullFailed"));
      setError(msg);
      toast("error", msg);
    } finally {
      setPulling(false);
    }
  };

  return {
    // Resources
    project,
    refetchProject,
    tasks,
    refetchTasks,
    gitStatus,
    refreshGitStatus,
    agents,
    refetchAgents,
    onboardGoals,
    onboardRoadmap,
    onboardSessions,

    // UI state
    cloning,
    pulling,
    error,
    setError,
    budgetAlert,
    setBudgetAlert,
    settingsOpen,
    setSettingsOpen,
    showCanvas,
    setShowCanvas,
    autoAgentStatus,

    // WS-driven state
    liveOutputTaskId,
    liveOutputLines,
    agentTerminals,
    activeRunCost,

    // Actions
    handleClone,
    handlePull,
  };
}
