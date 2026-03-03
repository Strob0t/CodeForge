import { expect, test } from "./benchmark-fixtures";

/**
 * Cleanup tests — delete all e2e-created benchmark entities.
 * These should run LAST in the benchmark test suite.
 */
test.describe("Benchmark Cleanup", () => {
  test("delete all e2e benchmark runs", async ({ benchApi }) => {
    const runs = await benchApi.listRuns();
    const e2eRuns = runs.filter(
      (r) =>
        r.model?.startsWith("test/") ||
        r.dataset?.startsWith("e2e-") ||
        r.model?.startsWith("groq/"),
    );
    for (const run of e2eRuns) {
      try {
        await benchApi.deleteRun(run.id);
      } catch {
        // best-effort
      }
    }
    const remaining = await benchApi.listRuns();
    const stillE2e = remaining.filter(
      (r) =>
        r.model?.startsWith("test/") ||
        r.dataset?.startsWith("e2e-") ||
        r.model?.startsWith("groq/"),
    );
    expect(stillE2e.length).toBe(0);
  });

  test("delete all e2e benchmark suites", async ({ benchApi }) => {
    const suites = await benchApi.listSuites();
    const e2eSuites = suites.filter((s) => s.name?.startsWith("e2e-"));
    for (const suite of e2eSuites) {
      try {
        await benchApi.deleteSuite(suite.id);
      } catch {
        // best-effort
      }
    }
    const remaining = await benchApi.listSuites();
    const stillE2e = remaining.filter((s) => s.name?.startsWith("e2e-"));
    expect(stillE2e.length).toBe(0);
  });

  test("verify clean state after cleanup", async ({ benchApi }) => {
    const runs = await benchApi.listRuns();
    const suites = await benchApi.listSuites();
    // Just verify the APIs work and return arrays
    expect(Array.isArray(runs)).toBe(true);
    expect(Array.isArray(suites)).toBe(true);
  });
});
