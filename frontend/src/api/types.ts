/** Matches Go domain/project.Project */
export interface Project {
  id: string;
  name: string;
  description: string;
  repo_url: string;
  provider: string;
  config: Record<string, string>;
  workspace_path?: string;
  created_at: string;
  updated_at: string;
}

/** Matches Go domain/project.CreateRequest */
export interface CreateProjectRequest {
  name: string;
  description: string;
  repo_url: string;
  provider: string;
  config: Record<string, string>;
}

/** Task status enum matching Go domain/task.Status */
export type TaskStatus = "pending" | "queued" | "running" | "completed" | "failed" | "cancelled";

/** Matches Go domain/task.Result */
export interface TaskResult {
  output: string;
  files?: string[];
  error?: string;
  tokens_in: number;
  tokens_out: number;
}

/** Matches Go domain/task.Task */
export interface Task {
  id: string;
  project_id: string;
  agent_id?: string;
  title: string;
  prompt: string;
  status: TaskStatus;
  result?: TaskResult;
  cost_usd: number;
  created_at: string;
  updated_at: string;
}

/** Matches Go domain/task.CreateRequest */
export interface CreateTaskRequest {
  title: string;
  prompt: string;
}

/** Matches Go domain/project.GitStatus */
export interface GitStatus {
  branch: string;
  commit_hash: string;
  commit_message: string;
  dirty: boolean;
  modified?: string[];
  untracked?: string[];
  ahead: number;
  behind: number;
}

/** Matches Go domain/project.Branch */
export interface Branch {
  name: string;
  current: boolean;
}

/** Agent status enum matching Go domain/agent.Status */
export type AgentStatus = "idle" | "running" | "error" | "stopped";

/** Matches Go domain/agent.Agent */
export interface Agent {
  id: string;
  project_id: string;
  name: string;
  backend: string;
  status: AgentStatus;
  config: Record<string, string>;
  created_at: string;
  updated_at: string;
}

/** Create agent request */
export interface CreateAgentRequest {
  name: string;
  backend: string;
  config?: Record<string, string>;
}

/** LLM Model from LiteLLM */
export interface LLMModel {
  model_name: string;
  litellm_provider?: string;
  model_id?: string;
  model_info?: Record<string, unknown>;
}

/** Add model request for LiteLLM */
export interface AddModelRequest {
  model_name: string;
  litellm_params: Record<string, string>;
  model_info?: Record<string, unknown>;
}

/** Agent event type constants matching Go domain/event.Type */
export type AgentEventType =
  | "agent.started"
  | "agent.step_done"
  | "agent.tool_called"
  | "agent.tool_result"
  | "agent.finished"
  | "agent.error";

/** Matches Go domain/event.AgentEvent */
export interface AgentEvent {
  id: string;
  agent_id: string;
  task_id: string;
  project_id: string;
  type: AgentEventType;
  payload: Record<string, unknown>;
  request_id?: string;
  version: number;
  created_at: string;
}

// --- Run types (Phase 4C) ---

/** Run status enum matching Go domain/run.Status */
export type RunStatus =
  | "pending"
  | "running"
  | "completed"
  | "failed"
  | "cancelled"
  | "timeout"
  | "quality_gate";

/** Deliver mode enum matching Go domain/run.DeliverMode */
export type DeliverMode = "" | "patch" | "commit-local" | "branch" | "pr";

/** Matches Go domain/run.Run */
export interface Run {
  id: string;
  task_id: string;
  agent_id: string;
  project_id: string;
  policy_profile: string;
  exec_mode: string;
  deliver_mode: DeliverMode;
  status: RunStatus;
  step_count: number;
  cost_usd: number;
  error?: string;
  version: number;
  started_at: string;
  completed_at?: string;
  created_at: string;
  updated_at: string;
}

/** Matches Go domain/run.StartRequest */
export interface StartRunRequest {
  task_id: string;
  agent_id: string;
  project_id: string;
  policy_profile?: string;
  exec_mode?: string;
  deliver_mode?: DeliverMode;
}

/** WS event: tool call status */
export interface ToolCallEvent {
  run_id: string;
  call_id: string;
  tool: string;
  decision?: string;
  phase: string;
}

/** WS event: run status change */
export interface RunStatusEvent {
  run_id: string;
  task_id: string;
  project_id: string;
  status: RunStatus;
  step_count: number;
  cost_usd?: number;
}

/** WS event: quality gate status */
export interface QualityGateEvent {
  run_id: string;
  task_id: string;
  project_id: string;
  status: "started" | "passed" | "failed";
  tests_passed?: boolean;
  lint_passed?: boolean;
  error?: string;
}

/** WS event: delivery status */
export interface DeliveryEvent {
  run_id: string;
  task_id: string;
  project_id: string;
  status: "started" | "completed" | "failed";
  mode: string;
  patch_path?: string;
  commit_hash?: string;
  branch_name?: string;
  pr_url?: string;
  error?: string;
}

/** Error response from API */
export interface ApiError {
  error: string;
}

/** Health endpoint response */
export interface HealthStatus {
  status: string;
  postgres: string;
  nats: string;
  litellm: string;
}

/** Provider list response */
export interface ProviderList {
  providers: string[];
}

/** Backend list response */
export interface BackendList {
  backends: string[];
}
