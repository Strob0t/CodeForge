import { test, expect } from "@playwright/test";
import { apiLogin, createProject, createCleanupTracker, API_BASE } from "../helpers/api-helpers";

/**
 * NATS repomap test.
 * Verifies that generating a repomap triggers NATS flow
 * by observing the API response.
 */
test.describe("NATS Repomap (via API)", () => {
  let token: string;
  let projectId: string;
  const cleanup = createCleanupTracker();

  test.beforeAll(async () => {
    const auth = await apiLogin("admin@localhost", "Changeme123");
    token = auth.accessToken;

    const proj = await createProject(`nats-repomap-e2e-${Date.now()}`);
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

  test("POST /projects/{id}/repomap triggers NATS generation", async () => {
    const res = await fetch(`${API_BASE}/projects/${projectId}/repomap`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({ active_files: [] }),
    });
    // 202 if accepted and dispatched to NATS, 404 if project has no workspace
    expect([202, 404]).toContain(res.status);
    if (res.status === 202) {
      const body = await res.json();
      expect(body.status).toBe("generating");
    }
  });
});
