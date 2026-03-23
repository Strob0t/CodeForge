import type { CoreClient } from "../core";
import { url } from "../factory";
import type {
  A2APushConfig,
  A2ARemoteAgent,
  A2ATask,
  CreateA2APushConfigRequest,
  CreateA2ARemoteAgentRequest,
  SendA2ATaskRequest,
} from "../types";

export function createA2AResource(c: CoreClient) {
  return {
    // Remote Agents
    listAgents: () => c.get<A2ARemoteAgent[]>("/a2a/agents"),

    registerAgent: (data: CreateA2ARemoteAgentRequest) =>
      c.post<A2ARemoteAgent>("/a2a/agents", data),

    deleteAgent: (id: string) => c.del<undefined>(url`/a2a/agents/${id}`),

    discoverAgent: (id: string) => c.post<A2ARemoteAgent>(url`/a2a/agents/${id}/discover`),

    // Tasks
    listTasks: (state?: string, direction?: string) => {
      const params = new URLSearchParams();
      if (state) params.set("state", state);
      if (direction) params.set("direction", direction);
      const qs = params.toString();
      return c.get<A2ATask[]>(`/a2a/tasks${qs ? `?${qs}` : ""}`);
    },

    getTask: (id: string) => c.get<A2ATask>(url`/a2a/tasks/${id}`),

    cancelTask: (id: string) => c.post<{ status: string }>(url`/a2a/tasks/${id}/cancel`),

    sendTask: (agentId: string, data: SendA2ATaskRequest) =>
      c.post<A2ATask>(url`/a2a/agents/${agentId}/send`, data),

    // Push Configs
    listPushConfigs: (taskId: string) =>
      c.get<A2APushConfig[]>(url`/a2a/tasks/${taskId}/push-config`),

    createPushConfig: (taskId: string, data: CreateA2APushConfigRequest) =>
      c.post<{ id: string }>(url`/a2a/tasks/${taskId}/push-config`, data),

    deletePushConfig: (id: string) => c.del<undefined>(url`/a2a/push-config/${id}`),
  };
}
