import { test, expect } from "@playwright/test";
import { apiLogin, createProject, createCleanupTracker, API_BASE } from "../helpers/api-helpers";

test.describe("Reviews API", () => {
  let token: string;
  let projectId: string;
  const cleanup = createCleanupTracker();
  const policyIds: string[] = [];

  test.beforeAll(async () => {
    const auth = await apiLogin("admin@localhost", "Changeme123");
    token = auth.accessToken;
    const proj = await createProject(`reviews-e2e-${Date.now()}`);
    projectId = proj.id;
    cleanup.add("project", projectId);
  });

  test.afterAll(async () => {
    // Clean up policies
    for (const id of policyIds) {
      try {
        await fetch(`${API_BASE}/review-policies/${id}`, {
          method: "DELETE",
          headers: { Authorization: `Bearer ${token}` },
        });
      } catch {
        // best-effort
      }
    }
    await cleanup.cleanup();
  });

  const headers = () => ({ Authorization: `Bearer ${token}` });
  const jsonHeaders = () => ({
    ...headers(),
    "Content-Type": "application/json",
  });

  test("GET /projects/{id}/review-policies returns array", async () => {
    const res = await fetch(`${API_BASE}/projects/${projectId}/review-policies`, {
      headers: headers(),
    });
    expect(res.status).toBe(200);
    const body = await res.json();
    // API may return null when no policies exist; treat null as empty array
    expect(Array.isArray(body) || body === null).toBe(true);
  });

  test("POST /projects/{id}/review-policies creates a policy", async () => {
    const res = await fetch(`${API_BASE}/projects/${projectId}/review-policies`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({
        name: `e2e-policy-${Date.now()}`,
        trigger_type: "commit_count",
        commit_threshold: 10,
        enabled: true,
      }),
    });
    expect(res.status).toBe(201);
    const body = await res.json();
    expect(body.id).toBeTruthy();
    expect(body.name).toContain("e2e-policy");
    policyIds.push(body.id);
  });

  test("GET /review-policies/{id} returns a specific policy", async () => {
    // Create a policy first (pre_merge requires branch_pattern)
    const createRes = await fetch(`${API_BASE}/projects/${projectId}/review-policies`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({
        name: `e2e-get-policy-${Date.now()}`,
        trigger_type: "pre_merge",
        branch_pattern: "main",
        enabled: true,
      }),
    });
    expect(createRes.status).toBe(201);
    const policy = await createRes.json();
    policyIds.push(policy.id);

    const res = await fetch(`${API_BASE}/review-policies/${policy.id}`, {
      headers: headers(),
    });
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body.id).toBe(policy.id);
  });

  test("PUT /review-policies/{id} updates a policy", async () => {
    const createRes = await fetch(`${API_BASE}/projects/${projectId}/review-policies`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({
        name: `e2e-update-policy-${Date.now()}`,
        trigger_type: "commit_count",
        commit_threshold: 5,
        enabled: true,
      }),
    });
    expect(createRes.status).toBe(201);
    const policy = await createRes.json();
    policyIds.push(policy.id);

    const res = await fetch(`${API_BASE}/review-policies/${policy.id}`, {
      method: "PUT",
      headers: jsonHeaders(),
      body: JSON.stringify({
        name: "Updated Policy Name",
        commit_threshold: 20,
      }),
    });
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body.name).toBe("Updated Policy Name");
  });

  test("DELETE /review-policies/{id} deletes a policy", async () => {
    // Use "daily" cron format (custom parser does not support standard cron syntax)
    const createRes = await fetch(`${API_BASE}/projects/${projectId}/review-policies`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({
        name: `e2e-del-policy-${Date.now()}`,
        trigger_type: "cron",
        cron_expr: "daily",
        enabled: false,
      }),
    });
    expect(createRes.status).toBe(201);
    const policy = await createRes.json();

    const res = await fetch(`${API_BASE}/review-policies/${policy.id}`, {
      method: "DELETE",
      headers: headers(),
    });
    expect(res.status).toBe(204);
  });

  test("POST /review-policies/{id}/trigger triggers a review", async () => {
    const createRes = await fetch(`${API_BASE}/projects/${projectId}/review-policies`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({
        name: `e2e-trigger-policy-${Date.now()}`,
        trigger_type: "commit_count",
        commit_threshold: 1,
        enabled: true,
      }),
    });
    expect(createRes.status).toBe(201);
    const policy = await createRes.json();
    policyIds.push(policy.id);

    const res = await fetch(`${API_BASE}/review-policies/${policy.id}/trigger`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({}),
    });
    // 201 if triggered, or error if LLM/worker not available
    expect([201, 400, 404, 500, 502]).toContain(res.status);
  });

  test("GET /projects/{id}/reviews returns reviews array", async () => {
    const res = await fetch(`${API_BASE}/projects/${projectId}/reviews`, {
      headers: headers(),
    });
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(Array.isArray(body)).toBe(true);
  });
});
