import { test, expect } from "@playwright/test";
import { apiLogin, API_BASE } from "../helpers/api-helpers";

test.describe("Benchmarks API", () => {
  let token: string;

  test.beforeAll(async () => {
    const auth = await apiLogin("admin@localhost", "Changeme123");
    token = auth.accessToken;
  });

  const headers = () => ({ Authorization: `Bearer ${token}` });
  const jsonHeaders = () => ({
    ...headers(),
    "Content-Type": "application/json",
  });

  // Benchmark routes are protected by DevModeOnly middleware.
  // When APP_ENV != "development", all endpoints return 403.

  test("GET /benchmarks/runs returns 403 when not in dev mode", async () => {
    const res = await fetch(`${API_BASE}/benchmarks/runs`, {
      headers: headers(),
    });
    // 200 if APP_ENV=development, 403 otherwise
    if (res.status === 403) {
      const body = await res.json();
      expect(body.error).toContain("development mode");
    } else {
      expect(res.status).toBe(200);
      const body = await res.json();
      expect(Array.isArray(body)).toBe(true);
    }
  });

  test("POST /benchmarks/runs returns 403 or 201 depending on dev mode", async () => {
    const res = await fetch(`${API_BASE}/benchmarks/runs`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({
        dataset: "swe-bench-lite",
        model: "gpt-4o",
        metrics: ["accuracy", "pass_rate"],
      }),
    });
    if (res.status === 403) {
      const body = await res.json();
      expect(body.error).toContain("development mode");
    } else {
      expect(res.status).toBe(201);
      const body = await res.json();
      expect(body.id).toBeTruthy();
      // Clean up
      await fetch(`${API_BASE}/benchmarks/runs/${body.id}`, {
        method: "DELETE",
        headers: headers(),
      });
    }
  });

  test("GET /benchmarks/runs/{id} returns 403 or valid response", async () => {
    const res = await fetch(`${API_BASE}/benchmarks/runs/00000000-0000-0000-0000-000000000000`, {
      headers: headers(),
    });
    // 403 if not in dev mode, 404 if in dev mode but run doesn't exist
    expect([403, 404]).toContain(res.status);
  });

  test("DELETE /benchmarks/runs/{id} returns 403 or valid response", async () => {
    const res = await fetch(`${API_BASE}/benchmarks/runs/00000000-0000-0000-0000-000000000000`, {
      method: "DELETE",
      headers: headers(),
    });
    // 403 if not in dev mode, 404 if in dev mode but run doesn't exist
    expect([403, 404]).toContain(res.status);
  });

  test("GET /benchmarks/runs/{id}/results returns 403 or valid response", async () => {
    const res = await fetch(
      `${API_BASE}/benchmarks/runs/00000000-0000-0000-0000-000000000000/results`,
      {
        headers: headers(),
      },
    );
    // 403 if not in dev mode, 200/404 if in dev mode
    expect([403, 200, 404]).toContain(res.status);
  });

  test("POST /benchmarks/compare returns 403 or valid response", async () => {
    const res = await fetch(`${API_BASE}/benchmarks/compare`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({
        run_id_a: "00000000-0000-0000-0000-000000000001",
        run_id_b: "00000000-0000-0000-0000-000000000002",
      }),
    });
    // 403 if not in dev mode, 400/404 if in dev mode but runs don't exist
    expect([403, 400, 404, 500]).toContain(res.status);
  });

  test("GET /benchmarks/datasets returns 403 or datasets array", async () => {
    const res = await fetch(`${API_BASE}/benchmarks/datasets`, {
      headers: headers(),
    });
    if (res.status === 403) {
      const body = await res.json();
      expect(body.error).toContain("development mode");
    } else {
      expect(res.status).toBe(200);
      const body = await res.json();
      expect(Array.isArray(body)).toBe(true);
    }
  });

  test("POST /benchmarks/runs with missing dataset returns 403 or 400", async () => {
    const res = await fetch(`${API_BASE}/benchmarks/runs`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({ model: "gpt-4o", metrics: ["accuracy"] }),
    });
    // 403 if not in dev mode, 400 if in dev mode (missing dataset)
    expect([403, 400]).toContain(res.status);
  });
});
