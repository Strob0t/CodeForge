import { test as base } from "@playwright/test";

import { type AuthTokens, apiLogin, ADMIN_EMAIL, ADMIN_PASS } from "./helpers/api-helpers";

const API_BASE = "http://localhost:8080/api/v1";

// Cache admin auth per worker process to avoid repeated logins.
let cachedAdminAuth: AuthTokens | null = null;
async function getCachedAdminAuth(): Promise<AuthTokens> {
  if (!cachedAdminAuth) {
    cachedAdminAuth = await apiLogin(ADMIN_EMAIL, ADMIN_PASS);
  }
  return cachedAdminAuth;
}

// --- Types ---

interface Project {
  id: string;
  name: string;
}

interface Agent {
  id: string;
  name: string;
}

interface TaskEntity {
  id: string;
  title: string;
}

interface Team {
  id: string;
  name: string;
}

interface Mode {
  id: string;
  name: string;
}

interface MCPServer {
  id: string;
  name: string;
}

interface KnowledgeBase {
  id: string;
  name: string;
}

interface Scope {
  id: string;
  name: string;
}

interface Conversation {
  id: string;
}

// --- API Helper Interface ---

interface ApiHelper {
  // Auth
  login(email: string, password: string): Promise<AuthTokens>;
  getAdminToken(): Promise<string>;

  // Projects
  createProject(name: string, description?: string): Promise<Project>;
  deleteProject(id: string): Promise<void>;
  listProjects(): Promise<Project[]>;
  deleteAllProjects(): Promise<void>;

  // Agents
  createAgent(projectId: string, name: string, backend?: string): Promise<Agent>;

  // Tasks
  createTask(projectId: string, title: string, prompt?: string): Promise<TaskEntity>;

  // Teams
  createTeam(projectId: string, name: string, protocol?: string): Promise<Team>;

  // Modes
  createMode(data: Record<string, unknown>): Promise<Mode>;

  // MCP Servers
  createMCPServer(data: Record<string, unknown>): Promise<MCPServer>;

  // Knowledge Bases
  createKnowledgeBase(data: Record<string, unknown>): Promise<KnowledgeBase>;

  // Scopes
  createScope(data: Record<string, unknown>): Promise<Scope>;

  // Conversations
  createConversation(projectId: string): Promise<Conversation>;

  // Prompt Sections
  createPromptSection(scope: string, data: Record<string, unknown>): Promise<{ id: string }>;
}

// --- Test Fixture ---

export const test = base.extend<{ api: ApiHelper }>({
  // Override page fixture to inject auth into all browser API requests.
  // Two problems with storageState-only auth:
  //  1. The refresh cookie is one-time-use (atomic rotation), so stale after first use.
  //  2. SolidJS resources fire before the async refreshTokens() completes (race condition).
  // Fix: intercept all API calls — mock the refresh response AND inject Authorization
  // headers on every other API request, so auth works regardless of the async flow.
  page: async ({ page }, use) => {
    const auth = await getCachedAdminAuth();
    await page.route("**/api/v1/**", async (route, request) => {
      if (request.url().includes("/api/v1/auth/refresh")) {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            access_token: auth.accessToken,
            expires_in: 14400,
            user: auth.user,
          }),
        });
      } else {
        const headers = {
          ...request.headers(),
          authorization: `Bearer ${auth.accessToken}`,
        };
        await route.continue({ headers });
      }
    });
    await use(page);
  },

  api: async ({}, use) => {
    const createdIds: Array<{ type: string; id: string }> = [];
    let adminToken: string | null = null;

    async function getToken(): Promise<string> {
      if (!adminToken) {
        const auth = await apiLogin(ADMIN_EMAIL, ADMIN_PASS);
        adminToken = auth.accessToken;
      }
      return adminToken;
    }

    async function authedFetch(path: string, options: RequestInit = {}): Promise<Response> {
      const token = await getToken();
      const headers: Record<string, string> = {
        ...(options.headers as Record<string, string> | undefined),
        Authorization: `Bearer ${token}`,
      };
      if (options.body && typeof options.body === "string") {
        headers["Content-Type"] = "application/json";
      }
      return fetch(`${API_BASE}${path}`, { ...options, headers });
    }

    const helper: ApiHelper = {
      async login(email: string, password: string): Promise<AuthTokens> {
        return apiLogin(email, password);
      },

      async getAdminToken(): Promise<string> {
        return getToken();
      },

      async createProject(name: string, description = ""): Promise<Project> {
        const res = await authedFetch("/projects", {
          method: "POST",
          body: JSON.stringify({ name, description, repo_url: "", provider: "", config: {} }),
        });
        if (!res.ok) throw new Error(`Failed to create project: ${res.status}`);
        const project = (await res.json()) as Project;
        createdIds.push({ type: "project", id: project.id });
        return project;
      },

      async deleteProject(id: string): Promise<void> {
        await authedFetch(`/projects/${encodeURIComponent(id)}`, { method: "DELETE" });
      },

      async listProjects(): Promise<Project[]> {
        const res = await authedFetch("/projects");
        if (!res.ok) throw new Error(`Failed to list projects: ${res.status}`);
        return (await res.json()) as Project[];
      },

      async deleteAllProjects(): Promise<void> {
        const projects = await helper.listProjects();
        await Promise.all(projects.map((p) => helper.deleteProject(p.id)));
      },

      async createAgent(projectId: string, name: string, backend = "aider"): Promise<Agent> {
        const res = await authedFetch(`/projects/${projectId}/agents`, {
          method: "POST",
          body: JSON.stringify({ name, backend }),
        });
        if (!res.ok) throw new Error(`Failed to create agent: ${res.status}`);
        const agent = (await res.json()) as Agent;
        createdIds.push({ type: "agent", id: agent.id });
        return agent;
      },

      async createTask(
        projectId: string,
        title: string,
        prompt = "test prompt",
      ): Promise<TaskEntity> {
        const res = await authedFetch(`/projects/${projectId}/tasks`, {
          method: "POST",
          body: JSON.stringify({ title, prompt }),
        });
        if (!res.ok) throw new Error(`Failed to create task: ${res.status}`);
        const task = (await res.json()) as TaskEntity;
        createdIds.push({ type: "task", id: task.id });
        return task;
      },

      async createTeam(projectId: string, name: string, protocol = "sequential"): Promise<Team> {
        const res = await authedFetch(`/projects/${projectId}/teams`, {
          method: "POST",
          body: JSON.stringify({ name, protocol, members: [] }),
        });
        if (!res.ok) throw new Error(`Failed to create team: ${res.status}`);
        const team = (await res.json()) as Team;
        createdIds.push({ type: "team", id: team.id });
        return team;
      },

      async createMode(data: Record<string, unknown>): Promise<Mode> {
        const res = await authedFetch("/modes", {
          method: "POST",
          body: JSON.stringify(data),
        });
        if (!res.ok) throw new Error(`Failed to create mode: ${res.status}`);
        const mode = (await res.json()) as Mode;
        createdIds.push({ type: "mode", id: mode.id });
        return mode;
      },

      async createMCPServer(data: Record<string, unknown>): Promise<MCPServer> {
        const res = await authedFetch("/mcp/servers", {
          method: "POST",
          body: JSON.stringify(data),
        });
        if (!res.ok) throw new Error(`Failed to create MCP server: ${res.status}`);
        const server = (await res.json()) as MCPServer;
        createdIds.push({ type: "mcp-server", id: server.id });
        return server;
      },

      async createKnowledgeBase(data: Record<string, unknown>): Promise<KnowledgeBase> {
        const res = await authedFetch("/knowledge-bases", {
          method: "POST",
          body: JSON.stringify(data),
        });
        if (!res.ok) throw new Error(`Failed to create knowledge base: ${res.status}`);
        const kb = (await res.json()) as KnowledgeBase;
        createdIds.push({ type: "knowledge-base", id: kb.id });
        return kb;
      },

      async createScope(data: Record<string, unknown>): Promise<Scope> {
        const res = await authedFetch("/scopes", {
          method: "POST",
          body: JSON.stringify(data),
        });
        if (!res.ok) throw new Error(`Failed to create scope: ${res.status}`);
        const scope = (await res.json()) as Scope;
        createdIds.push({ type: "scope", id: scope.id });
        return scope;
      },

      async createConversation(projectId: string): Promise<Conversation> {
        const res = await authedFetch(`/projects/${projectId}/conversations`, {
          method: "POST",
          body: JSON.stringify({}),
        });
        if (!res.ok) throw new Error(`Failed to create conversation: ${res.status}`);
        const conv = (await res.json()) as Conversation;
        createdIds.push({ type: "conversation", id: conv.id });
        return conv;
      },

      async createPromptSection(
        scope: string,
        data: Record<string, unknown>,
      ): Promise<{ id: string }> {
        const res = await authedFetch("/prompt-sections", {
          method: "PUT",
          body: JSON.stringify({ scope, ...data }),
        });
        if (!res.ok) throw new Error(`Failed to create prompt section: ${res.status}`);
        const section = (await res.json()) as { id: string };
        createdIds.push({ type: "prompt-section", id: section.id });
        return section;
      },
    };

    await use(helper);

    // Teardown: clean up all created entities in reverse order
    for (const { type, id } of [...createdIds].reverse()) {
      try {
        switch (type) {
          case "project":
            await helper.deleteProject(id);
            break;
          case "agent":
            await authedFetch(`/agents/${id}`, { method: "DELETE" });
            break;
          case "team":
            await authedFetch(`/teams/${id}`, { method: "DELETE" });
            break;
          case "mode":
            await authedFetch(`/modes/${id}`, { method: "DELETE" });
            break;
          case "mcp-server":
            await authedFetch(`/mcp/servers/${id}`, { method: "DELETE" });
            break;
          case "knowledge-base":
            await authedFetch(`/knowledge-bases/${id}`, { method: "DELETE" });
            break;
          case "scope":
            await authedFetch(`/scopes/${id}`, { method: "DELETE" });
            break;
          case "conversation":
            await authedFetch(`/conversations/${id}`, { method: "DELETE" });
            break;
          case "prompt-section":
            await authedFetch(`/prompt-sections/${id}`, { method: "DELETE" });
            break;
        }
      } catch {
        // Best-effort cleanup
      }
    }
  },
});

export { expect } from "@playwright/test";
