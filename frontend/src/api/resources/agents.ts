import type { CoreClient } from "../core";
import { url } from "../factory";
import type { Agent, CreateAgentRequest, CreateTaskRequest, Task } from "../types";

export function createAgentsResource(c: CoreClient) {
  return {
    list: (projectId: string) => c.get<Agent[]>(url`/projects/${projectId}/agents`),

    get: (id: string) => c.get<Agent>(url`/agents/${id}`),

    create: (projectId: string, data: CreateAgentRequest) =>
      c.post<Agent>(url`/projects/${projectId}/agents`, data),

    delete: (id: string) => c.del<undefined>(url`/agents/${id}`),

    dispatch: (agentId: string, taskId: string) =>
      c.post<{ status: string }>(url`/agents/${agentId}/dispatch`, { task_id: taskId }),

    stop: (agentId: string, taskId: string) =>
      c.post<{ status: string }>(url`/agents/${agentId}/stop`, { task_id: taskId }),

    active: (projectId: string) => c.get<Agent[]>(url`/projects/${projectId}/agents/active`),
  };
}

export function createTasksResource(c: CoreClient) {
  return {
    list: (projectId: string) => c.get<Task[]>(url`/projects/${projectId}/tasks`),

    create: (projectId: string, data: CreateTaskRequest) =>
      c.post<Task>(url`/projects/${projectId}/tasks`, data),
  };
}
