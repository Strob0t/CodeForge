import { test, expect } from "@playwright/test";
import { apiLogin, createCleanupTracker, API_BASE } from "../helpers/api-helpers";

test.describe("Policies API", () => {
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

  test("list policies returns profiles array", async ({ request }) => {
    const res = await request.get(`${API_BASE}/policies`, { headers: headers() });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(body.profiles).toBeDefined();
    expect(Array.isArray(body.profiles)).toBe(true);
  });

  test("built-in policies exist", async ({ request }) => {
    const res = await request.get(`${API_BASE}/policies`, { headers: headers() });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(body.profiles.length).toBeGreaterThan(0);
  });

  test("create custom policy returns 201", async ({ request }) => {
    const name = `e2e-policy-${Date.now()}`;
    const res = await request.post(`${API_BASE}/policies`, {
      headers: headers(),
      data: {
        name,
        description: "E2E test policy",
        mode: "default",
        rules: [{ specifier: { tool: "*" }, decision: "allow" }],
        quality_gate: {},
        termination: {},
      },
    });
    expect(res.status()).toBe(201);
    const body = await res.json();
    expect(body.name).toBe(name);
    cleanup.add("policy", name);
  });

  test("get policy by name", async ({ request }) => {
    const name = `e2e-get-pol-${Date.now()}`;
    await request.post(`${API_BASE}/policies`, {
      headers: headers(),
      data: {
        name,
        description: "get test",
        mode: "default",
        rules: [{ specifier: { tool: "*" }, decision: "allow" }],
        quality_gate: {},
        termination: {},
      },
    });
    cleanup.add("policy", name);

    const res = await request.get(`${API_BASE}/policies/${name}`, { headers: headers() });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(body.name).toBe(name);
  });

  test("delete custom policy returns 204", async ({ request }) => {
    const name = `e2e-del-pol-${Date.now()}`;
    await request.post(`${API_BASE}/policies`, {
      headers: headers(),
      data: {
        name,
        description: "delete test",
        mode: "default",
        rules: [{ specifier: { tool: "*" }, decision: "allow" }],
        quality_gate: {},
        termination: {},
      },
    });

    const res = await request.delete(`${API_BASE}/policies/${name}`, { headers: headers() });
    expect(res.status()).toBe(204);
  });

  test("evaluate policy returns result", async ({ request }) => {
    // Use the first available policy profile
    const listRes = await request.get(`${API_BASE}/policies`, { headers: headers() });
    const profiles = (await listRes.json()).profiles as string[];
    expect(profiles.length).toBeGreaterThan(0);

    const res = await request.post(`${API_BASE}/policies/${profiles[0]}/evaluate`, {
      headers: headers(),
      data: { tool: "Bash", command: "ls" },
    });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(body).toBeTruthy();
  });

  test("delete non-existent policy returns 404", async ({ request }) => {
    const res = await request.delete(`${API_BASE}/policies/nonexistent-policy-name-${Date.now()}`, {
      headers: headers(),
    });
    expect(res.status()).toBe(404);
  });
});
