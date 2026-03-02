import { test, expect } from "@playwright/test";
import { apiLogin, API_BASE } from "../helpers/api-helpers";

// Team API routes were removed in favor of orchestrator-managed teams.
// These tests verify the routes are no longer accessible (404).

test.describe("Teams API", () => {
  let token: string;

  test.beforeAll(async () => {
    const auth = await apiLogin("admin@localhost", "Changeme123");
    token = auth.accessToken;
  });

  const headers = () => ({ Authorization: `Bearer ${token}` });
  // Use a dummy project ID — we only care that the route itself is gone
  const dummyProjectId = "00000000-0000-0000-0000-000000000001";

  test("create team route returns 404 (removed)", async ({ request }) => {
    const res = await request.post(`${API_BASE}/projects/${dummyProjectId}/teams`, {
      headers: headers(),
      data: {
        name: `team-${Date.now()}`,
        protocol: "sequential",
        members: [],
      },
    });
    expect(res.status()).toBe(404);
  });

  test("list teams route returns 404 (removed)", async ({ request }) => {
    const res = await request.get(`${API_BASE}/projects/${dummyProjectId}/teams`, {
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
