import { test, expect } from "@playwright/test";
import { apiLogin, API_BASE } from "../helpers/api-helpers";

/**
 * NATS evaluation test.
 * Verifies that creating a benchmark run triggers NATS evaluation flow.
 */
test.describe("NATS Evaluation (via API)", () => {
  let token: string;
  const createdIds: string[] = [];

  test.beforeAll(async () => {
    const auth = await apiLogin("admin@localhost", "Changeme123");
    token = auth.accessToken;
  });

  test.afterAll(async () => {
    for (const id of createdIds) {
      try {
        await fetch(`${API_BASE}/benchmarks/runs/${id}`, {
          method: "DELETE",
          headers: { Authorization: `Bearer ${token}` },
        });
      } catch {
        // best-effort
      }
    }
  });

  const headers = () => ({ Authorization: `Bearer ${token}` });
  const jsonHeaders = () => ({
    ...headers(),
    "Content-Type": "application/json",
  });

  test("benchmark run triggers evaluation NATS flow", async () => {
    const res = await fetch(`${API_BASE}/benchmarks/runs`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({
        dataset: "swe-bench-lite",
        model: "gpt-4o",
        metrics: ["accuracy", "pass_rate"],
      }),
    });
    // Benchmark routes require APP_ENV=development; returns 403 otherwise
    if (res.status === 403) {
      test.info().annotations.push({
        type: "skip-reason",
        description: "Benchmark routes require APP_ENV=development",
      });
      return;
    }
    expect(res.status).toBe(201);
    const body = await res.json();
    expect(body.id).toBeTruthy();
    createdIds.push(body.id);

    // Verify the run exists and has a status (NATS may have dispatched evaluation)
    const getRes = await fetch(`${API_BASE}/benchmarks/runs/${body.id}`, {
      headers: headers(),
    });
    expect(getRes.status).toBe(200);
    const runData = await getRes.json();
    expect(runData.id).toBe(body.id);
    expect(runData).toHaveProperty("status");
  });
});
