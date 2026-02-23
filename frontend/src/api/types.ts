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
  team_id?: string;
  policy_profile: string;
  exec_mode: string;
  deliver_mode: DeliverMode;
  status: RunStatus;
  step_count: number;
  cost_usd: number;
  tokens_in: number;
  tokens_out: number;
  model: string;
  output?: string;
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
  tokens_in?: number;
  tokens_out?: number;
  model?: string;
}

/** WS event: budget alert */
export interface BudgetAlertEvent {
  run_id: string;
  task_id: string;
  project_id: string;
  cost_usd: number;
  max_cost: number;
  percentage: number;
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

// --- Execution Plan types (Phase 5A) ---

/** Plan protocol enum matching Go domain/plan.Protocol */
export type PlanProtocol = "sequential" | "parallel" | "ping_pong" | "consensus";

/** Plan status enum matching Go domain/plan.Status */
export type PlanStatus = "pending" | "running" | "completed" | "failed" | "cancelled";

/** Plan step status enum matching Go domain/plan.StepStatus */
export type PlanStepStatus =
  | "pending"
  | "running"
  | "completed"
  | "failed"
  | "skipped"
  | "cancelled";

/** Matches Go domain/plan.Step */
export interface PlanStep {
  id: string;
  plan_id: string;
  task_id: string;
  agent_id: string;
  policy_profile: string;
  deliver_mode: string;
  depends_on: string[];
  status: PlanStepStatus;
  run_id: string;
  round: number;
  error: string;
  created_at: string;
  updated_at: string;
}

/** Matches Go domain/plan.ExecutionPlan */
export interface ExecutionPlan {
  id: string;
  project_id: string;
  team_id?: string;
  name: string;
  description: string;
  protocol: PlanProtocol;
  status: PlanStatus;
  max_parallel: number;
  steps: PlanStep[];
  version: number;
  created_at: string;
  updated_at: string;
}

/** Matches Go domain/plan.CreateStepRequest */
export interface CreateStepRequest {
  task_id: string;
  agent_id: string;
  policy_profile?: string;
  deliver_mode?: string;
  depends_on?: string[];
}

/** Matches Go domain/plan.CreatePlanRequest */
export interface CreatePlanRequest {
  name: string;
  description?: string;
  protocol: PlanProtocol;
  max_parallel?: number;
  steps: CreateStepRequest[];
}

/** WS event: plan status change */
export interface PlanStatusEvent {
  plan_id: string;
  project_id: string;
  status: PlanStatus;
}

/** WS event: plan step status change */
export interface PlanStepStatusEvent {
  plan_id: string;
  step_id: string;
  project_id: string;
  status: PlanStepStatus;
  run_id: string;
  error: string;
}

// --- Feature Decomposition types (Phase 5B) ---

/** Orchestrator mode enum matching Go domain/plan.OrchestratorMode */
export type OrchestratorMode = "manual" | "semi_auto" | "full_auto";

/** Agent strategy enum matching Go domain/plan.AgentStrategy */
export type AgentStrategy = "single" | "pair" | "team";

/** Feature decomposition request matching Go domain/plan.DecomposeRequest */
export interface DecomposeRequest {
  feature: string;
  context?: string;
  model?: string;
  auto_start?: boolean;
}

// --- Agent Team types (Phase 5C) ---

/** Team role enum matching Go domain/agent.TeamRole */
export type TeamRole = "coder" | "reviewer" | "tester" | "documenter" | "planner";

/** Team status enum matching Go domain/agent.TeamStatus */
export type TeamStatus = "initializing" | "active" | "completed" | "failed";

/** Matches Go domain/agent.TeamMember */
export interface TeamMember {
  id: string;
  team_id: string;
  agent_id: string;
  role: TeamRole;
}

/** Matches Go domain/agent.Team */
export interface AgentTeam {
  id: string;
  project_id: string;
  name: string;
  protocol: string;
  status: TeamStatus;
  members: TeamMember[];
  version: number;
  created_at: string;
  updated_at: string;
}

/** Create team request matching Go domain/agent.CreateTeamRequest */
export interface CreateTeamRequest {
  name: string;
  protocol: string;
  members: { agent_id: string; role: TeamRole }[];
}

// --- Context-Optimized Planning types (Phase 5C) ---

/** Plan feature request matching Go domain/plan.PlanFeatureRequest */
export interface PlanFeatureRequest {
  feature: string;
  context?: string;
  model?: string;
  auto_start?: boolean;
  auto_team?: boolean;
}

// --- Context types (Phase 5D) ---

/** Context entry kind enum matching Go domain/context.EntryKind */
export type ContextEntryKind =
  | "file"
  | "snippet"
  | "summary"
  | "shared"
  | "repomap"
  | "hybrid"
  | "graph";

/** Matches Go domain/context.ContextEntry */
export interface ContextEntry {
  id: string;
  pack_id: string;
  kind: ContextEntryKind;
  path: string;
  content: string;
  tokens: number;
  priority: number;
}

/** Matches Go domain/context.ContextPack */
export interface ContextPack {
  id: string;
  task_id: string;
  project_id: string;
  token_budget: number;
  tokens_used: number;
  entries: ContextEntry[];
  created_at: string;
}

/** Matches Go domain/context.SharedContextItem */
export interface SharedContextItem {
  id: string;
  shared_id: string;
  key: string;
  value: string;
  author: string;
  tokens: number;
  created_at: string;
}

/** Matches Go domain/context.SharedContext */
export interface SharedContext {
  id: string;
  team_id: string;
  project_id: string;
  version: number;
  items: SharedContextItem[];
  created_at: string;
  updated_at: string;
}

/** Matches Go domain/context.AddSharedItemRequest */
export interface AddSharedItemRequest {
  key: string;
  value: string;
  author: string;
}

// --- Mode types (Phase 5E) ---

/** Matches Go domain/mode.Mode */
export interface Mode {
  id: string;
  name: string;
  description: string;
  builtin: boolean;
  tools: string[];
  denied_tools: string[];
  denied_actions: string[];
  required_artifact: string;
  llm_scenario: string;
  autonomy: number;
  prompt_prefix: string;
}

/** Create mode request */
export interface CreateModeRequest {
  id: string;
  name: string;
  description?: string;
  tools?: string[];
  denied_tools?: string[];
  denied_actions?: string[];
  required_artifact?: string;
  llm_scenario?: string;
  autonomy: number;
  prompt_prefix?: string;
}

// --- WS events (Phase 5E) ---

/** WS event: team status change */
export interface TeamStatusEvent {
  team_id: string;
  project_id: string;
  status: TeamStatus;
  name: string;
}

/** WS event: shared context update */
export interface SharedContextUpdateEvent {
  team_id: string;
  key: string;
  author: string;
  version: number;
}

// --- RepoMap types (Phase 6A) ---

/** Matches Go domain/context.RepoMap */
export interface RepoMap {
  id: string;
  project_id: string;
  map_text: string;
  token_count: number;
  file_count: number;
  symbol_count: number;
  languages: string[];
  version: number;
  created_at: string;
  updated_at: string;
}

/** WS event: repo map status change */
export interface RepoMapStatusEvent {
  project_id: string;
  status: "generating" | "ready" | "failed";
  token_count?: number;
  file_count?: number;
  symbol_count?: number;
  error?: string;
}

// --- Retrieval types (Phase 6B) ---

/** Search hit from hybrid retrieval */
export interface RetrievalSearchHit {
  filepath: string;
  start_line: number;
  end_line: number;
  content: string;
  language: string;
  symbol_name?: string;
  score: number;
  bm25_rank: number;
  semantic_rank: number;
}

/** Search result payload */
export interface RetrievalSearchResult {
  project_id: string;
  query: string;
  request_id: string;
  results: RetrievalSearchHit[];
  error?: string;
}

/** Index status */
export interface RetrievalIndexStatus {
  project_id: string;
  status: "building" | "ready" | "error";
  file_count: number;
  chunk_count: number;
  embedding_model: string;
  error?: string;
}

/** Search request body */
export interface SearchRequest {
  query: string;
  top_k?: number;
  bm25_weight?: number;
  semantic_weight?: number;
}

// --- Retrieval Sub-Agent types (Phase 6C) ---

/** Sub-agent search request body */
export interface SubAgentSearchRequest {
  query: string;
  top_k?: number;
  max_queries?: number;
  model?: string;
  rerank?: boolean;
}

/** Sub-agent search result */
export interface SubAgentSearchResult {
  project_id: string;
  query: string;
  request_id: string;
  results: RetrievalSearchHit[];
  expanded_queries: string[];
  total_candidates: number;
  error?: string;
}

/** WS event: retrieval status change */
export interface RetrievalStatusEvent {
  project_id: string;
  status: "building" | "ready" | "error";
  file_count?: number;
  chunk_count?: number;
  embedding_model?: string;
  error?: string;
}

// --- GraphRAG types (Phase 6D) ---

/** Graph node kind */
export type GraphNodeKind = "function" | "class" | "method" | "module";

/** Graph build status */
export interface GraphStatus {
  project_id: string;
  status: "pending" | "building" | "ready" | "error";
  node_count: number;
  edge_count: number;
  languages: string[];
  error?: string;
  built_at?: string;
}

/** Graph search hit */
export interface GraphSearchHit {
  filepath: string;
  symbol_name: string;
  kind: GraphNodeKind;
  start_line: number;
  end_line: number;
  distance: number;
  score: number;
  edge_path: string[];
}

/** Graph search request body */
export interface GraphSearchRequest {
  seed_symbols: string[];
  max_hops?: number;
  top_k?: number;
}

/** Graph search result */
export interface GraphSearchResult {
  project_id: string;
  request_id: string;
  results: GraphSearchHit[];
  error?: string;
}

/** WS event: graph status change */
export interface GraphStatusEvent {
  project_id: string;
  status: "building" | "ready" | "error";
  node_count?: number;
  edge_count?: number;
  languages?: string[];
  error?: string;
}

// --- Policy types (Phase 4A) ---

/** Policy decision enum matching Go domain/policy.Decision */
export type PolicyDecision = "allow" | "deny" | "ask";

/** Permission mode enum matching Go domain/policy.PermissionMode */
export type PermissionMode = "default" | "acceptEdits" | "plan" | "delegate";

/** Matches Go domain/policy.ToolSpecifier */
export interface ToolSpecifier {
  tool: string;
  sub_pattern?: string;
}

/** Matches Go domain/policy.PermissionRule */
export interface PermissionRule {
  specifier: ToolSpecifier;
  decision: PolicyDecision;
  path_allow?: string[];
  path_deny?: string[];
  command_allow?: string[];
  command_deny?: string[];
}

/** Matches Go domain/policy.QualityGate */
export interface PolicyQualityGate {
  require_tests_pass: boolean;
  require_lint_pass: boolean;
  rollback_on_gate_fail: boolean;
}

/** Matches Go domain/policy.TerminationCondition */
export interface TerminationCondition {
  max_steps?: number;
  timeout_seconds?: number;
  max_cost?: number;
  stall_detection?: boolean;
  stall_threshold?: number;
}

/** Matches Go domain/resource.Limits */
export interface ResourceLimits {
  memory_mb?: number;
  cpu_quota?: number;
  pids_limit?: number;
  storage_gb?: number;
  network_mode?: string;
}

/** Matches Go domain/policy.PolicyProfile */
export interface PolicyProfile {
  name: string;
  description?: string;
  mode: PermissionMode;
  rules: PermissionRule[];
  quality_gate: PolicyQualityGate;
  termination: TerminationCondition;
  resource_limits?: ResourceLimits;
}

/** Matches Go domain/policy.ToolCall */
export interface PolicyToolCall {
  tool: string;
  command?: string;
  path?: string;
}

// --- Cost Aggregation types (Phase 7) ---

/** Matches Go domain/cost.Summary */
export interface CostSummary {
  total_cost_usd: number;
  total_tokens_in: number;
  total_tokens_out: number;
  run_count: number;
}

/** Matches Go domain/cost.ProjectSummary */
export interface ProjectCostSummary {
  project_id: string;
  project_name: string;
  total_cost_usd: number;
  total_tokens_in: number;
  total_tokens_out: number;
  run_count: number;
}

/** Matches Go domain/cost.ModelSummary */
export interface ModelCostSummary {
  model: string;
  total_cost_usd: number;
  total_tokens_in: number;
  total_tokens_out: number;
  run_count: number;
}

/** Matches Go domain/cost.DailyCost */
export interface DailyCost {
  date: string;
  cost_usd: number;
  tokens_in: number;
  tokens_out: number;
  run_count: number;
}

/** Matches Go domain/cost.ToolSummary */
export interface ToolCostSummary {
  tool: string;
  model: string;
  cost_usd: number;
  tokens_in: number;
  tokens_out: number;
  call_count: number;
}

// --- Roadmap types (Phase 8) ---

/** Roadmap status enum matching Go domain/roadmap.RoadmapStatus */
export type RoadmapStatus = "draft" | "active" | "complete" | "archived";

/** Feature status enum matching Go domain/roadmap.FeatureStatus */
export type FeatureStatus = "backlog" | "planned" | "in_progress" | "done" | "cancelled";

/** Matches Go domain/roadmap.Feature */
export interface RoadmapFeature {
  id: string;
  milestone_id: string;
  roadmap_id: string;
  title: string;
  description: string;
  status: FeatureStatus;
  sort_order: number;
  labels: string[];
  spec_ref: string;
  external_ids: Record<string, string>;
  version: number;
  created_at: string;
  updated_at: string;
}

/** Matches Go domain/roadmap.Milestone */
export interface Milestone {
  id: string;
  roadmap_id: string;
  title: string;
  description: string;
  status: RoadmapStatus;
  sort_order: number;
  features: RoadmapFeature[];
  version: number;
  created_at: string;
  updated_at: string;
}

/** Matches Go domain/roadmap.Roadmap */
export interface Roadmap {
  id: string;
  project_id: string;
  title: string;
  description: string;
  status: RoadmapStatus;
  milestones: Milestone[];
  version: number;
  created_at: string;
  updated_at: string;
}

/** Create roadmap request */
export interface CreateRoadmapRequest {
  title: string;
  description?: string;
}

/** Create milestone request */
export interface CreateMilestoneRequest {
  title: string;
  description?: string;
  sort_order?: number;
}

/** Create feature request */
export interface CreateFeatureRequest {
  title: string;
  description?: string;
  sort_order?: number;
  labels?: string[];
}

/** Detection result from auto-detect */
export interface DetectionResult {
  found: boolean;
  format: string;
  path: string;
  file_markers: string[];
}

/** AI roadmap view */
export interface AIRoadmapView {
  project_id: string;
  format: string;
  content: string;
  generated_at: string;
}

/** Import result from spec or PM import */
export interface ImportResult {
  source: string;
  milestones_created: number;
  features_created: number;
  errors?: string[];
}

/** PM import request body */
export interface PMImportRequest {
  provider: string;
  project_ref: string;
}

/** Spec/PM provider info */
export interface ProviderInfo {
  name: string;
  capabilities: Record<string, boolean>;
}

/** WS event: roadmap status change */
export interface RoadmapStatusEvent {
  roadmap_id: string;
  project_id: string;
  status: string;
  title: string;
}

// --- Trajectory types (Phase 8) ---

/** Trajectory page response */
export interface TrajectoryPage {
  events: AgentEvent[];
  cursor: string;
  has_more: boolean;
  total: number;
  stats: TrajectorySummary;
}

/** Trajectory summary stats */
export interface TrajectorySummary {
  total_events: number;
  event_counts: Record<string, number>;
  duration_ms: number;
  tool_call_count: number;
  error_count: number;
  total_tokens_in: number;
  total_tokens_out: number;
  total_cost_usd: number;
}

// --- Auth types (Phase 10C) ---

/** User role enum matching Go domain/user.Role */
export type UserRole = "admin" | "editor" | "viewer";

/** Matches Go domain/user.User */
export interface User {
  id: string;
  email: string;
  name: string;
  role: UserRole;
  tenant_id: string;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

/** Matches Go domain/user.LoginRequest */
export interface LoginRequest {
  email: string;
  password: string;
}

/** Matches Go domain/user.LoginResponse */
export interface LoginResponse {
  access_token: string;
  expires_in: number;
  user: User;
}

/** Matches Go domain/user.CreateRequest */
export interface CreateUserRequest {
  email: string;
  name: string;
  password: string;
  role: UserRole;
}

/** Matches Go domain/user.UpdateRequest */
export interface UpdateUserRequest {
  name?: string;
  role?: UserRole;
  enabled?: boolean;
}

/** Matches Go domain/user.APIKey (without key_hash) */
export interface APIKeyInfo {
  id: string;
  user_id: string;
  name: string;
  prefix: string;
  expires_at?: string;
  created_at: string;
}

/** Matches Go domain/user.CreateAPIKeyRequest */
export interface CreateAPIKeyRequest {
  name: string;
  expires_in?: number;
}

/** Matches Go domain/user.CreateAPIKeyResponse */
export interface CreateAPIKeyResponse {
  api_key: APIKeyInfo;
  plain_key: string;
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

// --- AG-UI Protocol Types ---

export interface AGUIRunStartedEvent {
  run_id: string;
  thread_id?: string;
  agent_name?: string;
}

export interface AGUIRunFinishedEvent {
  run_id: string;
  status: "completed" | "failed" | "cancelled";
}

export interface AGUITextMessageEvent {
  run_id: string;
  role: string;
  content: string;
}

export interface AGUIToolCallEvent {
  run_id: string;
  call_id: string;
  name: string;
  args: string;
}

export interface AGUIToolResultEvent {
  run_id: string;
  call_id: string;
  result: string;
  error?: string;
}

export interface AGUIStateDeltaEvent {
  run_id: string;
  delta: string;
}

export interface AGUIStepStartedEvent {
  run_id: string;
  step_id: string;
  name: string;
}

export interface AGUIStepFinishedEvent {
  run_id: string;
  step_id: string;
  status: "completed" | "failed";
}

// Phase 12I: Review Policies & Reviews

export interface ReviewPolicy {
  id: string;
  project_id: string;
  tenant_id: string;
  name: string;
  trigger_type: "commit_count" | "pre_merge" | "cron";
  commit_threshold: number;
  cron_expr: string;
  branch_pattern: string;
  template_id: string;
  enabled: boolean;
  commit_counter: number;
  created_at: string;
  updated_at: string;
}

export interface Review {
  id: string;
  policy_id: string;
  project_id: string;
  tenant_id: string;
  plan_id: string;
  status: "pending" | "running" | "completed" | "failed";
  trigger_ref: string;
  created_at: string;
  completed_at?: string;
}

// Phase 12J: Stack Detection

/** Matches Go domain/project.Language */
export interface DetectedLanguage {
  name: string;
  confidence: number;
  manifests: string[];
  frameworks: string[];
}

/** Matches Go domain/project.ToolRecommendation */
export interface ToolRecommendation {
  category: string;
  id: string;
  name: string;
  reason: string;
}

/** Matches Go domain/project.StackDetectionResult */
export interface StackDetectionResult {
  languages: DetectedLanguage[];
  recommendations: ToolRecommendation[];
  scanned_path: string;
}

// Phase 12K: Knowledge Bases

/** Matches Go domain/knowledgebase.KnowledgeBase */
export interface KnowledgeBase {
  id: string;
  name: string;
  description: string;
  category: string;
  tags: string[];
  builtin: boolean;
  content_path: string;
  status: string;
  chunk_count: number;
  created_at: string;
  updated_at: string;
}

/** Matches Go domain/knowledgebase.CreateRequest */
export interface CreateKnowledgeBaseRequest {
  name: string;
  description: string;
  category: string;
  tags: string[];
  content_path: string;
}
