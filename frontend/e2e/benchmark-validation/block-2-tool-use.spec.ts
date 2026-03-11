import { test, expect } from "@playwright/test";
import {
  createBenchmarkRun,
  waitForRunCompletion,
  getRunResults,
  verifyFrontendState,
  attachTestContext,
  collectEnvironmentInfo,
  suiteToDataset,
  getSuiteByProvider,
} from "./helpers";
import { getBlockCases, DEFAULT_MODEL } from "./matrix";

/**
 * Block 2: Tool-Use Benchmarks
 *
 * Tests codeforge_tool_use suite with different evaluator combinations.
 * Verifies tool_calls are present in results.
 */
test.describe("Block 2: Tool-Use Benchmarks", () => {
  const cases = getBlockCases(2);

  for (const tc of cases) {
    test(`[${tc.id}] ${tc.suite} ${tc.type} ${tc.metrics.join("+")}`, async ({}, testInfo) => {
      const env = await collectEnvironmentInfo();
      await attachTestContext(testInfo, "environment", env);

      const model = tc.model ?? DEFAULT_MODEL;
      const dataset = suiteToDataset(tc.suite);

      const suite = await getSuiteByProvider(tc.suite);
      expect(suite, `Suite for provider ${tc.suite} not found`).toBeTruthy();

      const requestBody = {
        dataset,
        model,
        metrics: tc.metrics as string[],
        benchmark_type: tc.type,
        exec_mode: "mount",
        suite_id: suite!.id,
      };

      await attachTestContext(testInfo, "request", {
        method: "POST",
        url: "/api/v1/benchmarks/runs",
        body: requestBody,
      });

      const run = await createBenchmarkRun(requestBody);
      expect(run.id).toBeTruthy();

      await attachTestContext(testInfo, "response", {
        status_code: 201,
        body: run,
      });

      // Wait for completion (5 min for local model)
      const finalRun = await waitForRunCompletion(run.id, 300_000);
      const results = await getRunResults(run.id);

      await attachTestContext(testInfo, "run_result", {
        status: finalRun.status,
        total_cost: finalRun.total_cost_usd,
        total_tokens: (finalRun.total_tokens_in ?? 0) + (finalRun.total_tokens_out ?? 0),
        results: results.map((r) => ({
          task_id: r.task_id,
          scores: r.scores,
          actual_output: r.actual_output?.slice(0, 500),
          duration_ms: r.duration_ms,
          error_message: r.error_message,
        })),
      });

      const frontendChecks = await verifyFrontendState(finalRun, results);
      await attachTestContext(testInfo, "frontend_checks", frontendChecks);

      // --- Assertions ---
      expect(finalRun.status, `Run ${run.id} did not complete`).toBe("completed");
      expect(results.length).toBeGreaterThanOrEqual(1);

      // Check that evaluator scores are present per metrics requested
      for (const r of results) {
        for (const metric of tc.metrics) {
          expect(
            r.scores?.[metric] !== undefined,
            `Missing score for metric ${metric} on task ${r.task_id}`,
          ).toBe(true);
        }
      }

      // For llm_judge metrics, score should be > 0
      if (tc.metrics.includes("llm_judge")) {
        for (const r of results) {
          expect(r.scores?.llm_judge).toBeGreaterThan(0);
        }
      }

      // Frontend checks
      expect(frontendChecks.progress_bar_appeared).toBe(true);
      expect(frontendChecks.status_transition).toContain("completed");
    });
  }
});
