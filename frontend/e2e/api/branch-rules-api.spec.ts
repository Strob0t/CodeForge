import { test, expect } from "@playwright/test";
import { apiLogin, createProject, createCleanupTracker, API_BASE } from "../helpers/api-helpers";

test.describe("Branch Rules API", () => {
  let token: string;
  let projectId: string;
  const cleanup = createCleanupTracker();
  const ruleIds: string[] = [];

  test.beforeAll(async () => {
    const auth = await apiLogin("admin@localhost", "Changeme123");
    token = auth.accessToken;
    const proj = await createProject(`branch-rules-e2e-${Date.now()}`);
    projectId = proj.id;
    cleanup.add("project", projectId);
  });

  test.afterAll(async () => {
    for (const id of ruleIds) {
      try {
        await fetch(`${API_BASE}/branch-rules/${id}`, {
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

  test("POST /projects/{id}/branch-rules creates a rule", async () => {
    const res = await fetch(`${API_BASE}/projects/${projectId}/branch-rules`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({
        branch_pattern: "main",
        require_reviews: true,
        require_tests: true,
        require_lint: false,
        allow_force_push: false,
        allow_delete: false,
        enabled: true,
      }),
    });
    expect(res.status).toBe(201);
    const body = await res.json();
    expect(body.id).toBeTruthy();
    expect(body.branch_pattern).toBe("main");
    ruleIds.push(body.id);
  });

  test("GET /projects/{id}/branch-rules lists rules", async () => {
    const res = await fetch(`${API_BASE}/projects/${projectId}/branch-rules`, {
      headers: headers(),
    });
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(Array.isArray(body)).toBe(true);
    expect(body.length).toBeGreaterThan(0);
  });

  test("GET /branch-rules/{id} returns a specific rule", async () => {
    const createRes = await fetch(`${API_BASE}/projects/${projectId}/branch-rules`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({
        branch_pattern: "develop",
        require_reviews: false,
        require_tests: true,
        require_lint: true,
        allow_force_push: false,
        allow_delete: false,
        enabled: true,
      }),
    });
    const rule = await createRes.json();
    ruleIds.push(rule.id);

    const res = await fetch(`${API_BASE}/branch-rules/${rule.id}`, {
      headers: headers(),
    });
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body.id).toBe(rule.id);
    expect(body.branch_pattern).toBe("develop");
  });

  test("PUT /branch-rules/{id} updates a rule", async () => {
    const createRes = await fetch(`${API_BASE}/projects/${projectId}/branch-rules`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({
        branch_pattern: "staging",
        require_reviews: false,
        require_tests: false,
        require_lint: false,
        allow_force_push: true,
        allow_delete: false,
        enabled: true,
      }),
    });
    const rule = await createRes.json();
    ruleIds.push(rule.id);

    const res = await fetch(`${API_BASE}/branch-rules/${rule.id}`, {
      method: "PUT",
      headers: jsonHeaders(),
      body: JSON.stringify({
        branch_pattern: "release/*",
        require_reviews: true,
      }),
    });
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body.branch_pattern).toBe("release/*");
    expect(body.require_reviews).toBe(true);
  });

  test("DELETE /branch-rules/{id} deletes a rule", async () => {
    const createRes = await fetch(`${API_BASE}/projects/${projectId}/branch-rules`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({
        branch_pattern: "feature/*",
        require_reviews: false,
        require_tests: false,
        require_lint: false,
        allow_force_push: false,
        allow_delete: true,
        enabled: false,
      }),
    });
    const rule = await createRes.json();

    const res = await fetch(`${API_BASE}/branch-rules/${rule.id}`, {
      method: "DELETE",
      headers: headers(),
    });
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body.status).toBe("deleted");
  });
});
