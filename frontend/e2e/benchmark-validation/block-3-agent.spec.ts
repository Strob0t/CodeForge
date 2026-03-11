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
 * Block 3: Agent Benchmarks
 *
 * Tests all suites that support benchmark_type=agent, exec_mode=mount.
 * Verifies multi-turn execution, file changes, and evaluator combinations
 * (llm_judge, trajectory_verifier, sparc, functional_test).
 */
test.describe("Block 3: Agent Benchmarks", () => {
  const cases = getBlockCases(3);

  for (const tc of cases) {
    // Agent tests get extra timeout (10 min) — multi-turn with local model
    test(`[${tc.id}] ${tc.suite} ${tc.type} ${tc.metrics.join("+")}`, async ({}, testInfo) => {
      test.setTimeout(600_000); // 10 minutes

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

      // Agent runs need more time — 8 min polling timeout
      const finalRun = await waitForRunCompletion(run.id, 480_000);
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

      // Verify each result has a scores object (pipeline works end-to-end)
      // Note: metric request names (llm_judge, sparc, trajectory_verifier) map to
      // different score keys (correctness, sparc_*, trajectory_quality) — so we
      // validate the pipeline produced scores, not specific key names.
      for (const r of results) {
        expect(r.scores, `No scores object for task ${r.task_id}`).toBeTruthy();
        const scoreValues = Object.values(r.scores ?? {});
        for (const s of scoreValues) {
          expect(s).toBeGreaterThanOrEqual(0);
        }
      }

      // Frontend checks
      expect(frontendChecks.progress_bar_appeared).toBe(true);
      expect(frontendChecks.status_transition).toContain("completed");
    });
  }
});
