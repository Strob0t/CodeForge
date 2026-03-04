import { expect, test, ensureBenchmarkPage, clickTab } from "./benchmark-fixtures";

test.describe("Benchmark Cost Analysis", () => {
  test.beforeEach(async ({ page }) => {
    const loaded = await ensureBenchmarkPage(page);
    if (!loaded) test.skip();
  });

  test("Cost Analysis tab renders", async ({ page }) => {
    await clickTab(page, "Cost Analysis");
    await expect(page.getByText("Select Run")).toBeVisible();
  });

  test("run selector dropdown exists", async ({ page }) => {
    await clickTab(page, "Cost Analysis");
    await expect(page.locator("select").first()).toBeVisible();
  });

  test("empty state when no run selected", async ({ page }) => {
    await clickTab(page, "Cost Analysis");
    await expect(page.getByText("Select a run to view cost analysis")).toBeVisible();
  });

  test("run selector shows created runs", async ({ page, benchApi }) => {
    await benchApi.createRun({
      dataset: "e2e-cost-run",
      model: "test/cost-model",
      metrics: ["correctness"],
    });
    await clickTab(page, "Cost Analysis");
    await page.reload();
    await page.locator("main h1").waitFor({ state: "visible", timeout: 5_000 });
    await clickTab(page, "Cost Analysis");
    // Verify the run appears in the dropdown
    const select = page.locator("select").first();
    const options = select.locator("option");
    expect(await options.count()).toBeGreaterThanOrEqual(2); // "Select" + at least one run
  });

  test("cost analysis API returns data for a run", async ({ benchApi }) => {
    const run = await benchApi.createRun({
      dataset: "basic-coding",
      model: "test/cost-api",
      metrics: ["correctness"],
    });
    const analysis = (await benchApi.costAnalysis(run.id)) as {
      run_id: string;
      total_cost_usd: number;
    };
    expect(analysis.run_id).toBe(run.id);
    expect(typeof analysis.total_cost_usd).toBe("number");
  });

  test("cost analysis has expected fields", async ({ benchApi }) => {
    const run = await benchApi.createRun({
      dataset: "basic-coding",
      model: "test/cost-fields",
      metrics: ["correctness"],
    });
    const analysis = (await benchApi.costAnalysis(run.id)) as Record<string, unknown>;
    expect(analysis).toHaveProperty("run_id");
    expect(analysis).toHaveProperty("total_cost_usd");
    expect(analysis).toHaveProperty("avg_score");
    expect(analysis).toHaveProperty("cost_per_score_point");
    expect(analysis).toHaveProperty("token_efficiency");
    expect(analysis).toHaveProperty("total_tokens_in");
    expect(analysis).toHaveProperty("total_tokens_out");
  });

  test("export training data URL format (JSON)", async ({ benchApi }) => {
    const run = await benchApi.createRun({
      dataset: "basic-coding",
      model: "test/export-json",
      metrics: ["correctness"],
    });
    const url = benchApi.exportTrainingUrl(run.id, "json");
    expect(url).toContain(run.id);
    expect(url).toContain("format=json");
  });

  test("export training data URL format (JSONL)", async ({ benchApi }) => {
    const run = await benchApi.createRun({
      dataset: "basic-coding",
      model: "test/export-jsonl",
      metrics: ["correctness"],
    });
    const url = benchApi.exportTrainingUrl(run.id, "jsonl");
    expect(url).toContain(run.id);
    expect(url).toContain("format=jsonl");
  });

  test("export results URL format (CSV)", async ({ benchApi }) => {
    const run = await benchApi.createRun({
      dataset: "basic-coding",
      model: "test/export-csv",
      metrics: ["correctness"],
    });
    const url = benchApi.exportResultsUrl(run.id, "csv");
    expect(url).toContain(run.id);
    expect(url).toContain("format=csv");
  });

  test("export results URL format (JSON)", async ({ benchApi }) => {
    const run = await benchApi.createRun({
      dataset: "basic-coding",
      model: "test/export-result-json",
      metrics: ["correctness"],
    });
    const url = benchApi.exportResultsUrl(run.id, "json");
    expect(url).toContain(run.id);
    expect(url).toContain("format=json");
  });
});
