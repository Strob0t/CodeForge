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

      // Check requested evaluator scores are present
      for (const r of results) {
        for (const metric of tc.metrics) {
          const hasScore = r.scores?.[metric] !== undefined;
          if (metric === "functional_test") {
            // functional_test may be 0 or missing if no test_command
            // just verify the run didn't crash
          } else {
            expect(hasScore, `Missing score for ${metric} on task ${r.task_id}`).toBe(true);
          }
        }
      }

      // llm_judge scores should be > 0
      if (tc.metrics.includes("llm_judge")) {
        for (const r of results) {
          if (r.scores?.llm_judge !== undefined) {
            expect(r.scores.llm_judge).toBeGreaterThan(0);
          }
        }
      }

      // SPARC scores should be present if requested
      if (tc.metrics.includes("sparc")) {
        for (const r of results) {
          expect(r.scores?.sparc, `SPARC score missing for task ${r.task_id}`).toBeDefined();
        }
      }

      // trajectory_verifier scores should be present if requested
      if (tc.metrics.includes("trajectory_verifier")) {
        for (const r of results) {
          expect(
            r.scores?.trajectory_verifier,
            `trajectory_verifier score missing for task ${r.task_id}`,
          ).toBeDefined();
        }
      }

      // Frontend checks
      expect(frontendChecks.progress_bar_appeared).toBe(true);
      expect(frontendChecks.status_transition).toContain("completed");
    });
  }
});
