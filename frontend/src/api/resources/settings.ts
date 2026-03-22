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
  GraphSearchRequest,
  GraphSearchResult,
  KnowledgeBase,
  Mode,
  PolicyProfile,
  PolicyToolCall,
  PromptPreviewRequest,
  PromptPreviewResponse,
  PromptSectionRow,
  RetrievalScope,
  RetrievalSearchResult,
  Review,
  ReviewPolicy,
  SearchRequest,
  UpdateScopeRequest,
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
    get: (id: string) => c.get<Mode>(url`/modes/${id}`),
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

export function createReviewsResource(c: CoreClient) {
  return {
    listPolicies: (projectId: string) =>
      c.get<ReviewPolicy[]>(url`/projects/${projectId}/review-policies`),

    createPolicy: (projectId: string, data: Partial<ReviewPolicy>) =>
      c.post<ReviewPolicy>(url`/projects/${projectId}/review-policies`, data),

    getPolicy: (id: string) => c.get<ReviewPolicy>(url`/review-policies/${id}`),

    updatePolicy: (id: string, data: Partial<ReviewPolicy>) =>
      c.put<ReviewPolicy>(url`/review-policies/${id}`, data),

    deletePolicy: (id: string) => c.del<undefined>(url`/review-policies/${id}`),

    trigger: (policyId: string) => c.post<Review>(url`/review-policies/${policyId}/trigger`),

    list: (projectId: string) => c.get<Review[]>(url`/projects/${projectId}/reviews`),

    get: (id: string) => c.get<Review>(url`/reviews/${id}`),
  };
}

export function createScopesResource(c: CoreClient) {
  return {
    list: () => c.get<RetrievalScope[]>("/scopes"),
    get: (id: string) => c.get<RetrievalScope>(url`/scopes/${id}`),
    create: (data: CreateScopeRequest) => c.post<RetrievalScope>("/scopes", data),
    update: (id: string, data: UpdateScopeRequest) =>
      c.put<RetrievalScope>(url`/scopes/${id}`, data),
    delete: (id: string) => c.del<undefined>(url`/scopes/${id}`),
    addProject: (scopeId: string, projectId: string) =>
      c.post<undefined>(url`/scopes/${scopeId}/projects`, { project_id: projectId }),
    removeProject: (scopeId: string, projectId: string) =>
      c.del<undefined>(url`/scopes/${scopeId}/projects/${projectId}`),
    search: (scopeId: string, data: SearchRequest) =>
      c.post<RetrievalSearchResult>(url`/scopes/${scopeId}/search`, data),
    graphSearch: (scopeId: string, data: GraphSearchRequest) =>
      c.post<GraphSearchResult>(url`/scopes/${scopeId}/graph/search`, data),
  };
}

export function createKnowledgeBasesResource(c: CoreClient) {
  return {
    list: () => c.get<KnowledgeBase[]>("/knowledge-bases"),
    get: (id: string) => c.get<KnowledgeBase>(url`/knowledge-bases/${id}`),
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
