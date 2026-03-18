/**
 * Full Benchmark Validation Test Suite
 *
 * Runs the complete test matrix to verify the benchmark system works end-to-end.
 * Design:
 *   - 1 test for auto router verification (model=auto)
 *   - All other tests use lm_studio/qwen/qwen3-30b-a3b
 *   - Each test uses e2e-quick dataset (2 tasks) for manageable runtime
 *   - Estimated total: ~30-60 min depending on local model speed
 *
 * Prerequisites:
 *   - Go backend running with APP_ENV=development
 *   - Python worker running with LITELLM_BASE_URL=<litellm-container-ip>:4000
 *   - LiteLLM proxy with lm_studio/qwen/qwen3-30b-a3b available
 *   - NATS JetStream running
 *   - PostgreSQL running
 */

import { test, expect } from "@playwright/test";
import {
  checkBackendHealth,
  checkLiteLLMHealth,
  getLiteLLMModels,
  createBenchmarkRun,
  createBenchmarkRunRaw,
  waitForRunCompletion,
  getRunResults,
  deleteRun,
  listSuites,
  listDatasets,
  verifyFrontendState,
  attachTestContext,
  collectEnvironmentInfo,
  suiteToDataset,
  getSuiteByProvider,
} from "./helpers";
import { DEFAULT_MODEL, VALIDATION_MATRIX, ERROR_SCENARIOS, getBlockCases } from "./matrix";

// Increase timeout for local model runs (~2 min/task)
test.setTimeout(900_000); // 15 min per test

// ============================================================================
// Block 0: Prerequisites
// ============================================================================

test.describe("Block 0: Prerequisites", () => {
  test("[0.1] Backend is healthy and in dev mode", async () => {
    const health = await checkBackendHealth();
    expect(health.status).toBe("ok");
    expect(health.dev_mode).toBe(true);
  });

  test("[0.2] LiteLLM proxy is reachable", async () => {
    const healthy = await checkLiteLLMHealth();
    expect(healthy, "LiteLLM proxy not reachable").toBe(true);
  });

  test("[0.3] Default model is available in LiteLLM", async () => {
    const models = await getLiteLLMModels();
    expect(models.length, "No models available in LiteLLM").toBeGreaterThan(0);
    // Check for the model or a wildcard that covers it
    const hasModel = models.includes(DEFAULT_MODEL) || models.some((m) => m === "lm_studio/*");
    expect(hasModel, `Model ${DEFAULT_MODEL} not found. Available: ${models.join(", ")}`).toBe(
      true,
    );
  });

  test("[0.4] Benchmark suites are registered", async () => {
    const suites = await listSuites();
    expect(suites.length, "No benchmark suites registered").toBeGreaterThan(0);

    const requiredProviders = ["codeforge_simple", "codeforge_tool_use", "codeforge_agent"];
    for (const provider of requiredProviders) {
      const suite = suites.find((s) => s.provider_name === provider);
      expect(suite, `Suite for provider '${provider}' not found`).toBeTruthy();
    }
  });

  test("[0.5] Benchmark datasets are discoverable", async () => {
    const datasets = await listDatasets();
    expect(datasets.length, "No benchmark datasets found").toBeGreaterThan(0);
    const names = datasets.map((d) => d.name);
    expect(names).toContain("e2e-quick");
  });
});

// ============================================================================
// Block 1: Simple Benchmarks
// ============================================================================

test.describe("Block 1: Simple Benchmarks", () => {
  const cases = getBlockCases(1);

  for (const tc of cases) {
    test(`[${tc.id}] ${tc.suite} ${tc.type} ${tc.metrics.join("+")}`, async ({}, testInfo) => {
      const env = await collectEnvironmentInfo();
      await attachTestContext(testInfo, "environment", env);

      const model = tc.model ?? DEFAULT_MODEL;
      const dataset = suiteToDataset(tc.suite);

      const suite = await getSuiteByProvider(tc.suite);
      expect(suite, `Suite for provider ${tc.suite} not found`).toBeTruthy();

      const run = await createBenchmarkRun({
        dataset,
        model,
        metrics: tc.metrics as string[],
        benchmark_type: tc.type,
        exec_mode: "mount",
        suite_id: suite!.id,
      });
      expect(run.id).toBeTruthy();

      const finalRun = await waitForRunCompletion(run.id, 600_000);
      const results = await getRunResults(run.id);

      await attachTestContext(testInfo, "run_result", {
        status: finalRun.status,
        error_message: finalRun.error_message,
        results_count: results.length,
        results: results.map((r) => ({
          task_id: r.task_id,
          scores: r.scores,
          duration_ms: r.duration_ms,
        })),
      });

      expect(finalRun.status, `Run failed: ${finalRun.error_message ?? "unknown"}`).toBe(
        "completed",
      );
      expect(results.length).toBeGreaterThanOrEqual(1);

      // Verify scores exist (value may be 0 with local models)
      for (const r of results) {
        const scores = Object.values(r.scores ?? {});
        expect(scores.length, `No scores for task ${r.task_id}`).toBeGreaterThan(0);
        for (const s of scores) {
          expect(s).toBeGreaterThanOrEqual(0);
          expect(s).toBeLessThanOrEqual(1);
        }
      }

      const frontendChecks = await verifyFrontendState(finalRun, results);
      expect(frontendChecks.progress_bar_appeared).toBe(true);
      expect(frontendChecks.status_transition).toContain("completed");
    });
  }
});

// ============================================================================
// Block 2: Tool-Use Benchmarks
// ============================================================================

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

      const run = await createBenchmarkRun({
        dataset,
        model,
        metrics: tc.metrics as string[],
        benchmark_type: tc.type,
        exec_mode: "mount",
        suite_id: suite!.id,
      });
      expect(run.id).toBeTruthy();

      const finalRun = await waitForRunCompletion(run.id, 600_000);
      const results = await getRunResults(run.id);

      await attachTestContext(testInfo, "run_result", {
        status: finalRun.status,
        error_message: finalRun.error_message,
        results_count: results.length,
      });

      expect(finalRun.status, `Run failed: ${finalRun.error_message ?? "unknown"}`).toBe(
        "completed",
      );
      expect(results.length).toBeGreaterThanOrEqual(1);
    });
  }
});

// ============================================================================
// Block 3: Agent Benchmarks
// ============================================================================

test.describe("Block 3: Agent Benchmarks", () => {
  const cases = getBlockCases(3);

  for (const tc of cases) {
    test(`[${tc.id}] ${tc.suite} ${tc.type} ${tc.metrics.join("+")}`, async ({}, testInfo) => {
      const env = await collectEnvironmentInfo();
      await attachTestContext(testInfo, "environment", env);

      const model = tc.model ?? DEFAULT_MODEL;
      const dataset = suiteToDataset(tc.suite);

      const suite = await getSuiteByProvider(tc.suite);
      expect(suite, `Suite for provider ${tc.suite} not found`).toBeTruthy();

      const run = await createBenchmarkRun({
        dataset,
        model,
        metrics: tc.metrics as string[],
        benchmark_type: tc.type,
        exec_mode: "mount",
        suite_id: suite!.id,
      });
      expect(run.id).toBeTruthy();

      // Agent runs take longer — 10 min timeout
      const finalRun = await waitForRunCompletion(run.id, 600_000);
      const results = await getRunResults(run.id);

      await attachTestContext(testInfo, "run_result", {
        status: finalRun.status,
        error_message: finalRun.error_message,
        results_count: results.length,
      });

      expect(finalRun.status, `Run failed: ${finalRun.error_message ?? "unknown"}`).toBe(
        "completed",
      );
      expect(results.length).toBeGreaterThanOrEqual(1);

      // Agent results may have trajectory/sparc scores
      for (const r of results) {
        const scores = Object.values(r.scores ?? {});
        expect(scores.length, `No scores for task ${r.task_id}`).toBeGreaterThan(0);
      }
    });
  }
});

// ============================================================================
// Block 4: Intelligent Routing (Auto Router)
// ============================================================================

test.describe("Block 4: Intelligent Routing", () => {
  test("[4.1] model=auto with HybridRouter", async ({}, testInfo) => {
    const env = await collectEnvironmentInfo();
    await attachTestContext(testInfo, "environment", env);

    const dataset = suiteToDataset("codeforge_simple");
    const suite = await getSuiteByProvider("codeforge_simple");
    expect(suite, "Suite for codeforge_simple not found").toBeTruthy();

    const run = await createBenchmarkRun({
      dataset,
      model: "auto",
      metrics: ["llm_judge"],
      benchmark_type: "simple",
      exec_mode: "mount",
      suite_id: suite!.id,
    });
    expect(run.id).toBeTruthy();
    expect(run.model).toBe("auto");

    // Wait for completion — routing may select any available model
    const finalRun = await waitForRunCompletion(run.id, 600_000);
    const results = await getRunResults(run.id);

    await attachTestContext(testInfo, "run_result", {
      status: finalRun.status,
      error_message: finalRun.error_message,
      selected_model: finalRun.selected_model,
      routing_reason: finalRun.routing_reason,
      results_count: results.length,
      results: results.map((r) => ({
        task_id: r.task_id,
        scores: r.scores,
      })),
    });

    // Run must reach a terminal state
    expect(
      ["completed", "failed"].includes(finalRun.status),
      `Run stuck in ${finalRun.status}`,
    ).toBe(true);

    if (finalRun.status === "completed") {
      expect(results.length).toBeGreaterThanOrEqual(1);
      // Router made a decision — results should have scores
      for (const r of results) {
        expect(Object.keys(r.scores ?? {}).length).toBeGreaterThan(0);
      }
    } else {
      // If failed, error should mention routing
      expect(
        finalRun.error_message?.toLowerCase() ?? "",
        "Failed run should mention routing in error",
      ).toContain("routing");
    }
  });
});

// ============================================================================
// Block 5: Error Scenarios
// ============================================================================

test.describe("Block 5: Error Scenarios", () => {
  test("[5.1] Invalid dataset → fast failure with error message", async ({}, testInfo) => {
    const { status, body } = await createBenchmarkRunRaw(ERROR_SCENARIOS[0].params);
    await attachTestContext(testInfo, "response", { status, body });

    // Should fail at Go validation or after NATS dispatch
    if (status >= 400) {
      // Go rejected it directly — good
      expect(status).toBe(400);
    } else {
      // Run was created but should fail quickly
      const runBody = body as { id: string };
      const finalRun = await waitForRunCompletion(runBody.id, 30_000);
      expect(finalRun.status).toBe("failed");
      expect(finalRun.error_message).toBeTruthy();
    }
  });

  test("[5.2] Invalid model → failure with clear error", async ({}, testInfo) => {
    const { status, body } = await createBenchmarkRunRaw(ERROR_SCENARIOS[1].params);
    await attachTestContext(testInfo, "response", { status, body });

    if (status >= 400) {
      expect(status).toBeLessThan(500);
    } else {
      const runBody = body as { id: string };
      const finalRun = await waitForRunCompletion(runBody.id, 60_000);
      expect(finalRun.status).toBe("failed");
      expect(finalRun.error_message).toBeTruthy();
      expect(finalRun.error_message!.toLowerCase()).toContain("model");
    }
  });

  test("[5.3] Empty dataset → graceful handling", async ({}, testInfo) => {
    const { status, body } = await createBenchmarkRunRaw(ERROR_SCENARIOS[2].params);
    await attachTestContext(testInfo, "response", { status, body });

    // "empty-test" dataset doesn't exist → should fail at dataset resolution
    if (status >= 400) {
      expect(status).toBe(400);
    } else {
      const runBody = body as { id: string };
      const finalRun = await waitForRunCompletion(runBody.id, 30_000);
      expect(finalRun.status).toBe("failed");
    }
  });

  test("[5.4] Unknown evaluator → rejected by validation", async ({}, testInfo) => {
    const { status, body } = await createBenchmarkRunRaw(ERROR_SCENARIOS[3].params);
    await attachTestContext(testInfo, "response", { status, body });

    // Go ValidMetrics rejects unknown metrics → HTTP 400
    expect(status).toBe(400);
  });

  test("[5.5] Duplicate run → idempotent processing", async ({}, testInfo) => {
    // Start first run
    const run1 = await createBenchmarkRun({
      dataset: "e2e-quick",
      model: DEFAULT_MODEL,
      metrics: ["llm_judge"],
      benchmark_type: "simple",
    });

    // Start second run with same params (different run ID)
    const run2 = await createBenchmarkRun({
      dataset: "e2e-quick",
      model: DEFAULT_MODEL,
      metrics: ["llm_judge"],
      benchmark_type: "simple",
    });

    await attachTestContext(testInfo, "runs", { run1_id: run1.id, run2_id: run2.id });

    // Both should be separate runs (different IDs)
    expect(run1.id).not.toBe(run2.id);

    // Both should eventually complete
    const [final1, final2] = await Promise.all([
      waitForRunCompletion(run1.id, 600_000),
      waitForRunCompletion(run2.id, 600_000),
    ]);

    expect(final1.status).toBe("completed");
    expect(final2.status).toBe("completed");

    // Clean up
    await deleteRun(run1.id);
    await deleteRun(run2.id);
  });
});

// ============================================================================
// Block 6: Multi-Metric & Extended Metric Names
// ============================================================================

test.describe("Block 6: Multi-Metric & Extended Scenarios", () => {
  const cases = getBlockCases(6);

  for (const tc of cases) {
    test(`[${tc.id}] ${tc.metrics.join("+")}`, async ({}, testInfo) => {
      const env = await collectEnvironmentInfo();
      await attachTestContext(testInfo, "environment", env);

      const model = tc.model ?? DEFAULT_MODEL;
      const dataset = suiteToDataset(tc.suite);

      const suite = await getSuiteByProvider(tc.suite);
      expect(suite, `Suite for provider ${tc.suite} not found`).toBeTruthy();

      const run = await createBenchmarkRun({
        dataset,
        model,
        metrics: tc.metrics as string[],
        benchmark_type: tc.type,
        exec_mode: "mount",
        suite_id: suite!.id,
      });
      expect(run.id).toBeTruthy();

      const finalRun = await waitForRunCompletion(run.id, 600_000);
      const results = await getRunResults(run.id);

      await attachTestContext(testInfo, "run_result", {
        status: finalRun.status,
        error_message: finalRun.error_message,
        results_count: results.length,
        results: results.map((r) => ({
          task_id: r.task_id,
          scores: r.scores,
        })),
      });

      expect(finalRun.status, `Run failed: ${finalRun.error_message ?? "unknown"}`).toBe(
        "completed",
      );
      expect(results.length).toBeGreaterThanOrEqual(1);

      // Verify scores exist for each task
      for (const r of results) {
        const scoreKeys = Object.keys(r.scores ?? {});
        expect(scoreKeys.length, `No scores for task ${r.task_id}`).toBeGreaterThan(0);
      }
    });
  }
});
