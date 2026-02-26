import { test, expect } from "@playwright/test";
import {
  apiLogin,
  createProject,
  createAgent,
  createCleanupTracker,
  API_BASE,
} from "../helpers/api-helpers";

test.describe("Teams API", () => {
  let token: string;
  const cleanup = createCleanupTracker();

  test.beforeAll(async () => {
    const auth = await apiLogin("admin@localhost", "Changeme123");
    token = auth.accessToken;
  });

  test.afterAll(async () => {
    await cleanup.cleanup();
  });

  const headers = () => ({ Authorization: `Bearer ${token}` });

  /**
   * Helper: create a project + agent, then create a team with that agent as a member.
   * The backend requires at least one member with a valid agent_id and role.
   */
  async function createTeamWithAgent(
    projName: string,
    teamName: string,
  ): Promise<{ projId: string; agentId: string; teamId: string }> {
    const proj = await createProject(projName);
    cleanup.add("project", proj.id);

    const agent = await createAgent(proj.id, `${teamName}-agent`);
    cleanup.add("agent", agent.id);

    const res = await fetch(`${API_BASE}/projects/${proj.id}/teams`, {
      method: "POST",
      headers: { ...headers(), "Content-Type": "application/json" },
      body: JSON.stringify({
        name: teamName,
        protocol: "sequential",
        members: [{ agent_id: agent.id, role: "coder" }],
      }),
    });
    const body = await res.json();
    return { projId: proj.id, agentId: agent.id, teamId: body.id };
  }

  test("create team returns 201", async ({ request }) => {
    const proj = await createProject(`e2e-team-proj-${Date.now()}`);
    cleanup.add("project", proj.id);

    const agent = await createAgent(proj.id, `team-member-${Date.now()}`);
    cleanup.add("agent", agent.id);

    const res = await request.post(`${API_BASE}/projects/${proj.id}/teams`, {
      headers: headers(),
      data: {
        name: `team-${Date.now()}`,
        protocol: "sequential",
        members: [{ agent_id: agent.id, role: "coder" }],
      },
    });
    expect(res.status()).toBe(201);
    const body = await res.json();
    expect(body.id).toBeTruthy();
    expect(body.name).toBeTruthy();
    cleanup.add("team", body.id);
  });

  test("list teams returns array", async ({ request }) => {
    const { projId, teamId } = await createTeamWithAgent(
      `e2e-list-teams-${Date.now()}`,
      `team-list-${Date.now()}`,
    );
    cleanup.add("team", teamId);

    const res = await request.get(`${API_BASE}/projects/${projId}/teams`, {
      headers: headers(),
    });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(Array.isArray(body)).toBe(true);
    expect(body.length).toBeGreaterThanOrEqual(1);
  });

  test("get team by ID", async ({ request }) => {
    const { teamId } = await createTeamWithAgent(
      `e2e-get-team-${Date.now()}`,
      `team-get-${Date.now()}`,
    );
    cleanup.add("team", teamId);

    const res = await request.get(`${API_BASE}/teams/${teamId}`, { headers: headers() });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(body.id).toBe(teamId);
  });

  test("delete team returns 204", async ({ request }) => {
    const { teamId } = await createTeamWithAgent(
      `e2e-del-team-${Date.now()}`,
      `team-del-${Date.now()}`,
    );

    const res = await request.delete(`${API_BASE}/teams/${teamId}`, { headers: headers() });
    expect(res.status()).toBe(204);
  });

  test("get shared context for team", async ({ request }) => {
    const { teamId } = await createTeamWithAgent(
      `e2e-sc-team-${Date.now()}`,
      `team-sc-${Date.now()}`,
    );
    cleanup.add("team", teamId);

    const res = await request.get(`${API_BASE}/teams/${teamId}/shared-context`, {
      headers: headers(),
    });
    // 200 or 404 if no shared context exists yet
    expect([200, 404]).toContain(res.status());
  });

  test("add shared context item", async ({ request }) => {
    const { teamId } = await createTeamWithAgent(
      `e2e-add-sc-${Date.now()}`,
      `team-add-sc-${Date.now()}`,
    );
    cleanup.add("team", teamId);

    const res = await request.post(`${API_BASE}/teams/${teamId}/shared-context`, {
      headers: headers(),
      data: { key: "test_key", value: "test_value", source: "e2e-test" },
    });
    expect([201, 400]).toContain(res.status());
  });

  test("delete non-existent team returns 404", async ({ request }) => {
    const res = await request.delete(`${API_BASE}/teams/00000000-0000-0000-0000-000000000000`, {
      headers: headers(),
    });
    expect(res.status()).toBe(404);
  });

  test("create team validation requires name", async ({ request }) => {
    const proj = await createProject(`e2e-val-team-${Date.now()}`);
    cleanup.add("project", proj.id);

    const agent = await createAgent(proj.id, `val-agent-${Date.now()}`);
    cleanup.add("agent", agent.id);

    const res = await request.post(`${API_BASE}/projects/${proj.id}/teams`, {
      headers: headers(),
      data: {
        protocol: "sequential",
        members: [{ agent_id: agent.id, role: "coder" }],
      },
    });
    expect(res.status()).toBe(400);
  });

  test("create team validation requires at least one member", async ({ request }) => {
    const proj = await createProject(`e2e-val-members-${Date.now()}`);
    cleanup.add("project", proj.id);

    const res = await request.post(`${API_BASE}/projects/${proj.id}/teams`, {
      headers: headers(),
      data: {
        name: `team-no-members-${Date.now()}`,
        protocol: "sequential",
        members: [],
      },
    });
    expect(res.status()).toBe(400);
  });
});
