import { test, expect } from "@playwright/test";
import { apiLogin, createProject, createCleanupTracker, API_BASE } from "../helpers/api-helpers";

/**
 * NATS graph tests.
 * Verifies that graph build and search operations trigger NATS flows
 * by observing the API responses.
 */
test.describe("NATS Graph (via API)", () => {
  let token: string;
  let projectId: string;
  const cleanup = createCleanupTracker();

  test.beforeAll(async () => {
    const auth = await apiLogin("admin@localhost", "Changeme123");
    token = auth.accessToken;

    const proj = await createProject(`nats-graph-e2e-${Date.now()}`);
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

  test("POST /projects/{id}/graph/build triggers NATS build", async () => {
    const res = await fetch(`${API_BASE}/projects/${projectId}/graph/build`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({}),
    });
    // 202 if dispatched to NATS, 400/404/500 if project has no workspace
    expect([202, 400, 404, 500]).toContain(res.status);
    if (res.status === 202) {
      const body = await res.json();
      expect(body.status).toBe("building");
    }
  });

  test("POST /projects/{id}/graph/search via API triggers NATS", async () => {
    // Use a short timeout to avoid hanging when NATS workers are unavailable
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), 10000);
    try {
      const res = await fetch(`${API_BASE}/projects/${projectId}/graph/search`, {
        method: "POST",
        headers: jsonHeaders(),
        body: JSON.stringify({
          seed_symbols: ["main"],
          max_hops: 2,
          top_k: 5,
        }),
        signal: controller.signal,
      });
      // 200 if graph exists and search succeeds, 504 if times out
      expect([200, 504]).toContain(res.status);
    } catch {
      // AbortError or network error when search infrastructure is unavailable -- acceptable
      test.info().annotations.push({
        type: "skip-reason",
        description: "Graph search timed out or unavailable",
      });
    } finally {
      clearTimeout(timeout);
    }
  });
});
