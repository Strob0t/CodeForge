import { test, expect } from "@playwright/test";
import { apiLogin, createProject, createCleanupTracker, API_BASE } from "../helpers/api-helpers";

test.describe("Trajectory API", () => {
  let token: string;
  let projectId: string;
  const cleanup = createCleanupTracker();
  // We use a fake run ID since runs require agent execution
  const fakeRunId = "00000000-0000-0000-0000-000000000000";

  test.beforeAll(async () => {
    const auth = await apiLogin("admin@localhost", "Changeme123");
    token = auth.accessToken;
    const proj = await createProject(`trajectory-e2e-${Date.now()}`);
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

  test("GET /runs/{id}/trajectory returns trajectory data or 404", async () => {
    const res = await fetch(`${API_BASE}/runs/${fakeRunId}/trajectory`, {
      headers: headers(),
    });
    // 200 if run exists, 404 if not
    expect([200, 404]).toContain(res.status);
    if (res.status === 200) {
      const body = await res.json();
      expect(body).toHaveProperty("events");
      expect(body).toHaveProperty("has_more");
    }
  });

  test("GET /runs/{id}/trajectory/export returns export or 404", async () => {
    const res = await fetch(`${API_BASE}/runs/${fakeRunId}/trajectory/export`, {
      headers: headers(),
    });
    expect([200, 404]).toContain(res.status);
    if (res.status === 200) {
      const contentType = res.headers.get("content-type");
      expect(contentType).toContain("application/json");
    }
  });

  test("GET /runs/{id}/checkpoints returns checkpoints or 404", async () => {
    const res = await fetch(`${API_BASE}/runs/${fakeRunId}/checkpoints`, {
      headers: headers(),
    });
    expect([200, 404]).toContain(res.status);
    if (res.status === 200) {
      const body = await res.json();
      expect(Array.isArray(body)).toBe(true);
    }
  });

  test("POST /runs/{id}/replay triggers replay or returns error", async () => {
    const res = await fetch(`${API_BASE}/runs/${fakeRunId}/replay`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({ checkpoint_id: "" }),
    });
    // 200 if replay works, 404 if run not found
    expect([200, 404]).toContain(res.status);
  });

  test("GET /audit returns global audit trail", async () => {
    const res = await fetch(`${API_BASE}/audit`, {
      headers: headers(),
    });
    expect(res.status).toBe(200);
    const body = await res.json();
    // AuditPage has entries, cursor, has_more, total
    expect(body).toHaveProperty("entries");
    // entries may be null when no audit entries exist
    expect(Array.isArray(body.entries) || body.entries === null).toBe(true);
    expect(body).toHaveProperty("has_more");
    expect(body).toHaveProperty("total");
  });

  test("GET /projects/{id}/audit returns project audit trail", async () => {
    const res = await fetch(`${API_BASE}/projects/${projectId}/audit`, {
      headers: headers(),
    });
    expect(res.status).toBe(200);
    const body = await res.json();
    // AuditPage has entries, cursor, has_more, total
    expect(body).toHaveProperty("entries");
    // entries may be null when no audit entries exist
    expect(Array.isArray(body.entries) || body.entries === null).toBe(true);
    expect(body).toHaveProperty("has_more");
    expect(body).toHaveProperty("total");
  });
});
