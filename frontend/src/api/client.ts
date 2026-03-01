import { getCached, processQueue, queueAction, setCached } from "./cache";
import type {
  AddModelRequest,
  AddSharedItemRequest,
  Agent,
  AgentEvent,
  AgentTeam,
  AIRoadmapView,
  ApiError,
  APIKeyInfo,
  BackendList,
  Branch,
  ContextPack,
  CostSummary,
  CreateAgentRequest,
  CreateAPIKeyRequest,
  CreateAPIKeyResponse,
  CreateFeatureRequest,
  CreateMCPServerRequest,
  CreateMilestoneRequest,
  CreateModeRequest,
  CreatePlanRequest,
  CreateProjectRequest,
  CreateRoadmapRequest,
  CreateTaskRequest,
  CreateTeamRequest,
  CreateUserRequest,
  DailyCost,
  DecomposeRequest,
  DetectionResult,
  EvaluationResult,
  ExecutionPlan,
  ForgotPasswordRequest,
  GitStatus,
  GraphSearchRequest,
  GraphSearchResult,
  GraphStatus,
  HealthStatus,
  ImportResult,
  InitialSetupRequest,
  LLMModel,
  LoginRequest,
  LoginResponse,
  LSPDiagnostic,
  LSPDocumentSymbol,
  LSPHoverResult,
  LSPLocation,
  LSPServerInfo,
  MCPServer,
  MCPServerTool,
  MCPTestResult,
  Milestone,
  Mode,
  ModelCostSummary,
  PlanFeatureRequest,
  PlanGraph,
  PMImportRequest,
  PolicyProfile,
  PolicyToolCall,
  Project,
  ProjectCostSummary,
  ProviderInfo,
  ProviderList,
  RepoMap,
  ResetPasswordRequest,
  RetrievalIndexStatus,
  RetrievalSearchResult,
  ReviewDecision,
  Roadmap,
  RoadmapFeature,
  Run,
  SearchRequest,
  SetupStatusResponse,
  SharedContext,
  SharedContextItem,
  StartRunRequest,
  SubAgentSearchRequest,
  SubAgentSearchResult,
  Task,
  ToolCostSummary,
  TrajectoryPage,
  UpdateUserRequest,
  User,
} from "./types";

const BASE = "/api/v1";

import { MAX_RETRIES, RETRY_BASE_MS, RETRYABLE_STATUSES } from "~/config/constants";

import { url } from "./factory";

// Access token getter — set by AuthProvider to inject JWT into requests.
let accessTokenGetter: (() => string | null) | null = null;

/** Set the function that provides the current access token. */
export function setAccessTokenGetter(fn: () => string | null): void {
  accessTokenGetter = fn;
}

/** Return the current access token (used by WebSocket to append ?token=). */
export function getAccessToken(): string | null {
  return accessTokenGetter?.() ?? null;
}

class FetchError extends Error {
  constructor(
    public readonly status: number,
    public readonly body: ApiError,
  ) {
    super(body.error);
    this.name = "FetchError";
  }
}

function isRetryable(status: number, method: string): boolean {
  // Only retry on server errors for idempotent methods
  if (method === "POST") return false;
  return RETRYABLE_STATUSES.has(status);
}

function isOffline(): boolean {
  return !navigator.onLine;
}

async function executeRequest<T>(path: string, init?: RequestInit): Promise<T> {
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    ...(init?.headers as Record<string, string>),
  };

  // Inject access token if available.
  const token = accessTokenGetter?.();
  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }

  const res = await fetch(`${BASE}${path}`, {
    ...init,
    headers,
    credentials: "include", // send httpOnly refresh cookie
  });

  if (!res.ok) {
    let body: ApiError;
    try {
      body = (await res.json()) as ApiError;
    } catch {
      body = { error: `HTTP ${res.status} ${res.statusText || "Error"}` };
    }
    throw new FetchError(res.status, body);
  }

  // 204 No Content
  if (res.status === 204) {
    return undefined as T;
  }

  return res.json() as Promise<T>;
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const method = init?.method ?? "GET";
  let lastError: unknown;

  for (let attempt = 0; attempt <= MAX_RETRIES; attempt++) {
    try {
      const result = await executeRequest<T>(path, init);

      // Cache successful GET responses
      if (method === "GET") {
        setCached(path, result);
      }

      return result;
    } catch (err) {
      if (err instanceof FetchError) {
        if (attempt < MAX_RETRIES && isRetryable(err.status, method)) {
          lastError = err;
          await new Promise((r) => setTimeout(r, RETRY_BASE_MS * 2 ** attempt));
          continue;
        }
        throw err;
      }

      // Network errors (fetch throws TypeError on network failure)
      if (err instanceof TypeError) {
        if (attempt < MAX_RETRIES) {
          lastError = err;
          await new Promise((r) => setTimeout(r, RETRY_BASE_MS * 2 ** attempt));
          continue;
        }

        // All retries exhausted — fall back to cache for GETs
        if (method === "GET") {
          const cached = getCached<T>(path);
          if (cached !== undefined) return cached;
        }

        // Queue mutations for later retry when back online
        if (method !== "GET" && isOffline() && init) {
          return queueAction(path, init) as Promise<T>;
        }

        throw err;
      }

      throw err;
    }
  }

  throw lastError;
}

// Process queued actions when coming back online
if (typeof window !== "undefined") {
  window.addEventListener("online", () => {
    void processQueue((path, init) => executeRequest(path, init));
  });
}

export const api = {
  health: {
    check: () => fetch("/health").then((r) => r.json() as Promise<HealthStatus>),
  },

  projects: {
    list: () => request<Project[]>("/projects"),

    get: (id: string) => request<Project>(url`/projects/${id}`),

    create: (data: CreateProjectRequest) =>
      request<Project>("/projects", {
        method: "POST",
        body: JSON.stringify(data),
      }),

    update: (id: string, data: import("./types").UpdateProjectRequest) =>
      request<Project>(url`/projects/${id}`, {
        method: "PUT",
        body: JSON.stringify(data),
      }),

    delete: (id: string) =>
      request<undefined>(url`/projects/${id}`, {
        method: "DELETE",
      }),

    parseRepoURL: (repoUrl: string) =>
      request<import("./types").ParsedRepoURL>("/parse-repo-url", {
        method: "POST",
        body: JSON.stringify({ url: repoUrl }),
      }),

    clone: (id: string) =>
      request<Project>(url`/projects/${id}/clone`, {
        method: "POST",
      }),

    gitStatus: (id: string) => request<GitStatus>(url`/projects/${id}/git/status`),

    pull: (id: string) =>
      request<{ status: string }>(url`/projects/${id}/git/pull`, {
        method: "POST",
      }),

    branches: (id: string) => request<Branch[]>(url`/projects/${id}/git/branches`),

    checkout: (id: string, branch: string) =>
      request<{ status: string; branch: string }>(url`/projects/${id}/git/checkout`, {
        method: "POST",
        body: JSON.stringify({ branch }),
      }),

    detectStack: (id: string) =>
      request<import("./types").StackDetectionResult>(url`/projects/${id}/detect-stack`),

    detectStackByPath: (path: string) =>
      request<import("./types").StackDetectionResult>("/detect-stack", {
        method: "POST",
        body: JSON.stringify({ path }),
      }),

    setup: (id: string, branch?: string) =>
      request<import("./types").SetupResult>(url`/projects/${id}/setup`, {
        method: "POST",
        body: JSON.stringify(branch ? { branch } : {}),
      }),

    adopt: (id: string, body: { path: string }) =>
      request<Project>(url`/projects/${id}/adopt`, {
        method: "POST",
        body: JSON.stringify(body),
      }),

    remoteBranches: (repoUrl: string) =>
      request<{ branches: string[] }>(url`/projects/remote-branches?url=${repoUrl}`).then(
        (r) => r.branches,
      ),
  },

  agents: {
    list: (projectId: string) => request<Agent[]>(url`/projects/${projectId}/agents`),

    get: (id: string) => request<Agent>(url`/agents/${id}`),

    create: (projectId: string, data: CreateAgentRequest) =>
      request<Agent>(url`/projects/${projectId}/agents`, {
        method: "POST",
        body: JSON.stringify(data),
      }),

    delete: (id: string) => request<undefined>(url`/agents/${id}`, { method: "DELETE" }),

    dispatch: (agentId: string, taskId: string) =>
      request<{ status: string }>(url`/agents/${agentId}/dispatch`, {
        method: "POST",
        body: JSON.stringify({ task_id: taskId }),
      }),

    stop: (agentId: string, taskId: string) =>
      request<{ status: string }>(url`/agents/${agentId}/stop`, {
        method: "POST",
        body: JSON.stringify({ task_id: taskId }),
      }),
  },

  tasks: {
    list: (projectId: string) => request<Task[]>(url`/projects/${projectId}/tasks`),

    get: (id: string) => request<Task>(url`/tasks/${id}`),

    create: (projectId: string, data: CreateTaskRequest) =>
      request<Task>(url`/projects/${projectId}/tasks`, {
        method: "POST",
        body: JSON.stringify(data),
      }),

    events: (taskId: string) => request<AgentEvent[]>(url`/tasks/${taskId}/events`),

    context: (taskId: string) => request<ContextPack>(url`/tasks/${taskId}/context`),

    buildContext: (taskId: string, projectId: string, teamId?: string) =>
      request<ContextPack>(url`/tasks/${taskId}/context`, {
        method: "POST",
        body: JSON.stringify({ project_id: projectId, team_id: teamId ?? "" }),
      }),
  },

  llm: {
    models: () => request<LLMModel[]>("/llm/models"),

    addModel: (data: AddModelRequest) =>
      request<undefined>("/llm/models", {
        method: "POST",
        body: JSON.stringify(data),
      }),

    deleteModel: (modelId: string) =>
      request<undefined>(url`/llm/models/${modelId}`, {
        method: "DELETE",
      }),

    health: () => request<{ status: string }>("/llm/health"),

    discover: () => request<import("./types").DiscoverModelsResponse>("/llm/discover"),
  },

  runs: {
    start: (data: StartRunRequest) =>
      request<Run>("/runs", {
        method: "POST",
        body: JSON.stringify(data),
      }),

    get: (id: string) => request<Run>(url`/runs/${id}`),

    cancel: (id: string) =>
      request<{ status: string }>(url`/runs/${id}/cancel`, {
        method: "POST",
      }),

    listByTask: (taskId: string) => request<Run[]>(url`/tasks/${taskId}/runs`),
  },

  teams: {
    list: (projectId: string) => request<AgentTeam[]>(url`/projects/${projectId}/teams`),

    get: (id: string) => request<AgentTeam>(url`/teams/${id}`),

    create: (projectId: string, data: CreateTeamRequest) =>
      request<AgentTeam>(url`/projects/${projectId}/teams`, {
        method: "POST",
        body: JSON.stringify(data),
      }),

    delete: (id: string) => request<undefined>(url`/teams/${id}`, { method: "DELETE" }),

    sharedContext: (teamId: string) => request<SharedContext>(url`/teams/${teamId}/shared-context`),

    addSharedItem: (teamId: string, data: AddSharedItemRequest) =>
      request<SharedContextItem>(url`/teams/${teamId}/shared-context`, {
        method: "POST",
        body: JSON.stringify(data),
      }),
  },

  plans: {
    decompose: (projectId: string, data: DecomposeRequest) =>
      request<ExecutionPlan>(url`/projects/${projectId}/decompose`, {
        method: "POST",
        body: JSON.stringify(data),
      }),

    planFeature: (projectId: string, data: PlanFeatureRequest) =>
      request<ExecutionPlan>(url`/projects/${projectId}/plan-feature`, {
        method: "POST",
        body: JSON.stringify(data),
      }),

    list: (projectId: string) => request<ExecutionPlan[]>(url`/projects/${projectId}/plans`),

    get: (id: string) => request<ExecutionPlan>(url`/plans/${id}`),

    create: (projectId: string, data: CreatePlanRequest) =>
      request<ExecutionPlan>(url`/projects/${projectId}/plans`, {
        method: "POST",
        body: JSON.stringify(data),
      }),

    start: (id: string) =>
      request<ExecutionPlan>(url`/plans/${id}/start`, {
        method: "POST",
      }),

    cancel: (id: string) =>
      request<{ status: string }>(url`/plans/${id}/cancel`, {
        method: "POST",
      }),

    graph: (id: string) => request<PlanGraph>(url`/plans/${id}/graph`),

    evaluateStep: (planId: string, stepId: string) =>
      request<ReviewDecision>(url`/plans/${planId}/steps/${stepId}/evaluate`, { method: "POST" }),
  },

  modes: {
    list: () => request<Mode[]>("/modes"),

    get: (id: string) => request<Mode>(url`/modes/${id}`),

    create: (data: CreateModeRequest) =>
      request<Mode>("/modes", {
        method: "POST",
        body: JSON.stringify(data),
      }),

    update: (id: string, data: CreateModeRequest) =>
      request<Mode>(url`/modes/${id}`, {
        method: "PUT",
        body: JSON.stringify(data),
      }),

    delete: (id: string) => request<undefined>(url`/modes/${id}`, { method: "DELETE" }),

    scenarios: () => request<string[]>("/modes/scenarios"),
  },

  repomap: {
    get: (projectId: string) => request<RepoMap>(url`/projects/${projectId}/repomap`),

    generate: (projectId: string, activeFiles?: string[]) =>
      request<{ status: string }>(url`/projects/${projectId}/repomap`, {
        method: "POST",
        body: JSON.stringify({ active_files: activeFiles ?? [] }),
      }),
  },

  retrieval: {
    indexStatus: (projectId: string) =>
      request<RetrievalIndexStatus>(url`/projects/${projectId}/index`),

    buildIndex: (projectId: string, embeddingModel?: string) =>
      request<{ status: string }>(url`/projects/${projectId}/index`, {
        method: "POST",
        body: JSON.stringify({ embedding_model: embeddingModel ?? "" }),
      }),

    search: (projectId: string, data: SearchRequest) =>
      request<RetrievalSearchResult>(url`/projects/${projectId}/search`, {
        method: "POST",
        body: JSON.stringify(data),
      }),

    agentSearch: (projectId: string, data: SubAgentSearchRequest) =>
      request<SubAgentSearchResult>(url`/projects/${projectId}/search/agent`, {
        method: "POST",
        body: JSON.stringify(data),
      }),
  },

  graph: {
    status: (projectId: string) => request<GraphStatus>(url`/projects/${projectId}/graph/status`),

    build: (projectId: string) =>
      request<{ status: string }>(url`/projects/${projectId}/graph/build`, {
        method: "POST",
      }),

    search: (projectId: string, data: GraphSearchRequest) =>
      request<GraphSearchResult>(url`/projects/${projectId}/graph/search`, {
        method: "POST",
        body: JSON.stringify(data),
      }),
  },

  policies: {
    list: () => request<{ profiles: string[] }>("/policies"),

    get: (name: string) => request<PolicyProfile>(url`/policies/${name}`),

    create: (profile: PolicyProfile) =>
      request<PolicyProfile>("/policies", {
        method: "POST",
        body: JSON.stringify(profile),
      }),

    delete: (name: string) =>
      request<undefined>(url`/policies/${name}`, {
        method: "DELETE",
      }),

    evaluate: (name: string, call: PolicyToolCall) =>
      request<EvaluationResult>(url`/policies/${name}/evaluate`, {
        method: "POST",
        body: JSON.stringify(call),
      }),
  },

  costs: {
    global: () => request<ProjectCostSummary[]>("/costs"),

    project: (id: string) => request<CostSummary>(url`/projects/${id}/costs`),

    byModel: (id: string) => request<ModelCostSummary[]>(url`/projects/${id}/costs/by-model`),

    daily: (id: string, days = 30) =>
      request<DailyCost[]>(url`/projects/${id}/costs/daily?days=${days}`),

    recentRuns: (id: string, limit = 20) =>
      request<Run[]>(url`/projects/${id}/costs/runs?limit=${limit}`),

    byTool: (id: string) => request<ToolCostSummary[]>(url`/projects/${id}/costs/by-tool`),

    byToolForRun: (runId: string) => request<ToolCostSummary[]>(url`/runs/${runId}/costs/by-tool`),
  },

  roadmap: {
    get: (projectId: string) => request<Roadmap>(url`/projects/${projectId}/roadmap`),

    create: (projectId: string, data: CreateRoadmapRequest) =>
      request<Roadmap>(url`/projects/${projectId}/roadmap`, {
        method: "POST",
        body: JSON.stringify(data),
      }),

    update: (projectId: string, data: Partial<Roadmap> & { version: number }) =>
      request<Roadmap>(url`/projects/${projectId}/roadmap`, {
        method: "PUT",
        body: JSON.stringify(data),
      }),

    delete: (projectId: string) =>
      request<undefined>(url`/projects/${projectId}/roadmap`, {
        method: "DELETE",
      }),

    ai: (projectId: string, format: "json" | "yaml" | "markdown" = "markdown") =>
      request<AIRoadmapView>(url`/projects/${projectId}/roadmap/ai?format=${format}`),

    detect: (projectId: string) =>
      request<DetectionResult>(url`/projects/${projectId}/roadmap/detect`, {
        method: "POST",
      }),

    createMilestone: (projectId: string, data: CreateMilestoneRequest) =>
      request<Milestone>(url`/projects/${projectId}/roadmap/milestones`, {
        method: "POST",
        body: JSON.stringify(data),
      }),

    updateMilestone: (id: string, data: Partial<Milestone> & { version: number }) =>
      request<Milestone>(url`/milestones/${id}`, {
        method: "PUT",
        body: JSON.stringify(data),
      }),

    deleteMilestone: (id: string) =>
      request<undefined>(url`/milestones/${id}`, { method: "DELETE" }),

    createFeature: (milestoneId: string, data: CreateFeatureRequest) =>
      request<RoadmapFeature>(url`/milestones/${milestoneId}/features`, {
        method: "POST",
        body: JSON.stringify(data),
      }),

    updateFeature: (id: string, data: Partial<RoadmapFeature> & { version: number }) =>
      request<RoadmapFeature>(url`/features/${id}`, {
        method: "PUT",
        body: JSON.stringify(data),
      }),

    deleteFeature: (id: string) => request<undefined>(url`/features/${id}`, { method: "DELETE" }),

    importSpecs: (projectId: string) =>
      request<ImportResult>(url`/projects/${projectId}/roadmap/import`, {
        method: "POST",
      }),

    importPMItems: (projectId: string, data: PMImportRequest) =>
      request<ImportResult>(url`/projects/${projectId}/roadmap/import/pm`, {
        method: "POST",
        body: JSON.stringify(data),
      }),

    syncToFile: (projectId: string) =>
      request<{ status: string }>(url`/projects/${projectId}/roadmap/sync-to-file`, {
        method: "POST",
      }),
  },

  trajectory: {
    get: (runId: string, opts?: { types?: string; cursor?: string; limit?: number }) => {
      const params = new URLSearchParams();
      if (opts?.types) params.set("types", opts.types);
      if (opts?.cursor) params.set("cursor", opts.cursor);
      if (opts?.limit) params.set("limit", String(opts.limit));
      const qs = params.toString();
      return request<TrajectoryPage>(
        `/runs/${encodeURIComponent(runId)}/trajectory${qs ? `?${qs}` : ""}`,
      );
    },

    exportUrl: (runId: string) =>
      `${BASE}/runs/${encodeURIComponent(runId)}/trajectory/export?format=json`,
  },

  providers: {
    git: () => request<ProviderList>("/providers/git"),
    agent: () => request<BackendList>("/providers/agent"),
    spec: () => request<ProviderInfo[]>("/providers/spec"),
    pm: () => request<ProviderInfo[]>("/providers/pm"),
  },
  auth: {
    login: (data: LoginRequest) =>
      request<LoginResponse>("/auth/login", {
        method: "POST",
        body: JSON.stringify(data),
      }),

    refresh: () =>
      request<LoginResponse>("/auth/refresh", {
        method: "POST",
      }),

    logout: () =>
      request<{ status: string }>("/auth/logout", {
        method: "POST",
      }),

    me: () => request<User>("/auth/me"),

    changePassword: (data: import("./types").ChangePasswordRequest) =>
      request<{ status: string }>("/auth/change-password", {
        method: "POST",
        body: JSON.stringify(data),
      }),

    setupStatus: () => request<SetupStatusResponse>("/auth/setup-status"),

    setup: (data: InitialSetupRequest) =>
      request<LoginResponse>("/auth/setup", {
        method: "POST",
        body: JSON.stringify(data),
      }),

    forgotPassword: (data: ForgotPasswordRequest) =>
      request<{ status: string }>("/auth/forgot-password", {
        method: "POST",
        body: JSON.stringify(data),
      }),

    resetPassword: (data: ResetPasswordRequest) =>
      request<{ status: string }>("/auth/reset-password", {
        method: "POST",
        body: JSON.stringify(data),
      }),

    createAPIKey: (data: CreateAPIKeyRequest) =>
      request<CreateAPIKeyResponse>("/auth/api-keys", {
        method: "POST",
        body: JSON.stringify(data),
      }),

    listAPIKeys: () => request<APIKeyInfo[]>("/auth/api-keys"),

    deleteAPIKey: (id: string) =>
      request<undefined>(url`/auth/api-keys/${id}`, {
        method: "DELETE",
      }),
  },

  users: {
    list: () => request<User[]>("/users"),

    create: (data: CreateUserRequest) =>
      request<User>("/users", {
        method: "POST",
        body: JSON.stringify(data),
      }),

    update: (id: string, data: UpdateUserRequest) =>
      request<User>(url`/users/${id}`, {
        method: "PUT",
        body: JSON.stringify(data),
      }),

    delete: (id: string) =>
      request<undefined>(url`/users/${id}`, {
        method: "DELETE",
      }),
  },

  reviews: {
    listPolicies: (projectId: string) =>
      request<import("./types").ReviewPolicy[]>(url`/projects/${projectId}/review-policies`),

    createPolicy: (projectId: string, data: Partial<import("./types").ReviewPolicy>) =>
      request<import("./types").ReviewPolicy>(url`/projects/${projectId}/review-policies`, {
        method: "POST",
        body: JSON.stringify(data),
      }),

    getPolicy: (id: string) => request<import("./types").ReviewPolicy>(url`/review-policies/${id}`),

    updatePolicy: (id: string, data: Partial<import("./types").ReviewPolicy>) =>
      request<import("./types").ReviewPolicy>(url`/review-policies/${id}`, {
        method: "PUT",
        body: JSON.stringify(data),
      }),

    deletePolicy: (id: string) =>
      request<undefined>(url`/review-policies/${id}`, { method: "DELETE" }),

    trigger: (policyId: string) =>
      request<import("./types").Review>(url`/review-policies/${policyId}/trigger`, {
        method: "POST",
      }),

    list: (projectId: string) =>
      request<import("./types").Review[]>(url`/projects/${projectId}/reviews`),

    get: (id: string) => request<import("./types").Review>(url`/reviews/${id}`),
  },

  scopes: {
    list: () => request<import("./types").RetrievalScope[]>("/scopes"),

    get: (id: string) => request<import("./types").RetrievalScope>(url`/scopes/${id}`),

    create: (data: import("./types").CreateScopeRequest) =>
      request<import("./types").RetrievalScope>("/scopes", {
        method: "POST",
        body: JSON.stringify(data),
      }),

    update: (id: string, data: import("./types").UpdateScopeRequest) =>
      request<import("./types").RetrievalScope>(url`/scopes/${id}`, {
        method: "PUT",
        body: JSON.stringify(data),
      }),

    delete: (id: string) => request<undefined>(url`/scopes/${id}`, { method: "DELETE" }),

    addProject: (scopeId: string, projectId: string) =>
      request<undefined>(url`/scopes/${scopeId}/projects`, {
        method: "POST",
        body: JSON.stringify({ project_id: projectId }),
      }),

    removeProject: (scopeId: string, projectId: string) =>
      request<undefined>(url`/scopes/${scopeId}/projects/${projectId}`, { method: "DELETE" }),

    search: (scopeId: string, data: SearchRequest) =>
      request<RetrievalSearchResult>(url`/scopes/${scopeId}/search`, {
        method: "POST",
        body: JSON.stringify(data),
      }),

    graphSearch: (scopeId: string, data: GraphSearchRequest) =>
      request<GraphSearchResult>(url`/scopes/${scopeId}/graph/search`, {
        method: "POST",
        body: JSON.stringify(data),
      }),
  },

  knowledgeBases: {
    list: () => request<import("./types").KnowledgeBase[]>("/knowledge-bases"),

    get: (id: string) => request<import("./types").KnowledgeBase>(url`/knowledge-bases/${id}`),

    create: (data: import("./types").CreateKnowledgeBaseRequest) =>
      request<import("./types").KnowledgeBase>("/knowledge-bases", {
        method: "POST",
        body: JSON.stringify(data),
      }),

    delete: (id: string) => request<undefined>(url`/knowledge-bases/${id}`, { method: "DELETE" }),

    index: (id: string) =>
      request<{ status: string }>(url`/knowledge-bases/${id}/index`, {
        method: "POST",
      }),

    listByScope: (scopeId: string) =>
      request<import("./types").KnowledgeBase[]>(url`/scopes/${scopeId}/knowledge-bases`),

    attachToScope: (scopeId: string, kbId: string) =>
      request<undefined>(url`/scopes/${scopeId}/knowledge-bases`, {
        method: "POST",
        body: JSON.stringify({ knowledge_base_id: kbId }),
      }),

    detachFromScope: (scopeId: string, kbId: string) =>
      request<undefined>(url`/scopes/${scopeId}/knowledge-bases/${kbId}`, { method: "DELETE" }),
  },
  settings: {
    get: () => request<import("./types").AppSettings>("/settings"),

    update: (data: { settings: import("./types").AppSettings }) =>
      request<{ status: string }>("/settings", {
        method: "PUT",
        body: JSON.stringify(data),
      }),
  },

  conversations: {
    create: (projectId: string, data?: import("./types").CreateConversationRequest) =>
      request<import("./types").Conversation>(url`/projects/${projectId}/conversations`, {
        method: "POST",
        body: JSON.stringify(data ?? {}),
      }),

    list: (projectId: string) =>
      request<import("./types").Conversation[]>(url`/projects/${projectId}/conversations`),

    get: (id: string) => request<import("./types").Conversation>(url`/conversations/${id}`),

    delete: (id: string) =>
      request<undefined>(url`/conversations/${id}`, {
        method: "DELETE",
      }),

    messages: (id: string) =>
      request<import("./types").ConversationMessage[]>(url`/conversations/${id}/messages`),

    send: (id: string, data: import("./types").SendMessageRequest) =>
      request<import("./types").ConversationMessage>(url`/conversations/${id}/messages`, {
        method: "POST",
        body: JSON.stringify(data),
      }),

    stop: (id: string) =>
      request<{ status: string; conversation_id: string }>(url`/conversations/${id}/stop`, {
        method: "POST",
      }),
  },

  vcsAccounts: {
    list: () => request<import("./types").VCSAccount[]>("/vcs-accounts"),

    create: (data: import("./types").CreateVCSAccountRequest) =>
      request<import("./types").VCSAccount>("/vcs-accounts", {
        method: "POST",
        body: JSON.stringify(data),
      }),

    delete: (id: string) =>
      request<undefined>(url`/vcs-accounts/${id}`, {
        method: "DELETE",
      }),

    test: (id: string) =>
      request<{ status: string }>(url`/vcs-accounts/${id}/test`, {
        method: "POST",
      }),
  },

  lsp: {
    start: (projectId: string, languages?: string[]) =>
      request<{ status: string }>(url`/projects/${projectId}/lsp/start`, {
        method: "POST",
        body: JSON.stringify({ languages }),
      }),

    stop: (projectId: string) =>
      request<{ status: string }>(url`/projects/${projectId}/lsp/stop`, {
        method: "POST",
      }),

    status: (projectId: string) => request<LSPServerInfo[]>(url`/projects/${projectId}/lsp/status`),

    diagnostics: (projectId: string, uri?: string) =>
      request<LSPDiagnostic[]>(
        `/projects/${encodeURIComponent(projectId)}/lsp/diagnostics${uri ? `?uri=${encodeURIComponent(uri)}` : ""}`,
      ),

    definition: (projectId: string, uri: string, line: number, character: number) =>
      request<LSPLocation[]>(url`/projects/${projectId}/lsp/definition`, {
        method: "POST",
        body: JSON.stringify({ uri, line, character }),
      }),

    references: (projectId: string, uri: string, line: number, character: number) =>
      request<LSPLocation[]>(url`/projects/${projectId}/lsp/references`, {
        method: "POST",
        body: JSON.stringify({ uri, line, character }),
      }),

    symbols: (projectId: string, uri: string) =>
      request<LSPDocumentSymbol[]>(url`/projects/${projectId}/lsp/symbols`, {
        method: "POST",
        body: JSON.stringify({ uri }),
      }),

    hover: (projectId: string, uri: string, line: number, character: number) =>
      request<LSPHoverResult>(url`/projects/${projectId}/lsp/hover`, {
        method: "POST",
        body: JSON.stringify({ uri, line, character }),
      }),
  },

  dev: {
    benchmark: (body: import("./types").BenchmarkRequest) =>
      request<import("./types").BenchmarkResult>("/dev/benchmark", {
        method: "POST",
        body: JSON.stringify(body),
      }),
  },

  mcp: {
    listServers: () => request<MCPServer[]>("/mcp/servers"),

    getServer: (id: string) => request<MCPServer>(url`/mcp/servers/${id}`),

    createServer: (data: CreateMCPServerRequest) =>
      request<MCPServer>("/mcp/servers", {
        method: "POST",
        body: JSON.stringify(data),
      }),

    updateServer: (id: string, data: CreateMCPServerRequest) =>
      request<MCPServer>(url`/mcp/servers/${id}`, {
        method: "PUT",
        body: JSON.stringify(data),
      }),

    deleteServer: (id: string) => request<undefined>(url`/mcp/servers/${id}`, { method: "DELETE" }),

    testServer: (id: string) =>
      request<MCPTestResult>(url`/mcp/servers/${id}/test`, {
        method: "POST",
      }),

    testConnection: (data: CreateMCPServerRequest) =>
      request<MCPTestResult>("/mcp/servers/test", {
        method: "POST",
        body: JSON.stringify(data),
      }),

    listTools: (id: string) => request<MCPServerTool[]>(url`/mcp/servers/${id}/tools`),

    listProjectServers: (projectId: string) =>
      request<MCPServer[]>(url`/projects/${projectId}/mcp-servers`),

    assignToProject: (projectId: string, serverId: string) =>
      request<undefined>(url`/projects/${projectId}/mcp-servers`, {
        method: "POST",
        body: JSON.stringify({ server_id: serverId }),
      }),

    unassignFromProject: (projectId: string, serverId: string) =>
      request<undefined>(url`/projects/${projectId}/mcp-servers/${serverId}`, { method: "DELETE" }),
  },

  promptSections: {
    list: (scope = "global") =>
      request<import("./types").PromptSectionRow[]>(url`/prompt-sections?scope=${scope}`),

    upsert: (data: import("./types").PromptSectionRow) =>
      request<import("./types").PromptSectionRow>("/prompt-sections", {
        method: "PUT",
        body: JSON.stringify(data),
      }),

    delete: (id: string) =>
      request<undefined>(url`/prompt-sections/${id}`, {
        method: "DELETE",
      }),

    preview: (data: import("./types").PromptPreviewRequest) =>
      request<import("./types").PromptPreviewResponse>("/prompt-sections/preview", {
        method: "POST",
        body: JSON.stringify(data),
      }),
  },

  // --- Benchmark Mode (Phase 20) ---
  benchmarks: {
    listRuns: () => request<import("./types").BenchmarkRun[]>("/benchmarks/runs"),

    getRun: (id: string) => request<import("./types").BenchmarkRun>(url`/benchmarks/runs/${id}`),

    createRun: (data: import("./types").CreateBenchmarkRunRequest) =>
      request<import("./types").BenchmarkRun>("/benchmarks/runs", {
        method: "POST",
        body: JSON.stringify(data),
      }),

    deleteRun: (id: string) =>
      request<undefined>(url`/benchmarks/runs/${id}`, {
        method: "DELETE",
      }),

    listResults: (runId: string) =>
      request<import("./types").BenchmarkResult[]>(url`/benchmarks/runs/${runId}/results`),

    compare: (runIdA: string, runIdB: string) =>
      request<import("./types").BenchmarkCompareResult>("/benchmarks/compare", {
        method: "POST",
        body: JSON.stringify({ run_id_a: runIdA, run_id_b: runIdB }),
      }),

    listDatasets: () => request<import("./types").BenchmarkDatasetInfo[]>("/benchmarks/datasets"),
  },
} as const;

export { FetchError };
