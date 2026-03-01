import { test, expect } from "@playwright/test";
import {
  apiLogin,
  createProject,
  createAgent,
  createCleanupTracker,
  API_BASE,
} from "../helpers/api-helpers";

// Team API routes were removed in favor of orchestrator-managed teams.
// These tests verify the routes are no longer accessible (404).

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

  test("create team route returns 404 (removed)", async ({ request }) => {
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
    expect(res.status()).toBe(404);
  });

  test("list teams route returns 404 (removed)", async ({ request }) => {
    const proj = await createProject(`e2e-list-teams-${Date.now()}`);
    cleanup.add("project", proj.id);

    const res = await request.get(`${API_BASE}/projects/${proj.id}/teams`, {
      headers: headers(),
    });
    expect(res.status()).toBe(404);
  });

  test("get team by ID returns 404 (removed)", async ({ request }) => {
    const res = await request.get(`${API_BASE}/teams/00000000-0000-0000-0000-000000000000`, {
      headers: headers(),
    });
    expect(res.status()).toBe(404);
  });

  test("delete team returns 404 (removed)", async ({ request }) => {
    const res = await request.delete(`${API_BASE}/teams/00000000-0000-0000-0000-000000000000`, {
      headers: headers(),
    });
    expect(res.status()).toBe(404);
  });
});
