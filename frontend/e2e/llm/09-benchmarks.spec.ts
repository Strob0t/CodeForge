import { test, expect } from "@playwright/test";
import { apiLogin, API_BASE } from "../helpers/api-helpers";

/**
 * Benchmark evaluation endpoints (dev mode only).
 * These tests verify the full benchmark lifecycle: datasets, run creation, results, deletion.
 * Requires APP_ENV=development on the backend.
 */
test.describe("LLM E2E — Benchmarks", () => {
  let token: string;

  test.beforeAll(async () => {
    const auth = await apiLogin("admin@localhost", "Changeme123");
    token = auth.accessToken;

    // Verify backend is in development mode
    const res = await fetch(`${API_BASE.replace("/api/v1", "")}/health`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    expect(res.ok).toBe(true);
    const body = (await res.json()) as { dev_mode?: boolean };
    expect(body.dev_mode).toBe(true);
  });

  const headers = (): Record<string, string> => ({
    Authorization: `Bearer ${token}`,
  });

  const jsonHeaders = (): Record<string, string> => ({
    ...headers(),
    "Content-Type": "application/json",
  });

  test("GET /benchmarks/datasets returns datasets array", async () => {
    const res = await fetch(`${API_BASE}/benchmarks/datasets`, {
      headers: headers(),
    });
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(Array.isArray(body)).toBe(true);
  });

  test("benchmark run full lifecycle: create, list, get, results, delete", async () => {
    // Create benchmark run
    const createRes = await fetch(`${API_BASE}/benchmarks/runs`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({
        dataset: "humaneval",
        model: "openai/gpt-4o-mini",
        metrics: ["correctness"],
      }),
    });
    // 201 on success, 400 if dataset not found, 502 if model not available
    expect([201, 400, 502]).toContain(createRes.status);

    if (createRes.status === 201) {
      const createBody = await createRes.json();
      expect(createBody.id).toBeTruthy();
      const runId = createBody.id;

      // List benchmark runs
      const listRes = await fetch(`${API_BASE}/benchmarks/runs`, {
        headers: headers(),
      });
      expect(listRes.status).toBe(200);
      const runs = await listRes.json();
      expect(Array.isArray(runs)).toBe(true);

      // Get run details
      const getRes = await fetch(`${API_BASE}/benchmarks/runs/${runId}`, {
        headers: headers(),
      });
      expect(getRes.status).toBe(200);
      const run = await getRes.json();
      expect(run.id).toBe(runId);

      // Get run results
      const resultsRes = await fetch(`${API_BASE}/benchmarks/runs/${runId}/results`, {
        headers: headers(),
      });
      expect([200, 404]).toContain(resultsRes.status);
      if (resultsRes.status === 200) {
        const results = await resultsRes.json();
        expect(Array.isArray(results)).toBe(true);
      }

      // Delete run
      const delRes = await fetch(`${API_BASE}/benchmarks/runs/${runId}`, {
        method: "DELETE",
        headers: headers(),
      });
      expect([200, 204]).toContain(delRes.status);
    } else {
      // Even if creation failed, verify list endpoint works
      const listRes = await fetch(`${API_BASE}/benchmarks/runs`, {
        headers: headers(),
      });
      expect(listRes.status).toBe(200);
    }
  });

  test("GET /benchmarks/runs lists benchmark runs", async () => {
    const res = await fetch(`${API_BASE}/benchmarks/runs`, {
      headers: headers(),
    });
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(Array.isArray(body)).toBe(true);
  });

  test("POST /benchmarks/runs without dataset returns 400", async () => {
    const res = await fetch(`${API_BASE}/benchmarks/runs`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({
        model: "openai/gpt-4o-mini",
        metrics: ["correctness"],
      }),
    });
    // 400 for missing required field, 500 if worker-side validation
    expect([400, 500]).toContain(res.status);
  });
});
