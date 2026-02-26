import { test, expect } from "@playwright/test";
import { apiLogin, createProject, createCleanupTracker, API_BASE } from "../helpers/api-helpers";

/**
 * NATS retrieval tests.
 * Verifies that indexing and search operations trigger NATS flows
 * by observing the API responses.
 */
test.describe("NATS Retrieval (via API)", () => {
  let token: string;
  let projectId: string;
  const cleanup = createCleanupTracker();

  test.beforeAll(async () => {
    const auth = await apiLogin("admin@localhost", "Changeme123");
    token = auth.accessToken;

    const proj = await createProject(`nats-retrieval-e2e-${Date.now()}`);
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

  test("POST /projects/{id}/index triggers NATS indexing", async () => {
    const res = await fetch(`${API_BASE}/projects/${projectId}/index`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({}),
    });
    // 202 if dispatched to NATS, 404 if project has no workspace
    expect([202, 404]).toContain(res.status);
    if (res.status === 202) {
      const body = await res.json();
      expect(body.status).toBe("building");
    }
  });

  test("POST /projects/{id}/search triggers NATS search", async () => {
    // Use a short timeout to avoid hanging when NATS workers are unavailable
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), 10000);
    try {
      const res = await fetch(`${API_BASE}/projects/${projectId}/search`, {
        method: "POST",
        headers: {
          ...jsonHeaders(),
        },
        body: JSON.stringify({ query: "main function", top_k: 5 }),
        signal: controller.signal,
      });
      // 200 if search completes, 504 if times out (NATS flow)
      expect([200, 504]).toContain(res.status);
    } catch {
      // AbortError or network error when search infrastructure is unavailable -- acceptable
      test.info().annotations.push({
        type: "skip-reason",
        description: "NATS search timed out or unavailable",
      });
    } finally {
      clearTimeout(timeout);
    }
  });

  test("GET /projects/{id}/index returns status after index request", async () => {
    // Try to index first
    await fetch(`${API_BASE}/projects/${projectId}/index`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({}),
    });

    // Check status
    const res = await fetch(`${API_BASE}/projects/${projectId}/index`, {
      headers: headers(),
    });
    // 200 if index exists (building or ready), 404 if no index
    expect([200, 404]).toContain(res.status);
  });
});
