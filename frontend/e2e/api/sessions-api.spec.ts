import { test, expect } from "@playwright/test";
import { apiLogin, createProject, createCleanupTracker, API_BASE } from "../helpers/api-helpers";

test.describe("Sessions API", () => {
  let token: string;
  let projectId: string;
  const cleanup = createCleanupTracker();
  const fakeRunId = "00000000-0000-0000-0000-000000000000";

  test.beforeAll(async () => {
    const auth = await apiLogin("admin@localhost", "Changeme123");
    token = auth.accessToken;
    const proj = await createProject(`sessions-e2e-${Date.now()}`);
    projectId = proj.id;
    cleanup.add("project", projectId);
  });

  test.afterAll(async () => {
    await cleanup.cleanup();
  });

  const headers = () => ({ Authorization: `Bearer ${token}` });
  const jsonHeaders = () => ({
    ...headers(),
    "Content-Type": "application/json",
  });

  test("POST /runs/{id}/resume resumes or returns 404", async () => {
    const res = await fetch(`${API_BASE}/runs/${fakeRunId}/resume`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({}),
    });
    // 201 if run exists, 404 if not found
    expect([201, 404]).toContain(res.status);
  });

  test("POST /runs/{id}/fork forks or returns 404", async () => {
    const res = await fetch(`${API_BASE}/runs/${fakeRunId}/fork`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({}),
    });
    expect([201, 404]).toContain(res.status);
  });

  test("POST /runs/{id}/rewind rewinds or returns 404", async () => {
    const res = await fetch(`${API_BASE}/runs/${fakeRunId}/rewind`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({}),
    });
    expect([201, 404]).toContain(res.status);
  });

  test("GET /projects/{id}/sessions returns sessions array", async () => {
    const res = await fetch(`${API_BASE}/projects/${projectId}/sessions`, {
      headers: headers(),
    });
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(Array.isArray(body)).toBe(true);
  });

  test("GET /sessions/{id} returns 404 for non-existent session", async () => {
    const fakeSessionId = "00000000-0000-0000-0000-000000000000";
    const res = await fetch(`${API_BASE}/sessions/${fakeSessionId}`, {
      headers: headers(),
    });
    expect(res.status).toBe(404);
  });
});
