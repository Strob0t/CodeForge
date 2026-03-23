import { createResource, createSignal, onCleanup } from "solid-js";

import { api } from "~/api/client";
import type { AutoAgentStatus, BudgetAlertEvent } from "~/api/types";
import { useToast } from "~/components/Toast";
import { useWebSocket } from "~/components/WebSocketProvider";
import { useI18n } from "~/i18n";

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
  const [tasks, { refetch: refetchTasks }] = createResource(projectId, (id) => api.tasks.list(id));
  const [gitStatus, { refetch: refetchGitStatus }] = createResource(
    () => (project()?.workspace_path ? projectId() : undefined),
    (id: string) => api.projects.gitStatus(id),
  );
  const [, { refetch: refetchBranches }] = createResource(
    () => (project()?.workspace_path ? projectId() : undefined),
    (id: string) => api.projects.branches(id),
  );
  const [agents, { refetch: refetchAgents }] = createResource(projectId, (id) =>
    api.agents.list(id),
  );

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

  const cleanup = onMessage((msg) => {
    const payload = msg.payload;
    const pid = projectId();

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
        if ((payload.project_id as string) === pid) {
          const status = payload.status as string;
          if (status === "completed") toast("info", t("detail.toast.runCompleted"));
          else if (status === "failed") toast("error", t("detail.toast.runFailed"));
          else if (status === "cancelled") toast("info", t("detail.toast.runCancelled"));

          const costUsd = payload.cost_usd as number | undefined;
          if (costUsd !== undefined) {
            setActiveRunCost({
              costUsd,
              tokensIn: (payload.tokens_in as number) ?? 0,
              tokensOut: (payload.tokens_out as number) ?? 0,
              steps: (payload.steps as number) ?? 0,
              model: payload.model as string | undefined,
            });
          }
        }
        break;
      }
      case "run.toolcall":
        break;
      case "run.qualitygate":
      case "run.delivery":
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
        if ((payload.project_id as string) === pid) {
          const taskId = (payload.task_id as string) ?? null;
          const line = payload.line as string;
          const stream = (payload.stream as "stdout" | "stderr") ?? "stdout";
          const agentId = payload.agent_id as string | undefined;
          const agentName = payload.agent_name as string | undefined;

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
                      agentName: agentName ?? agentId,
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
      const msg = e instanceof Error ? e.message : t("detail.toast.cloneFailed");
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
      const msg = e instanceof Error ? e.message : t("detail.toast.pullFailed");
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
    agents,
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
