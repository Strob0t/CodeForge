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
  GitStatus,
  GraphSearchRequest,
  GraphSearchResult,
  GraphStatus,
  HealthStatus,
  ImportResult,
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
  PMImportRequest,
  PolicyProfile,
  PolicyToolCall,
  Project,
  ProjectCostSummary,
  ProviderInfo,
  ProviderList,
  RepoMap,
  RetrievalIndexStatus,
  RetrievalSearchResult,
  Roadmap,
  RoadmapFeature,
  Run,
  SearchRequest,
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

const MAX_RETRIES = 3;
const RETRY_BASE_MS = 1000;
const RETRYABLE_STATUSES = new Set([502, 503, 504]);

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
    const body = (await res.json()) as ApiError;
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

    get: (id: string) => request<Project>(`/projects/${encodeURIComponent(id)}`),

    create: (data: CreateProjectRequest) =>
      request<Project>("/projects", {
        method: "POST",
        body: JSON.stringify(data),
      }),

    update: (id: string, data: import("./types").UpdateProjectRequest) =>
      request<Project>(`/projects/${encodeURIComponent(id)}`, {
        method: "PUT",
        body: JSON.stringify(data),
      }),

    delete: (id: string) =>
      request<undefined>(`/projects/${encodeURIComponent(id)}`, {
        method: "DELETE",
      }),

    parseRepoURL: (url: string) =>
      request<import("./types").ParsedRepoURL>("/parse-repo-url", {
        method: "POST",
        body: JSON.stringify({ url }),
      }),

    clone: (id: string) =>
      request<Project>(`/projects/${encodeURIComponent(id)}/clone`, {
        method: "POST",
      }),

    gitStatus: (id: string) => request<GitStatus>(`/projects/${encodeURIComponent(id)}/git/status`),

    pull: (id: string) =>
      request<{ status: string }>(`/projects/${encodeURIComponent(id)}/git/pull`, {
        method: "POST",
      }),

    branches: (id: string) => request<Branch[]>(`/projects/${encodeURIComponent(id)}/git/branches`),

    checkout: (id: string, branch: string) =>
      request<{ status: string; branch: string }>(
        `/projects/${encodeURIComponent(id)}/git/checkout`,
        {
          method: "POST",
          body: JSON.stringify({ branch }),
        },
      ),

    detectStack: (id: string) =>
      request<import("./types").StackDetectionResult>(
        `/projects/${encodeURIComponent(id)}/detect-stack`,
      ),

    detectStackByPath: (path: string) =>
      request<import("./types").StackDetectionResult>("/detect-stack", {
        method: "POST",
        body: JSON.stringify({ path }),
      }),

    setup: (id: string, branch?: string) =>
      request<import("./types").SetupResult>(`/projects/${encodeURIComponent(id)}/setup`, {
        method: "POST",
        body: JSON.stringify(branch ? { branch } : {}),
      }),

    adopt: (id: string, body: { path: string }) =>
      request<Project>(`/projects/${encodeURIComponent(id)}/adopt`, {
        method: "POST",
        body: JSON.stringify(body),
      }),

    remoteBranches: (url: string) =>
      request<{ branches: string[] }>(
        `/projects/remote-branches?url=${encodeURIComponent(url)}`,
      ).then((r) => r.branches),
  },

  agents: {
    list: (projectId: string) =>
      request<Agent[]>(`/projects/${encodeURIComponent(projectId)}/agents`),

    get: (id: string) => request<Agent>(`/agents/${encodeURIComponent(id)}`),

    create: (projectId: string, data: CreateAgentRequest) =>
      request<Agent>(`/projects/${encodeURIComponent(projectId)}/agents`, {
        method: "POST",
        body: JSON.stringify(data),
      }),

    delete: (id: string) =>
      request<undefined>(`/agents/${encodeURIComponent(id)}`, { method: "DELETE" }),

    dispatch: (agentId: string, taskId: string) =>
      request<{ status: string }>(`/agents/${encodeURIComponent(agentId)}/dispatch`, {
        method: "POST",
        body: JSON.stringify({ task_id: taskId }),
      }),

    stop: (agentId: string, taskId: string) =>
      request<{ status: string }>(`/agents/${encodeURIComponent(agentId)}/stop`, {
        method: "POST",
        body: JSON.stringify({ task_id: taskId }),
      }),
  },

  tasks: {
    list: (projectId: string) =>
      request<Task[]>(`/projects/${encodeURIComponent(projectId)}/tasks`),

    get: (id: string) => request<Task>(`/tasks/${encodeURIComponent(id)}`),

    create: (projectId: string, data: CreateTaskRequest) =>
      request<Task>(`/projects/${encodeURIComponent(projectId)}/tasks`, {
        method: "POST",
        body: JSON.stringify(data),
      }),

    events: (taskId: string) =>
      request<AgentEvent[]>(`/tasks/${encodeURIComponent(taskId)}/events`),

    context: (taskId: string) =>
      request<ContextPack>(`/tasks/${encodeURIComponent(taskId)}/context`),

    buildContext: (taskId: string, projectId: string, teamId?: string) =>
      request<ContextPack>(`/tasks/${encodeURIComponent(taskId)}/context`, {
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
      request<undefined>(`/llm/models/${encodeURIComponent(modelId)}`, {
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

    get: (id: string) => request<Run>(`/runs/${encodeURIComponent(id)}`),

    cancel: (id: string) =>
      request<{ status: string }>(`/runs/${encodeURIComponent(id)}/cancel`, {
        method: "POST",
      }),

    listByTask: (taskId: string) => request<Run[]>(`/tasks/${encodeURIComponent(taskId)}/runs`),
  },

  teams: {
    list: (projectId: string) =>
      request<AgentTeam[]>(`/projects/${encodeURIComponent(projectId)}/teams`),

    get: (id: string) => request<AgentTeam>(`/teams/${encodeURIComponent(id)}`),

    create: (projectId: string, data: CreateTeamRequest) =>
      request<AgentTeam>(`/projects/${encodeURIComponent(projectId)}/teams`, {
        method: "POST",
        body: JSON.stringify(data),
      }),

    delete: (id: string) =>
      request<undefined>(`/teams/${encodeURIComponent(id)}`, { method: "DELETE" }),

    sharedContext: (teamId: string) =>
      request<SharedContext>(`/teams/${encodeURIComponent(teamId)}/shared-context`),

    addSharedItem: (teamId: string, data: AddSharedItemRequest) =>
      request<SharedContextItem>(`/teams/${encodeURIComponent(teamId)}/shared-context`, {
        method: "POST",
        body: JSON.stringify(data),
      }),
  },

  plans: {
    decompose: (projectId: string, data: DecomposeRequest) =>
      request<ExecutionPlan>(`/projects/${encodeURIComponent(projectId)}/decompose`, {
        method: "POST",
        body: JSON.stringify(data),
      }),

    planFeature: (projectId: string, data: PlanFeatureRequest) =>
      request<ExecutionPlan>(`/projects/${encodeURIComponent(projectId)}/plan-feature`, {
        method: "POST",
        body: JSON.stringify(data),
      }),

    list: (projectId: string) =>
      request<ExecutionPlan[]>(`/projects/${encodeURIComponent(projectId)}/plans`),

    get: (id: string) => request<ExecutionPlan>(`/plans/${encodeURIComponent(id)}`),

    create: (projectId: string, data: CreatePlanRequest) =>
      request<ExecutionPlan>(`/projects/${encodeURIComponent(projectId)}/plans`, {
        method: "POST",
        body: JSON.stringify(data),
      }),

    start: (id: string) =>
      request<ExecutionPlan>(`/plans/${encodeURIComponent(id)}/start`, {
        method: "POST",
      }),

    cancel: (id: string) =>
      request<{ status: string }>(`/plans/${encodeURIComponent(id)}/cancel`, {
        method: "POST",
      }),
  },

  modes: {
    list: () => request<Mode[]>("/modes"),

    get: (id: string) => request<Mode>(`/modes/${encodeURIComponent(id)}`),

    create: (data: CreateModeRequest) =>
      request<Mode>("/modes", {
        method: "POST",
        body: JSON.stringify(data),
      }),

    update: (id: string, data: CreateModeRequest) =>
      request<Mode>(`/modes/${encodeURIComponent(id)}`, {
        method: "PUT",
        body: JSON.stringify(data),
      }),

    scenarios: () => request<string[]>("/modes/scenarios"),
  },

  repomap: {
    get: (projectId: string) =>
      request<RepoMap>(`/projects/${encodeURIComponent(projectId)}/repomap`),

    generate: (projectId: string, activeFiles?: string[]) =>
      request<{ status: string }>(`/projects/${encodeURIComponent(projectId)}/repomap`, {
        method: "POST",
        body: JSON.stringify({ active_files: activeFiles ?? [] }),
      }),
  },

  retrieval: {
    indexStatus: (projectId: string) =>
      request<RetrievalIndexStatus>(`/projects/${encodeURIComponent(projectId)}/index`),

    buildIndex: (projectId: string, embeddingModel?: string) =>
      request<{ status: string }>(`/projects/${encodeURIComponent(projectId)}/index`, {
        method: "POST",
        body: JSON.stringify({ embedding_model: embeddingModel ?? "" }),
      }),

    search: (projectId: string, data: SearchRequest) =>
      request<RetrievalSearchResult>(`/projects/${encodeURIComponent(projectId)}/search`, {
        method: "POST",
        body: JSON.stringify(data),
      }),

    agentSearch: (projectId: string, data: SubAgentSearchRequest) =>
      request<SubAgentSearchResult>(`/projects/${encodeURIComponent(projectId)}/search/agent`, {
        method: "POST",
        body: JSON.stringify(data),
      }),
  },

  graph: {
    status: (projectId: string) =>
      request<GraphStatus>(`/projects/${encodeURIComponent(projectId)}/graph/status`),

    build: (projectId: string) =>
      request<{ status: string }>(`/projects/${encodeURIComponent(projectId)}/graph/build`, {
        method: "POST",
      }),

    search: (projectId: string, data: GraphSearchRequest) =>
      request<GraphSearchResult>(`/projects/${encodeURIComponent(projectId)}/graph/search`, {
        method: "POST",
        body: JSON.stringify(data),
      }),
  },

  policies: {
    list: () => request<{ profiles: string[] }>("/policies"),

    get: (name: string) => request<PolicyProfile>(`/policies/${encodeURIComponent(name)}`),

    create: (profile: PolicyProfile) =>
      request<PolicyProfile>("/policies", {
        method: "POST",
        body: JSON.stringify(profile),
      }),

    delete: (name: string) =>
      request<undefined>(`/policies/${encodeURIComponent(name)}`, {
        method: "DELETE",
      }),

    evaluate: (name: string, call: PolicyToolCall) =>
      request<EvaluationResult>(`/policies/${encodeURIComponent(name)}/evaluate`, {
        method: "POST",
        body: JSON.stringify(call),
      }),
  },

  costs: {
    global: () => request<ProjectCostSummary[]>("/costs"),

    project: (id: string) => request<CostSummary>(`/projects/${encodeURIComponent(id)}/costs`),

    byModel: (id: string) =>
      request<ModelCostSummary[]>(`/projects/${encodeURIComponent(id)}/costs/by-model`),

    daily: (id: string, days = 30) =>
      request<DailyCost[]>(`/projects/${encodeURIComponent(id)}/costs/daily?days=${days}`),

    recentRuns: (id: string, limit = 20) =>
      request<Run[]>(`/projects/${encodeURIComponent(id)}/costs/runs?limit=${limit}`),

    byTool: (id: string) =>
      request<ToolCostSummary[]>(`/projects/${encodeURIComponent(id)}/costs/by-tool`),

    byToolForRun: (runId: string) =>
      request<ToolCostSummary[]>(`/runs/${encodeURIComponent(runId)}/costs/by-tool`),
  },

  roadmap: {
    get: (projectId: string) =>
      request<Roadmap>(`/projects/${encodeURIComponent(projectId)}/roadmap`),

    create: (projectId: string, data: CreateRoadmapRequest) =>
      request<Roadmap>(`/projects/${encodeURIComponent(projectId)}/roadmap`, {
        method: "POST",
        body: JSON.stringify(data),
      }),

    update: (projectId: string, data: Partial<Roadmap> & { version: number }) =>
      request<Roadmap>(`/projects/${encodeURIComponent(projectId)}/roadmap`, {
        method: "PUT",
        body: JSON.stringify(data),
      }),

    delete: (projectId: string) =>
      request<undefined>(`/projects/${encodeURIComponent(projectId)}/roadmap`, {
        method: "DELETE",
      }),

    ai: (projectId: string, format: "json" | "yaml" | "markdown" = "markdown") =>
      request<AIRoadmapView>(
        `/projects/${encodeURIComponent(projectId)}/roadmap/ai?format=${format}`,
      ),

    detect: (projectId: string) =>
      request<DetectionResult>(`/projects/${encodeURIComponent(projectId)}/roadmap/detect`, {
        method: "POST",
      }),

    createMilestone: (projectId: string, data: CreateMilestoneRequest) =>
      request<Milestone>(`/projects/${encodeURIComponent(projectId)}/roadmap/milestones`, {
        method: "POST",
        body: JSON.stringify(data),
      }),

    updateMilestone: (id: string, data: Partial<Milestone> & { version: number }) =>
      request<Milestone>(`/milestones/${encodeURIComponent(id)}`, {
        method: "PUT",
        body: JSON.stringify(data),
      }),

    deleteMilestone: (id: string) =>
      request<undefined>(`/milestones/${encodeURIComponent(id)}`, { method: "DELETE" }),

    createFeature: (milestoneId: string, data: CreateFeatureRequest) =>
      request<RoadmapFeature>(`/milestones/${encodeURIComponent(milestoneId)}/features`, {
        method: "POST",
        body: JSON.stringify(data),
      }),

    updateFeature: (id: string, data: Partial<RoadmapFeature> & { version: number }) =>
      request<RoadmapFeature>(`/features/${encodeURIComponent(id)}`, {
        method: "PUT",
        body: JSON.stringify(data),
      }),

    deleteFeature: (id: string) =>
      request<undefined>(`/features/${encodeURIComponent(id)}`, { method: "DELETE" }),

    importSpecs: (projectId: string) =>
      request<ImportResult>(`/projects/${encodeURIComponent(projectId)}/roadmap/import`, {
        method: "POST",
      }),

    importPMItems: (projectId: string, data: PMImportRequest) =>
      request<ImportResult>(`/projects/${encodeURIComponent(projectId)}/roadmap/import/pm`, {
        method: "POST",
        body: JSON.stringify(data),
      }),

    syncToFile: (projectId: string) =>
      request<{ status: string }>(
        `/projects/${encodeURIComponent(projectId)}/roadmap/sync-to-file`,
        { method: "POST" },
      ),
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

    createAPIKey: (data: CreateAPIKeyRequest) =>
      request<CreateAPIKeyResponse>("/auth/api-keys", {
        method: "POST",
        body: JSON.stringify(data),
      }),

    listAPIKeys: () => request<APIKeyInfo[]>("/auth/api-keys"),

    deleteAPIKey: (id: string) =>
      request<undefined>(`/auth/api-keys/${encodeURIComponent(id)}`, {
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
      request<User>(`/users/${encodeURIComponent(id)}`, {
        method: "PUT",
        body: JSON.stringify(data),
      }),

    delete: (id: string) =>
      request<undefined>(`/users/${encodeURIComponent(id)}`, {
        method: "DELETE",
      }),
  },

  reviews: {
    listPolicies: (projectId: string) =>
      request<import("./types").ReviewPolicy[]>(
        `/projects/${encodeURIComponent(projectId)}/review-policies`,
      ),

    createPolicy: (projectId: string, data: Partial<import("./types").ReviewPolicy>) =>
      request<import("./types").ReviewPolicy>(
        `/projects/${encodeURIComponent(projectId)}/review-policies`,
        { method: "POST", body: JSON.stringify(data) },
      ),

    getPolicy: (id: string) =>
      request<import("./types").ReviewPolicy>(`/review-policies/${encodeURIComponent(id)}`),

    updatePolicy: (id: string, data: Partial<import("./types").ReviewPolicy>) =>
      request<import("./types").ReviewPolicy>(`/review-policies/${encodeURIComponent(id)}`, {
        method: "PUT",
        body: JSON.stringify(data),
      }),

    deletePolicy: (id: string) =>
      request<undefined>(`/review-policies/${encodeURIComponent(id)}`, { method: "DELETE" }),

    trigger: (policyId: string) =>
      request<import("./types").Review>(
        `/review-policies/${encodeURIComponent(policyId)}/trigger`,
        { method: "POST" },
      ),

    list: (projectId: string) =>
      request<import("./types").Review[]>(`/projects/${encodeURIComponent(projectId)}/reviews`),

    get: (id: string) => request<import("./types").Review>(`/reviews/${encodeURIComponent(id)}`),
  },

  scopes: {
    list: () => request<import("./types").RetrievalScope[]>("/scopes"),

    get: (id: string) =>
      request<import("./types").RetrievalScope>(`/scopes/${encodeURIComponent(id)}`),

    create: (data: import("./types").CreateScopeRequest) =>
      request<import("./types").RetrievalScope>("/scopes", {
        method: "POST",
        body: JSON.stringify(data),
      }),

    update: (id: string, data: import("./types").UpdateScopeRequest) =>
      request<import("./types").RetrievalScope>(`/scopes/${encodeURIComponent(id)}`, {
        method: "PUT",
        body: JSON.stringify(data),
      }),

    delete: (id: string) =>
      request<undefined>(`/scopes/${encodeURIComponent(id)}`, { method: "DELETE" }),

    addProject: (scopeId: string, projectId: string) =>
      request<undefined>(`/scopes/${encodeURIComponent(scopeId)}/projects`, {
        method: "POST",
        body: JSON.stringify({ project_id: projectId }),
      }),

    removeProject: (scopeId: string, projectId: string) =>
      request<undefined>(
        `/scopes/${encodeURIComponent(scopeId)}/projects/${encodeURIComponent(projectId)}`,
        { method: "DELETE" },
      ),

    search: (scopeId: string, data: SearchRequest) =>
      request<RetrievalSearchResult>(`/scopes/${encodeURIComponent(scopeId)}/search`, {
        method: "POST",
        body: JSON.stringify(data),
      }),

    graphSearch: (scopeId: string, data: GraphSearchRequest) =>
      request<GraphSearchResult>(`/scopes/${encodeURIComponent(scopeId)}/graph/search`, {
        method: "POST",
        body: JSON.stringify(data),
      }),
  },

  knowledgeBases: {
    list: () => request<import("./types").KnowledgeBase[]>("/knowledge-bases"),

    get: (id: string) =>
      request<import("./types").KnowledgeBase>(`/knowledge-bases/${encodeURIComponent(id)}`),

    create: (data: import("./types").CreateKnowledgeBaseRequest) =>
      request<import("./types").KnowledgeBase>("/knowledge-bases", {
        method: "POST",
        body: JSON.stringify(data),
      }),

    delete: (id: string) =>
      request<undefined>(`/knowledge-bases/${encodeURIComponent(id)}`, { method: "DELETE" }),

    index: (id: string) =>
      request<{ status: string }>(`/knowledge-bases/${encodeURIComponent(id)}/index`, {
        method: "POST",
      }),

    listByScope: (scopeId: string) =>
      request<import("./types").KnowledgeBase[]>(
        `/scopes/${encodeURIComponent(scopeId)}/knowledge-bases`,
      ),

    attachToScope: (scopeId: string, kbId: string) =>
      request<undefined>(`/scopes/${encodeURIComponent(scopeId)}/knowledge-bases`, {
        method: "POST",
        body: JSON.stringify({ knowledge_base_id: kbId }),
      }),

    detachFromScope: (scopeId: string, kbId: string) =>
      request<undefined>(
        `/scopes/${encodeURIComponent(scopeId)}/knowledge-bases/${encodeURIComponent(kbId)}`,
        { method: "DELETE" },
      ),
  },
  settings: {
    get: () => request<Record<string, unknown>>("/settings"),

    update: (data: { settings: Record<string, unknown> }) =>
      request<{ status: string }>("/settings", {
        method: "PUT",
        body: JSON.stringify(data),
      }),
  },

  conversations: {
    create: (projectId: string, data?: import("./types").CreateConversationRequest) =>
      request<import("./types").Conversation>(
        `/projects/${encodeURIComponent(projectId)}/conversations`,
        {
          method: "POST",
          body: JSON.stringify(data ?? {}),
        },
      ),

    list: (projectId: string) =>
      request<import("./types").Conversation[]>(
        `/projects/${encodeURIComponent(projectId)}/conversations`,
      ),

    get: (id: string) =>
      request<import("./types").Conversation>(`/conversations/${encodeURIComponent(id)}`),

    delete: (id: string) =>
      request<undefined>(`/conversations/${encodeURIComponent(id)}`, {
        method: "DELETE",
      }),

    messages: (id: string) =>
      request<import("./types").ConversationMessage[]>(
        `/conversations/${encodeURIComponent(id)}/messages`,
      ),

    send: (id: string, data: import("./types").SendMessageRequest) =>
      request<import("./types").ConversationMessage>(
        `/conversations/${encodeURIComponent(id)}/messages`,
        {
          method: "POST",
          body: JSON.stringify(data),
        },
      ),
  },

  vcsAccounts: {
    list: () => request<import("./types").VCSAccount[]>("/vcs-accounts"),

    create: (data: import("./types").CreateVCSAccountRequest) =>
      request<import("./types").VCSAccount>("/vcs-accounts", {
        method: "POST",
        body: JSON.stringify(data),
      }),

    delete: (id: string) =>
      request<undefined>(`/vcs-accounts/${encodeURIComponent(id)}`, {
        method: "DELETE",
      }),

    test: (id: string) =>
      request<{ status: string }>(`/vcs-accounts/${encodeURIComponent(id)}/test`, {
        method: "POST",
      }),
  },

  lsp: {
    start: (projectId: string, languages?: string[]) =>
      request<{ status: string }>(`/projects/${encodeURIComponent(projectId)}/lsp/start`, {
        method: "POST",
        body: JSON.stringify({ languages }),
      }),

    stop: (projectId: string) =>
      request<{ status: string }>(`/projects/${encodeURIComponent(projectId)}/lsp/stop`, {
        method: "POST",
      }),

    status: (projectId: string) =>
      request<LSPServerInfo[]>(`/projects/${encodeURIComponent(projectId)}/lsp/status`),

    diagnostics: (projectId: string, uri?: string) =>
      request<LSPDiagnostic[]>(
        `/projects/${encodeURIComponent(projectId)}/lsp/diagnostics${uri ? `?uri=${encodeURIComponent(uri)}` : ""}`,
      ),

    definition: (projectId: string, uri: string, line: number, character: number) =>
      request<LSPLocation[]>(`/projects/${encodeURIComponent(projectId)}/lsp/definition`, {
        method: "POST",
        body: JSON.stringify({ uri, line, character }),
      }),

    references: (projectId: string, uri: string, line: number, character: number) =>
      request<LSPLocation[]>(`/projects/${encodeURIComponent(projectId)}/lsp/references`, {
        method: "POST",
        body: JSON.stringify({ uri, line, character }),
      }),

    symbols: (projectId: string, uri: string) =>
      request<LSPDocumentSymbol[]>(`/projects/${encodeURIComponent(projectId)}/lsp/symbols`, {
        method: "POST",
        body: JSON.stringify({ uri }),
      }),

    hover: (projectId: string, uri: string, line: number, character: number) =>
      request<LSPHoverResult>(`/projects/${encodeURIComponent(projectId)}/lsp/hover`, {
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

    getServer: (id: string) => request<MCPServer>(`/mcp/servers/${encodeURIComponent(id)}`),

    createServer: (data: CreateMCPServerRequest) =>
      request<MCPServer>("/mcp/servers", {
        method: "POST",
        body: JSON.stringify(data),
      }),

    updateServer: (id: string, data: CreateMCPServerRequest) =>
      request<MCPServer>(`/mcp/servers/${encodeURIComponent(id)}`, {
        method: "PUT",
        body: JSON.stringify(data),
      }),

    deleteServer: (id: string) =>
      request<undefined>(`/mcp/servers/${encodeURIComponent(id)}`, { method: "DELETE" }),

    testServer: (id: string) =>
      request<MCPTestResult>(`/mcp/servers/${encodeURIComponent(id)}/test`, {
        method: "POST",
      }),

    testConnection: (data: CreateMCPServerRequest) =>
      request<MCPTestResult>("/mcp/servers/test", {
        method: "POST",
        body: JSON.stringify(data),
      }),

    listTools: (id: string) =>
      request<MCPServerTool[]>(`/mcp/servers/${encodeURIComponent(id)}/tools`),

    listProjectServers: (projectId: string) =>
      request<MCPServer[]>(`/projects/${encodeURIComponent(projectId)}/mcp-servers`),

    assignToProject: (projectId: string, serverId: string) =>
      request<undefined>(`/projects/${encodeURIComponent(projectId)}/mcp-servers`, {
        method: "POST",
        body: JSON.stringify({ server_id: serverId }),
      }),

    unassignFromProject: (projectId: string, serverId: string) =>
      request<undefined>(
        `/projects/${encodeURIComponent(projectId)}/mcp-servers/${encodeURIComponent(serverId)}`,
        { method: "DELETE" },
      ),
  },

  promptSections: {
    list: (scope = "global") =>
      request<import("./types").PromptSectionRow[]>(
        `/prompt-sections?scope=${encodeURIComponent(scope)}`,
      ),

    upsert: (data: import("./types").PromptSectionRow) =>
      request<import("./types").PromptSectionRow>("/prompt-sections", {
        method: "PUT",
        body: JSON.stringify(data),
      }),

    delete: (id: string) =>
      request<undefined>(`/prompt-sections/${encodeURIComponent(id)}`, {
        method: "DELETE",
      }),

    preview: (data: import("./types").PromptPreviewRequest) =>
      request<import("./types").PromptPreviewResponse>("/prompt-sections/preview", {
        method: "POST",
        body: JSON.stringify(data),
      }),
  },
} as const;

export { FetchError };
