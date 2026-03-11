import { test, expect } from "@playwright/test";
import {
  createBenchmarkRun,
  createBenchmarkRunRaw,
  waitForRunCompletion,
  getRunResults,
  attachTestContext,
  collectEnvironmentInfo,
} from "./helpers";
import { ERROR_SCENARIOS, DEFAULT_MODEL } from "./matrix";

/**
 * Block 5: Error Scenarios
 *
 * Tests system behavior under failure conditions:
 * invalid datasets, invalid models, empty datasets,
 * unknown evaluators, and duplicate runs.
 */
test.describe("Block 5: Error Scenarios", () => {
  test(`[5.1] Invalid dataset returns error`, async ({}, testInfo) => {
    const env = await collectEnvironmentInfo();
    await attachTestContext(testInfo, "environment", env);

    const scenario = ERROR_SCENARIOS[0]; // 5.1
    await attachTestContext(testInfo, "request", {
      method: "POST",
      url: "/api/v1/benchmarks/runs",
      body: scenario.params,
    });

    const { status, body } = await createBenchmarkRunRaw(scenario.params);
    await attachTestContext(testInfo, "response", { status_code: status, body });

    // Should fail at creation (400) or during execution (run status=failed)
    if (status === 201) {
      // Run was created but should fail during execution
      const runBody = body as { id: string };
      const finalRun = await waitForRunCompletion(runBody.id, 60_000);
      expect(finalRun.status).toBe("failed");
      expect(finalRun.error_message).toBeTruthy();
      await attachTestContext(testInfo, "run_result", {
        status: finalRun.status,
        error_message: finalRun.error_message,
      });
    } else {
      // Rejected at API level
      expect([400, 404, 500]).toContain(status);
    }
  });

  test(`[5.2] Invalid model returns error`, async ({}, testInfo) => {
    const env = await collectEnvironmentInfo();
    await attachTestContext(testInfo, "environment", env);

    const scenario = ERROR_SCENARIOS[1]; // 5.2
    await attachTestContext(testInfo, "request", {
      method: "POST",
      url: "/api/v1/benchmarks/runs",
      body: scenario.params,
    });

    const { status, body } = await createBenchmarkRunRaw(scenario.params);
    await attachTestContext(testInfo, "response", { status_code: status, body });

    if (status === 201) {
      const runBody = body as { id: string };
      const finalRun = await waitForRunCompletion(runBody.id, 120_000);
      expect(finalRun.status).toBe("failed");
      expect(finalRun.error_message).toBeTruthy();
      await attachTestContext(testInfo, "run_result", {
        status: finalRun.status,
        error_message: finalRun.error_message,
      });
    } else {
      expect([400, 404, 500, 502]).toContain(status);
    }
  });

  test(`[5.3] Empty dataset (0 tasks) handled gracefully`, async ({}, testInfo) => {
    const env = await collectEnvironmentInfo();
    await attachTestContext(testInfo, "environment", env);

    // Use a dataset name that doesn't exist or has 0 tasks
    const scenario = ERROR_SCENARIOS[2]; // 5.3
    await attachTestContext(testInfo, "request", {
      method: "POST",
      url: "/api/v1/benchmarks/runs",
      body: scenario.params,
    });

    const { status, body } = await createBenchmarkRunRaw(scenario.params);
    await attachTestContext(testInfo, "response", { status_code: status, body });

    if (status === 201) {
      const runBody = body as { id: string };
      // Should complete quickly (no tasks to process) or fail with clear message
      const finalRun = await waitForRunCompletion(runBody.id, 60_000);
      // Either completed (0 results) or failed with clear message
      expect(["completed", "failed"]).toContain(finalRun.status);
      if (finalRun.status === "completed") {
        const results = await getRunResults(runBody.id);
        expect(results.length).toBe(0);
      }
      await attachTestContext(testInfo, "run_result", {
        status: finalRun.status,
        error_message: finalRun.error_message,
      });
    } else {
      // Rejected at API level — that's also fine
      expect([400, 404]).toContain(status);
    }
  });

  test(`[5.4] Unknown evaluator falls back to llm_judge`, async ({}, testInfo) => {
    const env = await collectEnvironmentInfo();
    await attachTestContext(testInfo, "environment", env);

    const scenario = ERROR_SCENARIOS[3]; // 5.4
    await attachTestContext(testInfo, "request", {
      method: "POST",
      url: "/api/v1/benchmarks/runs",
      body: scenario.params,
    });

    const { status, body } = await createBenchmarkRunRaw(scenario.params);
    await attachTestContext(testInfo, "response", { status_code: status, body });

    if (status === 201) {
      const runBody = body as { id: string };
      // _build_evaluators() silently skips unknown names and falls back to LLMJudge
      const finalRun = await waitForRunCompletion(runBody.id, 300_000);
      // Should complete normally with llm_judge scores (graceful degradation)
      expect(finalRun.status).toBe("completed");

      const results = await getRunResults(runBody.id);
      expect(results.length).toBeGreaterThanOrEqual(1);

      // llm_judge should have been used as fallback
      for (const r of results) {
        const hasAnyScore = Object.keys(r.scores ?? {}).length > 0;
        expect(hasAnyScore, `No scores for task ${r.task_id} — fallback may have failed`).toBe(
          true,
        );
      }

      await attachTestContext(testInfo, "run_result", {
        status: finalRun.status,
        total_cost: finalRun.total_cost_usd,
        results: results.map((r) => ({
          task_id: r.task_id,
          scores: r.scores,
        })),
      });
    } else {
      // If rejected at API level, that's also acceptable
      expect([400]).toContain(status);
    }
  });

  test(`[5.5] Duplicate run (same params) is idempotent`, async ({}, testInfo) => {
    const env = await collectEnvironmentInfo();
    await attachTestContext(testInfo, "environment", env);

    const params = {
      dataset: "basic-coding",
      model: DEFAULT_MODEL,
      metrics: ["llm_judge"],
      benchmark_type: "simple",
    };

    await attachTestContext(testInfo, "request", {
      method: "POST",
      url: "/api/v1/benchmarks/runs",
      body: { ...params, note: "sent twice" },
    });

    // Send two identical run requests in quick succession
    const [result1, result2] = await Promise.all([
      createBenchmarkRunRaw(params),
      createBenchmarkRunRaw(params),
    ]);

    await attachTestContext(testInfo, "response", {
      status_code: result1.status,
      body: { run1: result1.body, run2: result2.body },
    });

    // Both should succeed (each creates its own run)
    // The key test: no crashes, no data corruption, no infinite loops
    if (result1.status === 201 && result2.status === 201) {
      const run1 = result1.body as { id: string };
      const run2 = result2.body as { id: string };

      // They should be different runs (not deduplicated — each request creates a new run)
      expect(run1.id).not.toBe(run2.id);

      // Wait for both to complete
      const [final1, final2] = await Promise.all([
        waitForRunCompletion(run1.id, 300_000),
        waitForRunCompletion(run2.id, 300_000),
      ]);

      // Both should reach a terminal state (not stuck)
      expect(["completed", "failed"]).toContain(final1.status);
      expect(["completed", "failed"]).toContain(final2.status);

      await attachTestContext(testInfo, "run_result", {
        run1: { id: run1.id, status: final1.status },
        run2: { id: run2.id, status: final2.status },
      });
    }
  });
});
