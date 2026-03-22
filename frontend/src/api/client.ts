import { createCoreClient, FetchError, getAccessToken, setAccessTokenGetter } from "./core";
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
import {
  createActiveWorkResource,
  createAuditResource,
  createAutoAgentResource,
  createChannelsResource,
  createDashboardResource,
  createDevResource,
  createGoalsResource,
  createGraphResource,
  createHealthResource,
  createLSPResource,
  createMCPResource,
  createPlansResource,
  createRepomapResource,
  createRetrievalResource,
  createRunsResource,
  createSearchResource,
  createSessionsResource,
  createTrajectoryResource,
} from "./resources/misc";
import { createBatchResource, createProjectsResource } from "./resources/projects";
import { createRoadmapResource } from "./resources/roadmap";
import {
  createAgentConfigResource,
  createKnowledgeBasesResource,
  createModesResource,
  createPoliciesResource,
  createPromptSectionsResource,
  createReviewsResource,
  createScopesResource,
  createSettingsResource,
  createUsersResource,
} from "./resources/settings";

export { FetchError, getAccessToken, setAccessTokenGetter };

const core = createCoreClient();

export const api = {
  health: createHealthResource(),
  projects: createProjectsResource(core),
  batch: createBatchResource(core),
  agents: createAgentsResource(core),
  tasks: createTasksResource(core),
  llm: createLLMResource(core),
  runs: createRunsResource(core),
  sessions: createSessionsResource(core),
  plans: createPlansResource(core),
  modes: createModesResource(core),
  repomap: createRepomapResource(core),
  retrieval: createRetrievalResource(core),
  graph: createGraphResource(core),
  policies: createPoliciesResource(core),
  costs: createCostsResource(core),
  dashboard: createDashboardResource(core),
  roadmap: createRoadmapResource(core),
  trajectory: createTrajectoryResource(core),
  search: createSearchResource(core),
  providers: createProvidersResource(core),
  auth: createAuthResource(core),
  users: createUsersResource(core),
  reviews: createReviewsResource(core),
  scopes: createScopesResource(core),
  knowledgeBases: createKnowledgeBasesResource(core),
  settings: createSettingsResource(core),
  agentConfig: createAgentConfigResource(core),
  conversations: createConversationsResource(core),
  vcsAccounts: createVCSAccountsResource(core),
  lsp: createLSPResource(core),
  dev: createDevResource(core),
  mcp: createMCPResource(core),
  promptSections: createPromptSectionsResource(core),
  benchmarks: createBenchmarksResource(core),
  files: createFilesResource(core),
  autoAgent: createAutoAgentResource(core),
  activeWork: createActiveWorkResource(core),
  goals: createGoalsResource(core),
  channels: createChannelsResource(core),
  subscriptionProviders: createSubscriptionProvidersResource(core),
  audit: createAuditResource(core),
};
