import type {
  AddModelRequest,
  Agent,
  AgentEvent,
  ApiError,
  BackendList,
  Branch,
  CreateAgentRequest,
  CreatePlanRequest,
  CreateProjectRequest,
  CreateTaskRequest,
  DecomposeRequest,
  ExecutionPlan,
  GitStatus,
  HealthStatus,
  LLMModel,
  Project,
  ProviderList,
  Run,
  StartRunRequest,
  Task,
} from "./types";

const BASE = "/api/v1";

class FetchError extends Error {
  constructor(
    public readonly status: number,
    public readonly body: ApiError,
  ) {
    super(body.error);
    this.name = "FetchError";
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      ...init?.headers,
    },
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

    delete: (id: string) =>
      request<void>(`/projects/${encodeURIComponent(id)}`, {
        method: "DELETE",
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
      request<void>(`/agents/${encodeURIComponent(id)}`, { method: "DELETE" }),

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
  },

  llm: {
    models: () => request<LLMModel[]>("/llm/models"),

    addModel: (data: AddModelRequest) =>
      request<void>("/llm/models", {
        method: "POST",
        body: JSON.stringify(data),
      }),

    deleteModel: (modelId: string) =>
      request<void>(`/llm/models/${encodeURIComponent(modelId)}`, {
        method: "DELETE",
      }),

    health: () => request<{ status: string }>("/llm/health"),
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

  plans: {
    decompose: (projectId: string, data: DecomposeRequest) =>
      request<ExecutionPlan>(`/projects/${encodeURIComponent(projectId)}/decompose`, {
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

  policies: {
    list: () => request<{ profiles: string[] }>("/policies"),
  },

  providers: {
    git: () => request<ProviderList>("/providers/git"),
    agent: () => request<BackendList>("/providers/agent"),
  },
} as const;

export { FetchError };
