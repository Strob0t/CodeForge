import { test, expect } from "@playwright/test";
import { apiLogin, API_BASE } from "../helpers/api-helpers";

/**
 * Benchmark evaluation endpoints (dev mode only).
 * These tests verify the full benchmark lifecycle: datasets, run creation, results, deletion.
 * The entire suite is skipped when the backend is not running in development mode.
 */
test.describe("LLM E2E — Benchmarks", () => {
  let token: string;
  let devMode = false;
  let createdRunId: string | null = null;

  test.beforeAll(async () => {
    const auth = await apiLogin("admin@localhost", "Changeme123");
    token = auth.accessToken;

    // Check if backend is in development mode
    const res = await fetch(`${API_BASE.replace("/api/v1", "")}/health`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    if (res.ok) {
      const body = (await res.json()) as { dev_mode?: boolean };
      devMode = body.dev_mode === true;
    }
  });

  const headers = (): Record<string, string> => ({
    Authorization: `Bearer ${token}`,
  });

  const jsonHeaders = (): Record<string, string> => ({
    ...headers(),
    "Content-Type": "application/json",
  });

  test("GET /benchmarks/datasets returns datasets array", async () => {
    test.skip(!devMode, "Benchmark endpoints require APP_ENV=development");

    const res = await fetch(`${API_BASE}/benchmarks/datasets`, {
      headers: headers(),
    });
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(Array.isArray(body)).toBe(true);
  });

  test("POST /benchmarks/runs creates benchmark run", async () => {
    test.skip(!devMode, "Benchmark endpoints require APP_ENV=development");

    const res = await fetch(`${API_BASE}/benchmarks/runs`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({
        dataset: "humaneval",
        model: "openai/gpt-4o-mini",
        metrics: ["correctness"],
      }),
    });
    // 201 on success, 400 if dataset not found, 502 if model not available
    expect([201, 400, 502]).toContain(res.status);
    if (res.status === 201) {
      const body = await res.json();
      expect(body.id).toBeTruthy();
      createdRunId = body.id;
    }
  });

  test("GET /benchmarks/runs lists benchmark runs", async () => {
    test.skip(!devMode, "Benchmark endpoints require APP_ENV=development");

    const res = await fetch(`${API_BASE}/benchmarks/runs`, {
      headers: headers(),
    });
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(Array.isArray(body)).toBe(true);
  });

  test("GET /benchmarks/runs/{id} returns run details", async () => {
    test.skip(!devMode, "Benchmark endpoints require APP_ENV=development");
    test.skip(!createdRunId, "No benchmark run was created in previous test");

    const res = await fetch(`${API_BASE}/benchmarks/runs/${createdRunId}`, {
      headers: headers(),
    });
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body.id).toBe(createdRunId);
  });

  test("GET /benchmarks/runs/{id}/results returns results", async () => {
    test.skip(!devMode, "Benchmark endpoints require APP_ENV=development");
    test.skip(!createdRunId, "No benchmark run was created in previous test");

    const res = await fetch(`${API_BASE}/benchmarks/runs/${createdRunId}/results`, {
      headers: headers(),
    });
    // 200 with results (may be empty if run not completed), 404 if run vanished
    expect([200, 404]).toContain(res.status);
    if (res.status === 200) {
      const body = await res.json();
      expect(Array.isArray(body)).toBe(true);
    }
  });

  test("DELETE /benchmarks/runs/{id} deletes run", async () => {
    test.skip(!devMode, "Benchmark endpoints require APP_ENV=development");
    test.skip(!createdRunId, "No benchmark run was created in previous test");

    const res = await fetch(`${API_BASE}/benchmarks/runs/${createdRunId}`, {
      method: "DELETE",
      headers: headers(),
    });
    expect([200, 204]).toContain(res.status);
    createdRunId = null;
  });

  test("POST /benchmarks/runs without dataset returns 400", async () => {
    test.skip(!devMode, "Benchmark endpoints require APP_ENV=development");

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
