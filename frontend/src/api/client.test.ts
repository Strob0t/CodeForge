import { describe, expect, it } from "vitest";

import { api, FetchError, getAccessToken, setAccessTokenGetter } from "./client";
import type { CoreClient } from "./core";

describe("API Client", () => {
  describe("exports", () => {
    it("should export api object", () => {
      expect(api).toBeDefined();
      expect(typeof api).toBe("object");
    });

    it("should export FetchError class", () => {
      expect(FetchError).toBeDefined();
      expect(typeof FetchError).toBe("function");
    });

    it("should export setAccessTokenGetter function", () => {
      expect(typeof setAccessTokenGetter).toBe("function");
    });

    it("should export getAccessToken function", () => {
      expect(typeof getAccessToken).toBe("function");
    });
  });

  describe("api resource groups", () => {
    it("should have health resource group", () => {
      expect(api.health).toBeDefined();
      expect(typeof api.health.check).toBe("function");
    });

    it("should have projects resource group", () => {
      expect(api.projects).toBeDefined();
      expect(typeof api.projects.list).toBe("function");
      expect(typeof api.projects.get).toBe("function");
      expect(typeof api.projects.create).toBe("function");
      expect(typeof api.projects.update).toBe("function");
      expect(typeof api.projects.delete).toBe("function");
    });

    it("should have agents resource group", () => {
      expect(api.agents).toBeDefined();
      expect(typeof api.agents.list).toBe("function");
      expect(typeof api.agents.get).toBe("function");
      expect(typeof api.agents.create).toBe("function");
    });

    it("should have tasks resource group", () => {
      expect(api.tasks).toBeDefined();
      expect(typeof api.tasks.list).toBe("function");
      expect(typeof api.tasks.create).toBe("function");
    });

    it("should have llm resource group", () => {
      expect(api.llm).toBeDefined();
      expect(typeof api.llm.models).toBe("function");
      expect(typeof api.llm.addModel).toBe("function");
      expect(typeof api.llm.health).toBe("function");
    });

    it("should have runs resource group", () => {
      expect(api.runs).toBeDefined();
      expect(typeof api.runs.start).toBe("function");
      expect(typeof api.runs.get).toBe("function");
      expect(typeof api.runs.cancel).toBe("function");
      expect(typeof api.runs.approve).toBe("function");
    });

    it("should have conversations resource group", () => {
      expect(api.conversations).toBeDefined();
      expect(typeof api.conversations.create).toBe("function");
      expect(typeof api.conversations.list).toBe("function");
      expect(typeof api.conversations.messages).toBe("function");
      expect(typeof api.conversations.send).toBe("function");
    });

    it("should have costs resource group", () => {
      expect(api.costs).toBeDefined();
      expect(typeof api.costs.global).toBe("function");
      expect(typeof api.costs.project).toBe("function");
      expect(typeof api.costs.daily).toBe("function");
    });

    it("should have dashboard resource group", () => {
      expect(api.dashboard).toBeDefined();
      expect(typeof api.dashboard.stats).toBe("function");
      expect(typeof api.dashboard.projectHealth).toBe("function");
    });

    it("should have auth resource group", () => {
      expect(api.auth).toBeDefined();
      expect(typeof api.auth.login).toBe("function");
      expect(typeof api.auth.refresh).toBe("function");
      expect(typeof api.auth.logout).toBe("function");
      expect(typeof api.auth.me).toBe("function");
      expect(typeof api.auth.setup).toBe("function");
    });

    it("should have modes resource group", () => {
      expect(api.modes).toBeDefined();
      expect(typeof api.modes.list).toBe("function");
      expect(typeof api.modes.create).toBe("function");
    });

    it("should have policies resource group", () => {
      expect(api.policies).toBeDefined();
      expect(typeof api.policies.list).toBe("function");
      expect(typeof api.policies.evaluate).toBe("function");
    });

    it("should have roadmap resource group", () => {
      expect(api.roadmap).toBeDefined();
      expect(typeof api.roadmap.get).toBe("function");
      expect(typeof api.roadmap.create).toBe("function");
    });

    it("should have mcp resource group", () => {
      expect(api.mcp).toBeDefined();
      expect(typeof api.mcp.listServers).toBe("function");
      expect(typeof api.mcp.createServer).toBe("function");
    });

    it("should have benchmarks resource group", () => {
      expect(api.benchmarks).toBeDefined();
      expect(typeof api.benchmarks.listRuns).toBe("function");
      expect(typeof api.benchmarks.createRun).toBe("function");
    });

    it("should have channels resource group", () => {
      expect(api.channels).toBeDefined();
      expect(typeof api.channels.list).toBe("function");
      expect(typeof api.channels.send).toBe("function");
    });

    it("should have files resource group", () => {
      expect(api.files).toBeDefined();
      expect(typeof api.files.list).toBe("function");
      expect(typeof api.files.read).toBe("function");
      expect(typeof api.files.write).toBe("function");
    });

    it("should have audit resource group", () => {
      expect(api.audit).toBeDefined();
      expect(typeof api.audit.list).toBe("function");
    });

    it("should have routing resource group", () => {
      expect(api.routing).toBeDefined();
      expect(typeof api.routing.stats).toBe("function");
      expect(typeof api.routing.refreshStats).toBe("function");
      expect(typeof api.routing.outcomes).toBe("function");
      expect(typeof api.routing.recordOutcome).toBe("function");
      expect(typeof api.routing.seedFromBenchmarks).toBe("function");
    });

    it("should have scopes resource group", () => {
      expect(api.scopes).toBeDefined();
      expect(typeof api.scopes.list).toBe("function");
      expect(typeof api.scopes.create).toBe("function");
    });

    it("should have knowledgeBases resource group", () => {
      expect(api.knowledgeBases).toBeDefined();
      expect(typeof api.knowledgeBases.list).toBe("function");
      expect(typeof api.knowledgeBases.create).toBe("function");
    });

    it("should have settings resource group", () => {
      expect(api.settings).toBeDefined();
      expect(typeof api.settings.get).toBe("function");
      expect(typeof api.settings.update).toBe("function");
    });

    it("should have users resource group", () => {
      expect(api.users).toBeDefined();
      expect(typeof api.users.list).toBe("function");
      expect(typeof api.users.create).toBe("function");
    });
  });

  describe("FetchError", () => {
    it("should create error with status and body", () => {
      const err = new FetchError(404, { error: "Not found" });
      expect(err.status).toBe(404);
      expect(err.body).toEqual({ error: "Not found" });
      expect(err.message).toBe("Not found");
      expect(err.name).toBe("FetchError");
    });

    it("should be an instance of Error", () => {
      const err = new FetchError(500, { error: "Server error" });
      expect(err).toBeInstanceOf(Error);
    });
  });

  describe("access token management", () => {
    it("should return null when no token getter is set", () => {
      setAccessTokenGetter(null as unknown as () => string | null);
      // getAccessToken uses optional chaining so this is safe
      expect(getAccessToken()).toBeNull();
    });

    it("should return token from getter function", () => {
      setAccessTokenGetter(() => "test-token-123");
      expect(getAccessToken()).toBe("test-token-123");
    });

    it("should return null when getter returns null", () => {
      setAccessTokenGetter(() => null);
      expect(getAccessToken()).toBeNull();
    });
  });
});

describe("API Core", () => {
  it("should export FetchError from core module", async () => {
    const mod = await import("./core");
    expect(mod.FetchError).toBeDefined();
    expect(typeof mod.FetchError).toBe("function");
  });

  it("should export createCoreClient from core module", async () => {
    const mod = await import("./core");
    expect(typeof mod.createCoreClient).toBe("function");
  });

  it("should create core client with expected methods", async () => {
    const mod = await import("./core");
    const client: CoreClient = mod.createCoreClient();
    expect(typeof client.get).toBe("function");
    expect(typeof client.post).toBe("function");
    expect(typeof client.put).toBe("function");
    expect(typeof client.patch).toBe("function");
    expect(typeof client.del).toBe("function");
    expect(typeof client.request).toBe("function");
    expect(typeof client.invalidateCache).toBe("function");
    expect(client.BASE).toBe("/api/v1");
  });

  it("should export setAccessTokenGetter and getAccessToken from core", async () => {
    const mod = await import("./core");
    expect(typeof mod.setAccessTokenGetter).toBe("function");
    expect(typeof mod.getAccessToken).toBe("function");
  });
});

describe("API Resources index", () => {
  it("should export all resource factories", async () => {
    const mod = await import("./resources");
    expect(typeof mod.createProjectsResource).toBe("function");
    expect(typeof mod.createBatchResource).toBe("function");
    expect(typeof mod.createConversationsResource).toBe("function");
    expect(typeof mod.createAuthResource).toBe("function");
    expect(typeof mod.createVCSAccountsResource).toBe("function");
    expect(typeof mod.createSubscriptionProvidersResource).toBe("function");
    expect(typeof mod.createFilesResource).toBe("function");
    expect(typeof mod.createAgentsResource).toBe("function");
    expect(typeof mod.createTasksResource).toBe("function");
    expect(typeof mod.createLLMResource).toBe("function");
    expect(typeof mod.createCostsResource).toBe("function");
    expect(typeof mod.createProvidersResource).toBe("function");
    expect(typeof mod.createBenchmarksResource).toBe("function");
    expect(typeof mod.createRoadmapResource).toBe("function");
    expect(typeof mod.createSettingsResource).toBe("function");
    expect(typeof mod.createAgentConfigResource).toBe("function");
    expect(typeof mod.createModesResource).toBe("function");
    expect(typeof mod.createPoliciesResource).toBe("function");
    expect(typeof mod.createUsersResource).toBe("function");
    expect(typeof mod.createReviewsResource).toBe("function");
    expect(typeof mod.createScopesResource).toBe("function");
    expect(typeof mod.createKnowledgeBasesResource).toBe("function");
    expect(typeof mod.createPromptSectionsResource).toBe("function");
    expect(typeof mod.createHealthResource).toBe("function");
    expect(typeof mod.createRunsResource).toBe("function");
    expect(typeof mod.createSessionsResource).toBe("function");
    expect(typeof mod.createPlansResource).toBe("function");
    expect(typeof mod.createRepomapResource).toBe("function");
    expect(typeof mod.createRetrievalResource).toBe("function");
    expect(typeof mod.createGraphResource).toBe("function");
    expect(typeof mod.createDashboardResource).toBe("function");
    expect(typeof mod.createTrajectoryResource).toBe("function");
    expect(typeof mod.createSearchResource).toBe("function");
    expect(typeof mod.createLSPResource).toBe("function");
    expect(typeof mod.createDevResource).toBe("function");
    expect(typeof mod.createMCPResource).toBe("function");
    expect(typeof mod.createAutoAgentResource).toBe("function");
    expect(typeof mod.createActiveWorkResource).toBe("function");
    expect(typeof mod.createGoalsResource).toBe("function");
    expect(typeof mod.createChannelsResource).toBe("function");
    expect(typeof mod.createAuditResource).toBe("function");
    expect(typeof mod.createRoutingResource).toBe("function");
  });
});
