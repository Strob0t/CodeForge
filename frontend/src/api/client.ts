import type {
  ApiError,
  BackendList,
  CreateProjectRequest,
  CreateTaskRequest,
  HealthStatus,
  Project,
  ProviderList,
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
  },

  providers: {
    git: () => request<ProviderList>("/providers/git"),
    agent: () => request<BackendList>("/providers/agent"),
  },
} as const;

export { FetchError };
