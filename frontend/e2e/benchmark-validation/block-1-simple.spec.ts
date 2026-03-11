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
 * Block 1: Simple Benchmarks
 *
 * Tests all suites that support benchmark_type=simple, one task each.
 * Verifies llm_judge and functional_test evaluator combinations.
 */
test.describe("Block 1: Simple Benchmarks", () => {
  const cases = getBlockCases(1);

  // Attach environment info on first test
  test.beforeAll(async () => {
    // Environment info is attached per-test via the reporter
  });

  for (const tc of cases) {
    test(`[${tc.id}] ${tc.suite} ${tc.type} ${tc.metrics.join("+")}`, async ({}, testInfo) => {
      const env = await collectEnvironmentInfo();
      await attachTestContext(testInfo, "environment", env);

      const model = tc.model ?? DEFAULT_MODEL;
      const dataset = suiteToDataset(tc.suite);

      // Look up suite to get suite_id
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

      // Create the run
      const run = await createBenchmarkRun(requestBody);
      expect(run.id).toBeTruthy();

      await attachTestContext(testInfo, "response", {
        status_code: 201,
        body: run,
      });

      // Wait for completion (5 min timeout for local model)
      const finalRun = await waitForRunCompletion(run.id, 600_000);
      const results = await getRunResults(run.id);

      await attachTestContext(testInfo, "run_result", {
        status: finalRun.status,
        total_cost: finalRun.total_cost_usd,
        total_tokens: (finalRun.total_tokens_in ?? 0) + (finalRun.total_tokens_out ?? 0),
        results: results.map((r) => ({
          task_id: r.task_id,
          scores: r.scores,
          duration_ms: r.duration_ms,
          error_message: r.error_message,
        })),
      });

      // Frontend checks
      const frontendChecks = await verifyFrontendState(finalRun, results);
      await attachTestContext(testInfo, "frontend_checks", frontendChecks);

      // --- Assertions ---
      expect(finalRun.status, `Run ${run.id} did not complete`).toBe("completed");
      expect(results.length).toBeGreaterThanOrEqual(1);

      if (tc.id === "1.2") {
        // Graceful degradation: functional_test on simple should not crash
        // Score should be 0 (no files written for simple runs)
        for (const r of results) {
          const ftScore = r.scores?.functional_test;
          if (ftScore !== undefined) {
            expect(ftScore).toBe(0);
          }
        }
      } else if (tc.id === "1.3") {
        // Both evaluators produce scores; llm_judge may be 0 with local models
        for (const r of results) {
          if (r.scores?.llm_judge !== undefined) {
            expect(r.scores.llm_judge).toBeGreaterThanOrEqual(0);
          }
          if (r.scores?.functional_test !== undefined) {
            expect(r.scores.functional_test).toBe(0);
          }
        }
      } else {
        // Standard case: scores must be present (pipeline works end-to-end)
        // Note: scores may be 0 with local models due to context-size limits
        // on the LLM judge — we validate the pipeline, not model quality.
        for (const r of results) {
          const scores = Object.values(r.scores ?? {});
          expect(scores.length, `No scores for task ${r.task_id}`).toBeGreaterThan(0);
          for (const s of scores) {
            expect(s).toBeGreaterThanOrEqual(0);
          }
        }
      }

      // Frontend display checks
      expect(frontendChecks.progress_bar_appeared).toBe(true);
      expect(frontendChecks.status_transition).toContain("completed");
    });
  }
});
