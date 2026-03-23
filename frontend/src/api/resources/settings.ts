import type { CoreClient } from "../core";
import { url } from "../factory";
import type {
  AgentConfig,
  AppSettings,
  CreateKnowledgeBaseRequest,
  CreateModeRequest,
  CreateScopeRequest,
  CreateUserRequest,
  EvaluationResult,
  KnowledgeBase,
  Mode,
  PolicyProfile,
  PolicyToolCall,
  PromptPreviewRequest,
  PromptPreviewResponse,
  PromptSectionRow,
  RetrievalScope,
  RetrievalSearchResult,
  SearchRequest,
  UpdateUserRequest,
  User,
} from "../types";

export function createSettingsResource(c: CoreClient) {
  return {
    get: () => c.get<AppSettings>("/settings"),
    update: (data: { settings: AppSettings }) => c.put<{ status: string }>("/settings", data),
  };
}

export function createAgentConfigResource(c: CoreClient) {
  return {
    get: () => c.get<AgentConfig>("/agent-config"),
  };
}

export function createModesResource(c: CoreClient) {
  return {
    list: () => c.get<Mode[]>("/modes"),
    create: (data: CreateModeRequest) => c.post<Mode>("/modes", data),
    update: (id: string, data: CreateModeRequest) => c.put<Mode>(url`/modes/${id}`, data),
    delete: (id: string) => c.del<undefined>(url`/modes/${id}`),
    scenarios: () => c.get<string[]>("/modes/scenarios"),
    tools: () => c.get<string[]>("/modes/tools"),
    artifactTypes: () => c.get<string[]>("/modes/artifact-types"),
  };
}

export function createPoliciesResource(c: CoreClient) {
  return {
    list: () => c.get<{ profiles: string[] }>("/policies"),
    get: (name: string) => c.get<PolicyProfile>(url`/policies/${name}`),
    create: (profile: PolicyProfile) => c.post<PolicyProfile>("/policies", profile),
    delete: (name: string) => c.del<undefined>(url`/policies/${name}`),
    evaluate: (name: string, call: PolicyToolCall) =>
      c.post<EvaluationResult>(url`/policies/${name}/evaluate`, call),
    allowAlways: (projectId: string, tool: string, command?: string) =>
      c.post<PolicyProfile>("/policies/allow-always", {
        project_id: projectId,
        tool,
        command,
      }),
  };
}

export function createUsersResource(c: CoreClient) {
  return {
    list: () => c.get<User[]>("/users"),
    create: (data: CreateUserRequest) => c.post<User>("/users", data),
    update: (id: string, data: UpdateUserRequest) => c.put<User>(url`/users/${id}`, data),
    delete: (id: string) => c.del<undefined>(url`/users/${id}`),
  };
}

export function createScopesResource(c: CoreClient) {
  return {
    list: () => c.get<RetrievalScope[]>("/scopes"),
    create: (data: CreateScopeRequest) => c.post<RetrievalScope>("/scopes", data),
    delete: (id: string) => c.del<undefined>(url`/scopes/${id}`),
    addProject: (scopeId: string, projectId: string) =>
      c.post<undefined>(url`/scopes/${scopeId}/projects`, { project_id: projectId }),
    removeProject: (scopeId: string, projectId: string) =>
      c.del<undefined>(url`/scopes/${scopeId}/projects/${projectId}`),
    search: (scopeId: string, data: SearchRequest) =>
      c.post<RetrievalSearchResult>(url`/scopes/${scopeId}/search`, data),
  };
}

export function createKnowledgeBasesResource(c: CoreClient) {
  return {
    list: () => c.get<KnowledgeBase[]>("/knowledge-bases"),
    create: (data: CreateKnowledgeBaseRequest) => c.post<KnowledgeBase>("/knowledge-bases", data),
    delete: (id: string) => c.del<undefined>(url`/knowledge-bases/${id}`),
    index: (id: string) => c.post<{ status: string }>(url`/knowledge-bases/${id}/index`),
    listByScope: (scopeId: string) =>
      c.get<KnowledgeBase[]>(url`/scopes/${scopeId}/knowledge-bases`),
    attachToScope: (scopeId: string, kbId: string) =>
      c.post<undefined>(url`/scopes/${scopeId}/knowledge-bases`, { knowledge_base_id: kbId }),
    detachFromScope: (scopeId: string, kbId: string) =>
      c.del<undefined>(url`/scopes/${scopeId}/knowledge-bases/${kbId}`),
  };
}

export function createPromptSectionsResource(c: CoreClient) {
  return {
    list: (scope = "global") => c.get<PromptSectionRow[]>(url`/prompt-sections?scope=${scope}`),
    upsert: (data: PromptSectionRow) => c.put<PromptSectionRow>("/prompt-sections", data),
    delete: (id: string) => c.del<undefined>(url`/prompt-sections/${id}`),
    preview: (data: PromptPreviewRequest) =>
      c.post<PromptPreviewResponse>("/prompt-sections/preview", data),
  };
}
