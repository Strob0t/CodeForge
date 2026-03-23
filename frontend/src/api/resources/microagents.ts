import type { CoreClient } from "../core";
import { url } from "../factory";
import type {
  CreateMicroagentRequest,
  CreateSkillRequest,
  ImportSkillRequest,
  Microagent,
  Skill,
  UpdateMicroagentRequest,
  UpdateSkillRequest,
} from "../types";

export function createMicroagentsResource(c: CoreClient) {
  return {
    list: (projectId: string) => c.get<Microagent[]>(url`/projects/${projectId}/microagents`),

    get: (id: string) => c.get<Microagent>(url`/microagents/${id}`),

    create: (projectId: string, data: CreateMicroagentRequest) =>
      c.post<Microagent>(url`/projects/${projectId}/microagents`, data),

    update: (id: string, data: UpdateMicroagentRequest) =>
      c.put<Microagent>(url`/microagents/${id}`, data),

    delete: (id: string) => c.del<undefined>(url`/microagents/${id}`),
  };
}

export function createSkillsResource(c: CoreClient) {
  return {
    list: (projectId: string) => c.get<Skill[]>(url`/projects/${projectId}/skills`),

    get: (id: string) => c.get<Skill>(url`/skills/${id}`),

    create: (projectId: string, data: CreateSkillRequest) =>
      c.post<Skill>(url`/projects/${projectId}/skills`, data),

    update: (id: string, data: UpdateSkillRequest) => c.put<Skill>(url`/skills/${id}`, data),

    delete: (id: string) => c.del<undefined>(url`/skills/${id}`),

    import: (data: ImportSkillRequest) => c.post<Skill>("/skills/import", data),
  };
}
