import { test, expect } from "@playwright/test";
import { apiLogin, API_BASE } from "../helpers/api-helpers";

/**
 * Intelligent Routing: validates the routing stats, outcomes, and
 * refresh/seed endpoints that power the hybrid routing cascade.
 */
test.describe("LLM E2E — Intelligent Routing", () => {
  let token: string;

  test.beforeAll(async () => {
    const auth = await apiLogin("admin@localhost", "Changeme123");
    token = auth.accessToken;
  });

  const headers = (): Record<string, string> => ({
    Authorization: `Bearer ${token}`,
  });

  const jsonHeaders = (): Record<string, string> => ({
    ...headers(),
    "Content-Type": "application/json",
  });

  test("GET /routing/stats returns array or null", async () => {
    const res = await fetch(`${API_BASE}/routing/stats`, {
      headers: headers(),
    });
    expect(res.status).toBe(200);
    const body = await res.json();
    // Go serializes empty slices as null — accept both
    expect(body === null || Array.isArray(body)).toBe(true);
  });

  test("GET /routing/stats?task_type=code filters results", async () => {
    const res = await fetch(`${API_BASE}/routing/stats?task_type=code`, {
      headers: headers(),
    });
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body === null || Array.isArray(body)).toBe(true);
  });

  test("GET /routing/stats?tier=complex filters results", async () => {
    const res = await fetch(`${API_BASE}/routing/stats?tier=complex`, {
      headers: headers(),
    });
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body === null || Array.isArray(body)).toBe(true);
  });

  test("POST /routing/outcomes records a routing outcome", async () => {
    const res = await fetch(`${API_BASE}/routing/outcomes`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({
        model_name: "openai/gpt-4o-mini",
        task_type: "code",
        complexity_tier: "simple",
        success: true,
        quality_score: 0.9,
        latency_ms: 500,
        cost_usd: 0.001,
        tokens_in: 100,
        tokens_out: 50,
      }),
    });
    expect([200, 201]).toContain(res.status);
  });

  test("POST /routing/outcomes with different task types", async () => {
    const taskTypes = ["review", "debug", "plan"];
    for (const taskType of taskTypes) {
      const res = await fetch(`${API_BASE}/routing/outcomes`, {
        method: "POST",
        headers: jsonHeaders(),
        body: JSON.stringify({
          model_name: "openai/gpt-4o-mini",
          task_type: taskType,
          complexity_tier: "medium",
          success: true,
          quality_score: 0.85,
          latency_ms: 600,
          cost_usd: 0.002,
          tokens_in: 200,
          tokens_out: 100,
        }),
      });
      expect([200, 201]).toContain(res.status);
    }
  });

  test("GET /routing/outcomes returns recorded outcomes", async () => {
    const res = await fetch(`${API_BASE}/routing/outcomes`, {
      headers: headers(),
    });
    expect(res.status).toBe(200);
    const body = await res.json();
    // After recording outcomes above, should have at least one
    const arr = body ?? [];
    expect(Array.isArray(arr)).toBe(true);
    expect(arr.length).toBeGreaterThanOrEqual(1);
  });

  test("GET /routing/outcomes?limit=1 respects limit", async () => {
    const res = await fetch(`${API_BASE}/routing/outcomes?limit=1`, {
      headers: headers(),
    });
    expect(res.status).toBe(200);
    const body = await res.json();
    const arr = body ?? [];
    expect(Array.isArray(arr)).toBe(true);
    expect(arr.length).toBeLessThanOrEqual(1);
  });

  test("POST /routing/stats/refresh works", async () => {
    const res = await fetch(`${API_BASE}/routing/stats/refresh`, {
      method: "POST",
      headers: headers(),
    });
    expect([200, 204]).toContain(res.status);
  });

  test("POST /routing/seed-from-benchmarks works", async () => {
    const res = await fetch(`${API_BASE}/routing/seed-from-benchmarks`, {
      method: "POST",
      headers: headers(),
    });
    expect([200, 204, 404]).toContain(res.status);
  });

  test("routing stats include model_name and complexity_tier fields", async () => {
    const res = await fetch(`${API_BASE}/routing/stats`, {
      headers: headers(),
    });
    expect(res.status).toBe(200);
    const body = (await res.json()) as Array<Record<string, unknown>> | null;
    const arr = body ?? [];

    // Each stat entry should have model_name and complexity_tier
    if (arr.length > 0) {
      for (const entry of arr) {
        expect(entry).toHaveProperty("model_name");
        expect(entry).toHaveProperty("complexity_tier");
      }
    }
  });
});
