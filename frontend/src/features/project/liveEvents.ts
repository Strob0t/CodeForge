// Pure helpers that attribute WebSocket events (internal/domain/event) to the
// project page's panels. The panels refetch or update in place with them.

import type {
  DebateStatusEvent,
  ReviewDecisionSnapshot,
  Run,
  RunStatus,
  Task,
  ToolCallEvent,
} from "~/api/types";
import type { WSMessage } from "~/api/websocket";

type Payload = Record<string, unknown>;

/** A string field of an event payload, or undefined when it is missing or not a string. */
export function payloadString(payload: Payload, key: string): string | undefined {
  const value = payload[key];
  return typeof value === "string" ? value : undefined;
}

function payloadNumber(payload: Payload, key: string): number | undefined {
  const value = payload[key];
  return typeof value === "number" && Number.isFinite(value) ? value : undefined;
}

function payloadBoolean(payload: Payload, key: string): boolean | undefined {
  const value = payload[key];
  return typeof value === "boolean" ? value : undefined;
}

/** Whether the event's payload names the given project. */
export function isProjectEvent(msg: WSMessage, projectId: string): boolean {
  return projectId !== "" && payloadString(msg.payload, "project_id") === projectId;
}

// ---------------------------------------------------------------------------
// Task output
// ---------------------------------------------------------------------------

export interface TaskOutputLine {
  taskId: string;
  line: string;
  stream: "stdout" | "stderr";
}

/** Parses a task.output event; null for other events and malformed payloads. */
export function parseTaskOutput(msg: WSMessage): TaskOutputLine | null {
  if (msg.type !== "task.output") return null;
  const taskId = payloadString(msg.payload, "task_id");
  const line = payloadString(msg.payload, "line");
  if (!taskId || line === undefined) return null;
  const stream = payloadString(msg.payload, "stream") === "stderr" ? "stderr" : "stdout";
  return { taskId, line, stream };
}

/**
 * The tasks of one project, with the agent working on each when known.
 *
 * task.output carries no project ID: its sources (the workers' runs.output and
 * tasks.output messages) name only the task. Every task belongs to one project,
 * and before a task produces output the backend broadcasts a project event that
 * names it (task.status on dispatch, run.status on run start), so the index
 * knows a task by the time its output arrives, even before the task list is
 * refetched. Conversation runs stream through task.output with their run ID as
 * task ID; they are no tasks of the project and stay out (the chat shows them).
 */
export interface ProjectTaskIndex {
  readonly projectId: string;
  /** Adds the listed tasks that belong to the project (a stale list may not). */
  addTasks(tasks: readonly Pick<Task, "id" | "project_id" | "agent_id">[]): void;
  observe(msg: WSMessage): void;
  owns(taskId: string): boolean;
  agentOf(taskId: string): string | undefined;
}

export function createProjectTaskIndex(projectId: string): ProjectTaskIndex {
  const agentByTask = new Map<string, string | undefined>();
  const remember = (taskId: string, agentId: string | undefined): void => {
    agentByTask.set(taskId, agentId || agentByTask.get(taskId));
  };
  return {
    projectId,
    addTasks(tasks) {
      for (const task of tasks) {
        if (task.project_id === projectId) remember(task.id, task.agent_id);
      }
    },
    observe(msg) {
      if (!isProjectEvent(msg, projectId)) return;
      const taskId = payloadString(msg.payload, "task_id");
      if (taskId) remember(taskId, payloadString(msg.payload, "agent_id"));
    },
    owns: (taskId) => agentByTask.has(taskId),
    agentOf: (taskId) => agentByTask.get(taskId),
  };
}

// ---------------------------------------------------------------------------
// Runs
// ---------------------------------------------------------------------------

const RUN_STATUSES: readonly RunStatus[] = [
  "pending",
  "running",
  "completed",
  "failed",
  "cancelled",
  "timeout",
  "quality_gate",
];

function isRunStatus(value: string | undefined): value is RunStatus {
  return RUN_STATUSES.some((status) => status === value);
}

function isActiveRunStatus(status: RunStatus): boolean {
  return status === "pending" || status === "running" || status === "quality_gate";
}

/**
 * Applies a run.status event to the run it names; null when the event is for
 * another run. Go omits zero cost, tokens and model, so absent metrics keep
 * their current value.
 */
export function applyRunStatus(run: Run, msg: WSMessage): Run | null {
  if (msg.type !== "run.status") return null;
  const p = msg.payload;
  const status = payloadString(p, "status");
  if (payloadString(p, "run_id") !== run.id || !isRunStatus(status)) return null;
  return {
    ...run,
    status,
    step_count: payloadNumber(p, "step_count") ?? run.step_count,
    cost_usd: payloadNumber(p, "cost_usd") ?? run.cost_usd,
    tokens_in: payloadNumber(p, "tokens_in") ?? run.tokens_in,
    tokens_out: payloadNumber(p, "tokens_out") ?? run.tokens_out,
    model: payloadString(p, "model") ?? run.model,
  };
}

/** Parses a run.toolcall event; null for other events and malformed payloads. */
export function parseToolCall(msg: WSMessage): ToolCallEvent | null {
  if (msg.type !== "run.toolcall") return null;
  const p = msg.payload;
  const runId = payloadString(p, "run_id");
  const phase = payloadString(p, "phase");
  if (!runId || !phase) return null;
  const call: ToolCallEvent = {
    run_id: runId,
    call_id: payloadString(p, "call_id") ?? "",
    tool: payloadString(p, "tool") ?? "",
    phase,
  };
  const decision = payloadString(p, "decision");
  if (decision) call.decision = decision;
  return call;
}

// ---------------------------------------------------------------------------
// Agent lanes
// ---------------------------------------------------------------------------

/** The run and task an agent works on, and the run's progress. */
export interface AgentWork {
  runId?: string;
  taskId?: string;
  steps: number;
  costUsd: number;
}

export const IDLE_WORK: AgentWork = { steps: 0, costUsd: 0 };

/**
 * Follows an agent's work through the events that name it: run.status (run
 * start, quality gate, completion) and task.status / activework.claimed
 * (dispatch). Returns `work` itself when the event changes nothing.
 */
export function reduceAgentWork(work: AgentWork, msg: WSMessage, agentId: string): AgentWork {
  const p = msg.payload;
  if (payloadString(p, "agent_id") !== agentId) return work;
  const taskId = payloadString(p, "task_id");

  switch (msg.type) {
    case "run.status": {
      const runId = payloadString(p, "run_id");
      if (!runId) return work;
      const sameRun = runId === work.runId;
      return {
        runId,
        taskId: taskId ?? (sameRun ? work.taskId : undefined),
        steps: payloadNumber(p, "step_count") ?? (sameRun ? work.steps : 0),
        costUsd: payloadNumber(p, "cost_usd") ?? (sameRun ? work.costUsd : 0),
      };
    }
    case "task.status":
    case "activework.claimed":
      if (!taskId || taskId === work.taskId) return work;
      return { taskId, steps: 0, costUsd: 0 };
    default:
      return work;
  }
}

/** The agent's newest active run (runs newest first), for a lane opened mid-run. */
export function agentWorkFromRuns(runs: readonly Run[], agentId: string): AgentWork | undefined {
  const r = runs.find(
    (candidate) => candidate.agent_id === agentId && isActiveRunStatus(candidate.status),
  );
  return r
    ? { runId: r.id, taskId: r.task_id, steps: r.step_count, costUsd: r.cost_usd }
    : undefined;
}

// ---------------------------------------------------------------------------
// Plans
// ---------------------------------------------------------------------------

/** What the plan panel does for a plan event. */
export interface PlanEventEffect {
  refetchPlans: boolean;
  refetchSelected: boolean;
  reviewDecision?: { stepId: string; decision: ReviewDecisionSnapshot };
  debate?: DebateStatusEvent;
}

function parseReviewDecision(p: Payload): ReviewDecisionSnapshot | undefined {
  const needsReview = payloadBoolean(p, "needs_review");
  const confidence = payloadNumber(p, "confidence");
  if (needsReview === undefined || confidence === undefined) return undefined;
  return {
    needs_review: needsReview,
    confidence,
    reason: payloadString(p, "reason") ?? "",
    routed: payloadBoolean(p, "routed") ?? false,
  };
}

function parseDebate(p: Payload): DebateStatusEvent | undefined {
  const planId = payloadString(p, "plan_id");
  const stepId = payloadString(p, "step_id");
  const projectId = payloadString(p, "project_id");
  const status = payloadString(p, "status");
  if (!planId || !stepId || !projectId) return undefined;
  if (status !== "started" && status !== "completed" && status !== "failed") return undefined;
  const debate: DebateStatusEvent = {
    plan_id: planId,
    step_id: stepId,
    project_id: projectId,
    debate_plan_id: payloadString(p, "debate_plan_id") ?? "",
    status,
  };
  const synthesis = payloadString(p, "synthesis");
  if (synthesis) debate.synthesis = synthesis;
  return debate;
}

/**
 * Maps a project's plan.status, plan.step.status, review_router.decision and
 * debate.status events to the plan panel's reaction; null for other events.
 */
export function planEventEffect(
  msg: WSMessage,
  projectId: string,
  selectedPlanId: string | null,
): PlanEventEffect | null {
  if (!isProjectEvent(msg, projectId)) return null;
  const p = msg.payload;
  const planId = payloadString(p, "plan_id");
  const selected = planId !== undefined && planId === selectedPlanId;
  const stepId = payloadString(p, "step_id");

  switch (msg.type) {
    case "plan.status":
      return { refetchPlans: true, refetchSelected: selected };
    case "plan.step.status": {
      const effect: PlanEventEffect = { refetchPlans: true, refetchSelected: selected };
      const snapshot = p.review_decision;
      if (stepId && typeof snapshot === "object" && snapshot !== null) {
        const decision = parseReviewDecision(snapshot as Payload);
        if (decision) effect.reviewDecision = { stepId, decision };
      }
      return effect;
    }
    case "review_router.decision": {
      const decision = parseReviewDecision(p);
      if (!stepId || !decision) return null;
      return { refetchPlans: false, refetchSelected: false, reviewDecision: { stepId, decision } };
    }
    case "debate.status": {
      const debate = parseDebate(p);
      return debate ? { refetchPlans: false, refetchSelected: false, debate } : null;
    }
    default:
      return null;
  }
}
