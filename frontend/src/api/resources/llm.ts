import type { CoreClient } from "../core";
import { url } from "../factory";
import type {
  AddModelRequest,
  CostSummary,
  DailyCost,
  DiscoverModelsResponse,
  LLMModel,
  ModelCostSummary,
  ProjectCostSummary,
  Run,
  ToolCostSummary,
} from "../types";

export function createLLMResource(c: CoreClient) {
  return {
    models: () => c.get<LLMModel[]>("/llm/models"),

    addModel: (data: AddModelRequest) =>
      c.post<undefined>("/llm/models", data).then((r) => {
        c.invalidateCache("/llm");
        return r;
      }),

    deleteModel: (modelId: string) =>
      c.del<undefined>(url`/llm/models/${modelId}`).then((r) => {
        c.invalidateCache("/llm");
        return r;
      }),

    health: () => c.get<{ status: string }>("/llm/health"),

    discover: () => c.get<DiscoverModelsResponse>("/llm/discover"),
  };
}

export function createCostsResource(c: CoreClient) {
  return {
    global: () => c.get<ProjectCostSummary[]>("/costs"),

    project: (id: string) => c.get<CostSummary>(url`/projects/${id}/costs`),

    byModel: (id: string) => c.get<ModelCostSummary[]>(url`/projects/${id}/costs/by-model`),

    daily: (id: string, days = 30) =>
      c.get<DailyCost[]>(url`/projects/${id}/costs/daily?days=${days}`),

    recentRuns: (id: string, limit = 20) =>
      c.get<Run[]>(url`/projects/${id}/costs/runs?limit=${limit}`),

    byTool: (id: string) => c.get<ToolCostSummary[]>(url`/projects/${id}/costs/by-tool`),
  };
}

export function createProvidersResource(c: CoreClient) {
  return {
    git: () => c.get<import("../types").ProviderList>("/providers/git"),
    agent: () => c.get<import("../types").BackendList>("/providers/agent"),
    spec: () => c.get<import("../types").ProviderInfo[]>("/providers/spec"),
    pm: () => c.get<import("../types").ProviderInfo[]>("/providers/pm"),
  };
}
