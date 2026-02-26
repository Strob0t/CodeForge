import { test, expect } from "@playwright/test";
import { apiLogin, createProject, createCleanupTracker, API_BASE } from "../helpers/api-helpers";

test.describe("Costs API", () => {
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

  test("global cost summary returns array", async ({ request }) => {
    const res = await request.get(`${API_BASE}/costs`, { headers: headers() });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(Array.isArray(body)).toBe(true);
  });

  test("project cost summary", async ({ request }) => {
    const proj = await createProject(`e2e-cost-proj-${Date.now()}`);
    cleanup.add("project", proj.id);

    const res = await request.get(`${API_BASE}/projects/${proj.id}/costs`, {
      headers: headers(),
    });
    expect(res.status()).toBe(200);
  });

  test("project cost by model returns array", async ({ request }) => {
    const proj = await createProject(`e2e-cost-model-${Date.now()}`);
    cleanup.add("project", proj.id);

    const res = await request.get(`${API_BASE}/projects/${proj.id}/costs/by-model`, {
      headers: headers(),
    });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(Array.isArray(body)).toBe(true);
  });

  test("project cost by tool returns array", async ({ request }) => {
    const proj = await createProject(`e2e-cost-tool-${Date.now()}`);
    cleanup.add("project", proj.id);

    const res = await request.get(`${API_BASE}/projects/${proj.id}/costs/by-tool`, {
      headers: headers(),
    });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(Array.isArray(body)).toBe(true);
  });

  test("daily cost time series returns array", async ({ request }) => {
    const proj = await createProject(`e2e-cost-daily-${Date.now()}`);
    cleanup.add("project", proj.id);

    const res = await request.get(`${API_BASE}/projects/${proj.id}/costs/daily`, {
      headers: headers(),
    });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(Array.isArray(body)).toBe(true);
  });

  test("project recent runs returns array", async ({ request }) => {
    const proj = await createProject(`e2e-cost-runs-${Date.now()}`);
    cleanup.add("project", proj.id);

    const res = await request.get(`${API_BASE}/projects/${proj.id}/costs/runs`, {
      headers: headers(),
    });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(Array.isArray(body)).toBe(true);
  });

  test("run cost by tool for non-existent run", async ({ request }) => {
    const res = await request.get(
      `${API_BASE}/runs/00000000-0000-0000-0000-000000000000/costs/by-tool`,
      { headers: headers() },
    );
    // May return 404 or empty array
    expect([200, 404]).toContain(res.status());
  });

  test("non-existent project costs returns 404 or empty", async ({ request }) => {
    const res = await request.get(
      `${API_BASE}/projects/00000000-0000-0000-0000-000000000000/costs`,
      { headers: headers() },
    );
    expect([200, 404]).toContain(res.status());
  });
});
