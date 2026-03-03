import { expect, test } from "./benchmark-fixtures";

/**
 * Full orchestration workflow tests — API-level.
 * Tests the complete lifecycle: suites → runs → compare → leaderboard → cost analysis → export.
 */
test.describe("Benchmark Orchestrator - Full Workflow", () => {
  test("create suite and link run to it", async ({ benchApi }) => {
    const suite = await benchApi.createSuite({
      name: "e2e-orch-suite",
      type: "deepeval",
      provider_name: "deepeval",
    });
    const run = await benchApi.createRun({
      dataset: "basic-coding",
      model: "test/orch-linked",
      metrics: ["correctness"],
      suite_id: suite.id,
    });
    expect(run.id).toBeTruthy();
  });

  test("create multiple runs with different models", async ({ benchApi }) => {
    const run1 = await benchApi.createRun({
      dataset: "basic-coding",
      model: "test/orch-model-a",
      metrics: ["correctness"],
    });
    const run2 = await benchApi.createRun({
      dataset: "basic-coding",
      model: "test/orch-model-b",
      metrics: ["correctness"],
    });
    expect(run1.id).not.toBe(run2.id);
    expect(run1.model).toBe("test/orch-model-a");
    expect(run2.model).toBe("test/orch-model-b");
  });

  test("compare 2 runs from same dataset", async ({ benchApi }) => {
    const run1 = await benchApi.createRun({
      dataset: "basic-coding",
      model: "test/orch-cmp-a",
      metrics: ["correctness"],
    });
    const run2 = await benchApi.createRun({
      dataset: "basic-coding",
      model: "test/orch-cmp-b",
      metrics: ["correctness"],
    });
    const comparison = (await benchApi.compareMulti([run1.id, run2.id])) as Array<{
      run: { id: string };
    }>;
    expect(comparison.length).toBe(2);
    expect(comparison[0].run.id).toBe(run1.id);
    expect(comparison[1].run.id).toBe(run2.id);
  });

  test("compare 3 runs (N-way)", async ({ benchApi }) => {
    const runs = await Promise.all([
      benchApi.createRun({
        dataset: "basic-coding",
        model: "test/nway-a",
        metrics: ["correctness"],
      }),
      benchApi.createRun({
        dataset: "basic-coding",
        model: "test/nway-b",
        metrics: ["correctness"],
      }),
      benchApi.createRun({
        dataset: "basic-coding",
        model: "test/nway-c",
        metrics: ["correctness"],
      }),
    ]);
    const comparison = await benchApi.compareMulti(runs.map((r) => r.id));
    expect((comparison as unknown[]).length).toBe(3);
  });

  test("leaderboard available after creating runs", async ({ benchApi }) => {
    await benchApi.createRun({
      dataset: "basic-coding",
      model: "test/lb-run",
      metrics: ["correctness"],
    });
    const entries = await benchApi.leaderboard();
    expect(Array.isArray(entries)).toBe(true);
  });

  test("cost analysis available for created run", async ({ benchApi }) => {
    const run = await benchApi.createRun({
      dataset: "basic-coding",
      model: "test/cost-run",
      metrics: ["correctness"],
    });
    const analysis = (await benchApi.costAnalysis(run.id)) as { run_id: string };
    expect(analysis.run_id).toBe(run.id);
  });

  test("export results URL is valid for created run", async ({ benchApi }) => {
    const run = await benchApi.createRun({
      dataset: "basic-coding",
      model: "test/export-run",
      metrics: ["correctness"],
    });
    const csvUrl = benchApi.exportResultsUrl(run.id, "csv");
    expect(csvUrl).toContain(run.id);
    const jsonUrl = benchApi.exportResultsUrl(run.id, "json");
    expect(jsonUrl).toContain(run.id);
  });

  test("full lifecycle: create suite → run → cost → compare → cleanup", async ({ benchApi }) => {
    // 1. Create suite
    const suite = await benchApi.createSuite({
      name: "e2e-lifecycle-suite",
      type: "deepeval",
      provider_name: "deepeval",
    });

    // 2. Create runs
    const run1 = await benchApi.createRun({
      dataset: "basic-coding",
      model: "test/lifecycle-a",
      metrics: ["correctness"],
      suite_id: suite.id,
    });
    const run2 = await benchApi.createRun({
      dataset: "basic-coding",
      model: "test/lifecycle-b",
      metrics: ["correctness"],
      suite_id: suite.id,
    });

    // 3. Cost analysis
    const cost = (await benchApi.costAnalysis(run1.id)) as { run_id: string };
    expect(cost.run_id).toBe(run1.id);

    // 4. Compare
    const comparison = await benchApi.compareMulti([run1.id, run2.id]);
    expect((comparison as unknown[]).length).toBe(2);

    // 5. Leaderboard (with suite filter)
    const lb = await benchApi.leaderboard(suite.id);
    expect(Array.isArray(lb)).toBe(true);

    // 6. Cleanup is automatic via fixture teardown
  });

  test("cancel run and verify status change", async ({ benchApi }) => {
    const run = await benchApi.createRun({
      dataset: "basic-coding",
      model: "test/cancel-lifecycle",
      metrics: ["correctness"],
    });
    expect(run.status).toBe("running");

    const cancelled = await benchApi.cancelRun(run.id);
    expect(cancelled.status).toBe("failed");

    // Verify via GET
    const fetched = await benchApi.getRun(run.id);
    expect(fetched.status).toBe("failed");
  });

  test("update suite and verify changes persist", async ({ benchApi }) => {
    const suite = await benchApi.createSuite({
      name: "e2e-persist-suite",
      type: "deepeval",
      provider_name: "deepeval",
    });
    await benchApi.updateSuite(suite.id, {
      name: "e2e-persist-updated",
      description: "Updated via orchestrator test",
    });
    const suites = await benchApi.listSuites();
    const found = suites.find((s) => s.id === suite.id);
    expect(found).toBeDefined();
    expect(found!.name).toBe("e2e-persist-updated");
  });

  test("datasets API returns array", async ({ benchApi }) => {
    const datasets = await benchApi.listDatasets();
    expect(Array.isArray(datasets)).toBe(true);
  });
});
