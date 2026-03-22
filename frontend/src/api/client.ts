import { createCoreClient, FetchError, getAccessToken, setAccessTokenGetter } from "./core";
import { url } from "./factory";
import { createAgentsResource, createTasksResource } from "./resources/agents";
import {
  createAuthResource,
  createSubscriptionProvidersResource,
  createVCSAccountsResource,
} from "./resources/auth";
import { createBenchmarksResource } from "./resources/benchmarks";
import { createConversationsResource } from "./resources/conversations";
import { createFilesResource } from "./resources/files";
import { createCostsResource, createLLMResource, createProvidersResource } from "./resources/llm";
import { createBatchResource, createProjectsResource } from "./resources/projects";
import { createRoadmapResource } from "./resources/roadmap";
import type {
  ActiveWorkItem,
  AgentPerf,
  CreateMCPServerRequest,
  CreateModeRequest,
  CreatePlanRequest,
  CreateUserRequest,
  DailyCost,
  DashboardStats,
  DecomposeRequest,
  EvaluationResult,
  ExecutionPlan,
  GraphSearchRequest,
  GraphSearchResult,
  GraphStatus,
  HealthStatus,
  LSPDiagnostic,
  LSPDocumentSymbol,
  LSPHoverResult,
  LSPLocation,
  LSPServerInfo,
  MCPServer,
  MCPServerTool,
  MCPTestResult,
  Mode,
  ModelUsage,
  PlanFeatureRequest,
  PlanGraph,
  PolicyProfile,
  PolicyToolCall,
  ProjectCostBar,
  ProjectHealth,
  RepoMap,
  RetrievalIndexStatus,
  RetrievalSearchResult,
  ReviewDecision,
  Run,
  RunOutcome,
  SearchRequest,
  StartRunRequest,
  SubAgentSearchRequest,
  SubAgentSearchResult,
  TrajectoryPage,
  UpdateUserRequest,
  User,
} from "./types";

export { FetchError, getAccessToken, setAccessTokenGetter };

const core = createCoreClient();
const { request } = core;
const BASE = core.BASE;

export const api = {
  health: {
    check: async (): Promise<HealthStatus> => {
      try {
        const r = await fetch("/health");
        if (!r.ok) return { status: "unavailable", dev_mode: false };
        return (await r.json()) as HealthStatus;
      } catch {
        return { status: "unavailable", dev_mode: false };
      }
    },
  },

  projects: createProjectsResource(core),

  batch: createBatchResource(core),

  agents: createAgentsResource(core),

  tasks: createTasksResource(core),

  llm: createLLMResource(core),

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

    resume: (id: string, data?: { prompt?: string }) =>
      request<import("./types").Session>(url`/runs/${id}/resume`, {
        method: "POST",
        body: JSON.stringify(data ?? {}),
      }),

    fork: (id: string, data?: { from_event_id?: string; prompt?: string }) =>
      request<import("./types").Session>(url`/runs/${id}/fork`, {
        method: "POST",
        body: JSON.stringify(data ?? {}),
      }),

    rewind: (id: string, data?: { to_event_id?: string }) =>
      request<import("./types").Session>(url`/runs/${id}/rewind`, {
        method: "POST",
        body: JSON.stringify(data ?? {}),
      }),

    approve: (runId: string, callId: string, decision: "allow" | "deny") =>
      request<{ status: string; run_id: string; call_id: string; decision: string }>(
        url`/runs/${runId}/approve/${callId}`,
        {
          method: "POST",
          body: JSON.stringify({ decision }),
        },
      ),

    revert: (runId: string, callId: string) =>
      request<{ status: string }>(url`/runs/${runId}/revert/${callId}`, {
        method: "POST",
      }),
  },

  sessions: {
    list: (projectId: string) =>
      request<import("./types").Session[]>(url`/projects/${projectId}/sessions`),

    get: (id: string) => request<import("./types").Session>(url`/sessions/${id}`),
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

    tools: () => request<string[]>("/modes/tools"),

    artifactTypes: () => request<string[]>("/modes/artifact-types"),
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

    allowAlways: (projectId: string, tool: string, command?: string) =>
      request<PolicyProfile>("/policies/allow-always", {
        method: "POST",
        body: JSON.stringify({ project_id: projectId, tool, command }),
      }),
  },

  costs: createCostsResource(core),

  dashboard: {
    stats: () => request<DashboardStats>("/dashboard/stats"),

    projectHealth: (id: string) => request<ProjectHealth>(url`/projects/${id}/health`),

    costTrend: (days = 30) => request<DailyCost[]>(url`/dashboard/charts/cost-trend?days=${days}`),

    runOutcomes: (days = 7) =>
      request<RunOutcome[]>(url`/dashboard/charts/run-outcomes?days=${days}`),

    agentPerformance: () => request<AgentPerf[]>("/dashboard/charts/agent-performance"),

    modelUsage: () => request<ModelUsage[]>("/dashboard/charts/model-usage"),

    costByProject: () => request<ProjectCostBar[]>("/dashboard/charts/cost-by-project"),
  },

  roadmap: createRoadmapResource(core),

  trajectory: {
    get: (
      runId: string,
      opts?: { types?: string; cursor?: string; limit?: number; after_sequence?: number },
    ) => {
      const params = new URLSearchParams();
      if (opts?.types) params.set("types", opts.types);
      if (opts?.cursor) params.set("cursor", opts.cursor);
      if (opts?.limit) params.set("limit", String(opts.limit));
      if (opts?.after_sequence) params.set("after_sequence", String(opts.after_sequence));
      const qs = params.toString();
      return request<TrajectoryPage>(
        `/runs/${encodeURIComponent(runId)}/trajectory${qs ? `?${qs}` : ""}`,
      );
    },

    exportUrl: (runId: string) =>
      `${BASE}/runs/${encodeURIComponent(runId)}/trajectory/export?format=json`,
  },

  search: {
    global: (query: string, projectIds?: string[], limit?: number) =>
      request<{
        query: string;
        total: number;
        results: {
          project_id: string;
          file: string;
          start_line: number;
          end_line: number;
          snippet: string;
          language?: string;
          symbol_name?: string;
          score: number;
        }[];
      }>("/search", {
        method: "POST",
        body: JSON.stringify({
          query,
          project_ids: projectIds,
          limit: limit ?? 20,
        }),
      }),

    conversations: (query: string, projectIds?: string[], limit?: number) =>
      request<{
        query: string;
        total: number;
        results: {
          conversation_id: string;
          message_id: string;
          role: string;
          content: string;
          model: string;
          created_at: string;
        }[];
      }>("/search/conversations", {
        method: "POST",
        body: JSON.stringify({
          query,
          project_ids: projectIds,
          limit: limit ?? 20,
        }),
      }),
  },

  providers: createProvidersResource(core),
  auth: createAuthResource(core),

  users: {
    list: () => request<User[]>("/users"),
    create: (data: CreateUserRequest) => core.post<User>("/users", data),
    update: (id: string, data: UpdateUserRequest) => core.put<User>(url`/users/${id}`, data),
    delete: (id: string) => core.del<undefined>(url`/users/${id}`),
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

  agentConfig: {
    get: () => request<import("./types").AgentConfig>("/agent-config"),
  },

  conversations: createConversationsResource(core),

  vcsAccounts: createVCSAccountsResource(core),

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

  benchmarks: createBenchmarksResource(core),

  files: createFilesResource(core),

  autoAgent: {
    start: (projectId: string) =>
      request<import("./types").AutoAgentStatus>(url`/projects/${projectId}/auto-agent/start`, {
        method: "POST",
      }),

    stop: (projectId: string) =>
      request<{ status: string }>(url`/projects/${projectId}/auto-agent/stop`, {
        method: "POST",
      }),

    status: (projectId: string) =>
      request<import("./types").AutoAgentStatus>(url`/projects/${projectId}/auto-agent/status`),
  },

  activeWork: {
    list: (projectId: string) => request<ActiveWorkItem[]>(url`/projects/${projectId}/active-work`),
  },

  goals: {
    list: (projectId: string) =>
      request<import("./types").ProjectGoal[]>(url`/projects/${projectId}/goals`),

    create: (projectId: string, data: import("./types").CreateGoalRequest) =>
      request<import("./types").ProjectGoal>(url`/projects/${projectId}/goals`, {
        method: "POST",
        body: JSON.stringify(data),
      }),

    detect: (projectId: string) =>
      request<import("./types").GoalDiscoveryResult>(url`/projects/${projectId}/goals/detect`, {
        method: "POST",
      }),

    get: (id: string) => request<import("./types").ProjectGoal>(url`/goals/${id}`),

    update: (id: string, data: import("./types").UpdateGoalRequest) =>
      request<import("./types").ProjectGoal>(url`/goals/${id}`, {
        method: "PUT",
        body: JSON.stringify(data),
      }),

    delete: (id: string) => request<undefined>(url`/goals/${id}`, { method: "DELETE" }),

    aiDiscover: (projectId: string) =>
      request<{ conversation_id: string; status: string }>(
        url`/projects/${projectId}/goals/ai-discover`,
        { method: "POST" },
      ),
  },
  channels: {
    list: () =>
      request<
        {
          id: string;
          tenant_id: string;
          project_id: string;
          name: string;
          type: "project" | "bot";
          description: string;
          created_by: string;
          created_at: string;
        }[]
      >("/channels"),

    get: (id: string) =>
      request<{
        id: string;
        name: string;
        type: string;
        description: string;
        project_id: string;
        created_at: string;
      }>(url`/channels/${id}`),

    messages: (id: string, cursor?: string, limit?: number) => {
      const params = new URLSearchParams();
      if (cursor) params.set("cursor", cursor);
      if (limit) params.set("limit", String(limit));
      const qs = params.toString();
      return request<
        {
          id: string;
          channel_id: string;
          sender_type: string;
          sender_name: string;
          content: string;
          parent_id: string;
          created_at: string;
        }[]
      >(`/channels/${encodeURIComponent(id)}/messages${qs ? `?${qs}` : ""}`);
    },

    send: (id: string, content: string, senderName: string) =>
      request<{ id: string }>(url`/channels/${id}/messages`, {
        method: "POST",
        body: JSON.stringify({ content, sender_name: senderName, sender_type: "user" }),
      }),

    sendThreadReply: (
      channelId: string,
      parentId: string,
      data: { sender_name: string; sender_type: string; content: string },
    ) =>
      request<{
        id: string;
        channel_id: string;
        sender_type: string;
        sender_name: string;
        content: string;
        parent_id: string;
        created_at: string;
      }>(url`/channels/${channelId}/messages/${parentId}/thread`, {
        method: "POST",
        body: JSON.stringify(data),
      }),
  },

  subscriptionProviders: createSubscriptionProvidersResource(core),

  audit: {
    list: (opts?: { action?: string; cursor?: string; limit?: number }) => {
      const params = new URLSearchParams();
      if (opts?.action) params.set("action", opts.action);
      if (opts?.cursor) params.set("cursor", opts.cursor);
      if (opts?.limit) params.set("limit", String(opts.limit));
      const qs = params.toString();
      return request<import("./types").AuditPage>(`/audit${qs ? `?${qs}` : ""}`);
    },

    listByProject: (
      projectId: string,
      opts?: { action?: string; cursor?: string; limit?: number },
    ) => {
      const params = new URLSearchParams();
      if (opts?.action) params.set("action", opts.action);
      if (opts?.cursor) params.set("cursor", opts.cursor);
      if (opts?.limit) params.set("limit", String(opts.limit));
      const qs = params.toString();
      return request<import("./types").AuditPage>(
        `${url`/projects/${projectId}/audit`}${qs ? `?${qs}` : ""}`,
      );
    },
  },
} as const;
