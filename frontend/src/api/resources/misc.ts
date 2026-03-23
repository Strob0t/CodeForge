import type { CoreClient } from "../core";
import { url } from "../factory";
import type {
  ActiveWorkItem,
  AgentPerf,
  AuditPage,
  AutoAgentStatus,
  BenchmarkRequest,
  BenchmarkResult,
  CreateGoalRequest,
  CreateMCPServerRequest,
  CreatePlanRequest,
  DailyCost,
  DashboardStats,
  DecomposeRequest,
  ExecutionPlan,
  GoalDiscoveryResult,
  GraphSearchRequest,
  GraphSearchResult,
  GraphStatus,
  HealthStatus,
  LSPServerInfo,
  MCPServer,
  MCPServerTool,
  MCPTestResult,
  ModelUsage,
  PlanGraph,
  ProjectCostBar,
  ProjectGoal,
  ProjectHealth,
  RepoMap,
  RetrievalIndexStatus,
  RetrievalSearchResult,
  ReviewDecision,
  Run,
  RunOutcome,
  SearchRequest,
  Session,
  StartRunRequest,
  SubAgentSearchRequest,
  SubAgentSearchResult,
  TrajectoryPage,
  UpdateGoalRequest,
} from "../types";

export function createHealthResource() {
  return {
    check: async (): Promise<HealthStatus> => {
      try {
        const r = await fetch("/health");
        if (!r.ok) return { status: "unavailable", dev_mode: false };
        return (await r.json()) as HealthStatus;
      } catch {
        return { status: "unavailable", dev_mode: false };
      }
    },
  };
}

export function createRunsResource(c: CoreClient) {
  return {
    start: (data: StartRunRequest) => c.post<Run>("/runs", data),
    get: (id: string) => c.get<Run>(url`/runs/${id}`),
    cancel: (id: string) => c.post<{ status: string }>(url`/runs/${id}/cancel`),
    listByTask: (taskId: string) => c.get<Run[]>(url`/tasks/${taskId}/runs`),
    resume: (id: string, data?: { prompt?: string }) =>
      c.post<Session>(url`/runs/${id}/resume`, data ?? {}),
    fork: (id: string, data?: { from_event_id?: string; prompt?: string }) =>
      c.post<Session>(url`/runs/${id}/fork`, data ?? {}),
    rewind: (id: string, data?: { to_event_id?: string }) =>
      c.post<Session>(url`/runs/${id}/rewind`, data ?? {}),
    approve: (runId: string, callId: string, decision: "allow" | "deny") =>
      c.post<{ status: string; run_id: string; call_id: string; decision: string }>(
        url`/runs/${runId}/approve/${callId}`,
        { decision },
      ),
    revert: (runId: string, callId: string) =>
      c.post<{ status: string }>(url`/runs/${runId}/revert/${callId}`),
  };
}

export function createSessionsResource(c: CoreClient) {
  return {
    list: (projectId: string) => c.get<Session[]>(url`/projects/${projectId}/sessions`),
  };
}

export function createPlansResource(c: CoreClient) {
  return {
    decompose: (projectId: string, data: DecomposeRequest) =>
      c.post<ExecutionPlan>(url`/projects/${projectId}/decompose`, data),
    list: (projectId: string) => c.get<ExecutionPlan[]>(url`/projects/${projectId}/plans`),
    get: (id: string) => c.get<ExecutionPlan>(url`/plans/${id}`),
    create: (projectId: string, data: CreatePlanRequest) =>
      c.post<ExecutionPlan>(url`/projects/${projectId}/plans`, data),
    start: (id: string) => c.post<ExecutionPlan>(url`/plans/${id}/start`),
    cancel: (id: string) => c.post<{ status: string }>(url`/plans/${id}/cancel`),
    graph: (id: string) => c.get<PlanGraph>(url`/plans/${id}/graph`),
    evaluateStep: (planId: string, stepId: string) =>
      c.post<ReviewDecision>(url`/plans/${planId}/steps/${stepId}/evaluate`),
  };
}

export function createRepomapResource(c: CoreClient) {
  return {
    get: (projectId: string) => c.get<RepoMap>(url`/projects/${projectId}/repomap`),
    generate: (projectId: string, activeFiles?: string[]) =>
      c.post<{ status: string }>(url`/projects/${projectId}/repomap`, {
        active_files: activeFiles ?? [],
      }),
  };
}

export function createRetrievalResource(c: CoreClient) {
  return {
    indexStatus: (projectId: string) =>
      c.get<RetrievalIndexStatus>(url`/projects/${projectId}/index`),
    buildIndex: (projectId: string, embeddingModel?: string) =>
      c.post<{ status: string }>(url`/projects/${projectId}/index`, {
        embedding_model: embeddingModel ?? "",
      }),
    search: (projectId: string, data: SearchRequest) =>
      c.post<RetrievalSearchResult>(url`/projects/${projectId}/search`, data),
    agentSearch: (projectId: string, data: SubAgentSearchRequest) =>
      c.post<SubAgentSearchResult>(url`/projects/${projectId}/search/agent`, data),
  };
}

export function createGraphResource(c: CoreClient) {
  return {
    status: (projectId: string) => c.get<GraphStatus>(url`/projects/${projectId}/graph/status`),
    build: (projectId: string) =>
      c.post<{ status: string }>(url`/projects/${projectId}/graph/build`),
    search: (projectId: string, data: GraphSearchRequest) =>
      c.post<GraphSearchResult>(url`/projects/${projectId}/graph/search`, data),
  };
}

export function createDashboardResource(c: CoreClient) {
  return {
    stats: () => c.get<DashboardStats>("/dashboard/stats"),
    projectHealth: (id: string) => c.get<ProjectHealth>(url`/projects/${id}/health`),
    costTrend: (days = 30) => c.get<DailyCost[]>(url`/dashboard/charts/cost-trend?days=${days}`),
    runOutcomes: (days = 7) =>
      c.get<RunOutcome[]>(url`/dashboard/charts/run-outcomes?days=${days}`),
    agentPerformance: () => c.get<AgentPerf[]>("/dashboard/charts/agent-performance"),
    modelUsage: () => c.get<ModelUsage[]>("/dashboard/charts/model-usage"),
    costByProject: () => c.get<ProjectCostBar[]>("/dashboard/charts/cost-by-project"),
  };
}

export function createTrajectoryResource(c: CoreClient) {
  return {
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
      return c.get<TrajectoryPage>(
        `/runs/${encodeURIComponent(runId)}/trajectory${qs ? `?${qs}` : ""}`,
      );
    },
    exportUrl: (runId: string) =>
      `${c.BASE}/runs/${encodeURIComponent(runId)}/trajectory/export?format=json`,
  };
}

export function createSearchResource(c: CoreClient) {
  return {
    global: (query: string, projectIds?: string[], limit?: number) =>
      c.post<{
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
        query,
        project_ids: projectIds,
        limit: limit ?? 20,
      }),

    conversations: (query: string, projectIds?: string[], limit?: number) =>
      c.post<{
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
        query,
        project_ids: projectIds,
        limit: limit ?? 20,
      }),
  };
}

export function createLSPResource(c: CoreClient) {
  return {
    start: (projectId: string, languages?: string[]) =>
      c.post<{ status: string }>(url`/projects/${projectId}/lsp/start`, { languages }),
    stop: (projectId: string) => c.post<{ status: string }>(url`/projects/${projectId}/lsp/stop`),
    status: (projectId: string) => c.get<LSPServerInfo[]>(url`/projects/${projectId}/lsp/status`),
  };
}

export function createDevResource(c: CoreClient) {
  return {
    benchmark: (body: BenchmarkRequest) => c.post<BenchmarkResult>("/dev/benchmark", body),
  };
}

export function createMCPResource(c: CoreClient) {
  return {
    listServers: () => c.get<MCPServer[]>("/mcp/servers"),
    createServer: (data: CreateMCPServerRequest) => c.post<MCPServer>("/mcp/servers", data),
    updateServer: (id: string, data: CreateMCPServerRequest) =>
      c.put<MCPServer>(url`/mcp/servers/${id}`, data),
    deleteServer: (id: string) => c.del<undefined>(url`/mcp/servers/${id}`),
    testServer: (id: string) => c.post<MCPTestResult>(url`/mcp/servers/${id}/test`),
    testConnection: (data: CreateMCPServerRequest) =>
      c.post<MCPTestResult>("/mcp/servers/test", data),
    listTools: (id: string) => c.get<MCPServerTool[]>(url`/mcp/servers/${id}/tools`),
  };
}

export function createAutoAgentResource(c: CoreClient) {
  return {
    start: (projectId: string) =>
      c.post<AutoAgentStatus>(url`/projects/${projectId}/auto-agent/start`),
    stop: (projectId: string) =>
      c.post<{ status: string }>(url`/projects/${projectId}/auto-agent/stop`),
    status: (projectId: string) =>
      c.get<AutoAgentStatus>(url`/projects/${projectId}/auto-agent/status`),
  };
}

export function createActiveWorkResource(c: CoreClient) {
  return {
    list: (projectId: string) => c.get<ActiveWorkItem[]>(url`/projects/${projectId}/active-work`),
  };
}

export function createGoalsResource(c: CoreClient) {
  return {
    list: (projectId: string) => c.get<ProjectGoal[]>(url`/projects/${projectId}/goals`),
    create: (projectId: string, data: CreateGoalRequest) =>
      c.post<ProjectGoal>(url`/projects/${projectId}/goals`, data),
    detect: (projectId: string) =>
      c.post<GoalDiscoveryResult>(url`/projects/${projectId}/goals/detect`),
    update: (id: string, data: UpdateGoalRequest) => c.put<ProjectGoal>(url`/goals/${id}`, data),
    delete: (id: string) => c.del<undefined>(url`/goals/${id}`),
    aiDiscover: (projectId: string) =>
      c.post<{ conversation_id: string; status: string }>(
        url`/projects/${projectId}/goals/ai-discover`,
      ),
  };
}

export function createChannelsResource(c: CoreClient) {
  return {
    list: () =>
      c.get<
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
      c.get<{
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
      return c.get<
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
      c.post<{ id: string }>(url`/channels/${id}/messages`, {
        content,
        sender_name: senderName,
        sender_type: "user",
      }),

    sendThreadReply: (
      channelId: string,
      parentId: string,
      data: { sender_name: string; sender_type: string; content: string },
    ) =>
      c.post<{
        id: string;
        channel_id: string;
        sender_type: string;
        sender_name: string;
        content: string;
        parent_id: string;
        created_at: string;
      }>(url`/channels/${channelId}/messages/${parentId}/thread`, data),
  };
}

export function createAuditResource(c: CoreClient) {
  return {
    list: (opts?: { action?: string; cursor?: string; limit?: number }) => {
      const params = new URLSearchParams();
      if (opts?.action) params.set("action", opts.action);
      if (opts?.cursor) params.set("cursor", opts.cursor);
      if (opts?.limit) params.set("limit", String(opts.limit));
      const qs = params.toString();
      return c.get<AuditPage>(`/audit${qs ? `?${qs}` : ""}`);
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
      return c.get<AuditPage>(`${url`/projects/${projectId}/audit`}${qs ? `?${qs}` : ""}`);
    },
  };
}
