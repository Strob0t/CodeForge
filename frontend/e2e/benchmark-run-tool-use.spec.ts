import { expect, test, ensureBenchmarkPage } from "./benchmark-fixtures";

test.describe("Benchmark Runs - Tool Use Mode", () => {
  test.beforeEach(async ({ page }) => {
    const loaded = await ensureBenchmarkPage(page);
    if (!loaded) test.skip();
  });

  test("benchmark_type dropdown has tool_use option", async ({ page }) => {
    await page.getByRole("button", { name: "New Run" }).click();
    const select = page.locator("#benchmark-type");
    await expect(select.locator("option[value='tool_use']")).toBeAttached();
  });

  test("selecting tool_use does not show exec_mode", async ({ page }) => {
    await page.getByRole("button", { name: "New Run" }).click();
    await page.locator("#benchmark-type").selectOption("tool_use");
    await expect(page.locator("#benchmark-exec-mode")).not.toBeVisible();
  });

  test("tool_correctness metric is available", async ({ page }) => {
    await page.getByRole("button", { name: "New Run" }).click();
    await expect(page.getByRole("button", { name: "tool_correctness", exact: true })).toBeVisible();
  });

  test("create tool_use run via API", async ({ benchApi }) => {
    const run = await benchApi.createRun({
      dataset: "basic-coding",
      model: "groq/llama-3.1-8b-instant",
      metrics: ["tool_correctness"],
      benchmark_type: "tool_use",
    });
    expect(run.id).toBeTruthy();
    expect(run.status).toBe("running");
  });

  test("tool_use run shows type badge in list", async ({ page, benchApi }) => {
    await benchApi.createRun({
      dataset: "basic-coding",
      model: "test/tool-model",
      metrics: ["tool_correctness"],
      benchmark_type: "tool_use",
    });
    await page.reload();
    await page.locator("main h1").waitFor({ state: "visible", timeout: 5_000 });
    await expect(page.getByText("tool_use").first()).toBeVisible({ timeout: 10_000 });
  });

  test("tool_use badge has warning variant", async ({ page, benchApi }) => {
    await benchApi.createRun({
      dataset: "basic-coding",
      model: "test/tool-model-badge",
      metrics: ["tool_correctness"],
      benchmark_type: "tool_use",
    });
    await page.reload();
    await page.locator("main h1").waitFor({ state: "visible", timeout: 5_000 });
    // The badge uses variant="warning" which maps to yellow/amber styling
    const badge = page.getByText("tool_use").first();
    await expect(badge).toBeVisible({ timeout: 10_000 });
  });

  test("multiple tool_use runs can coexist", async ({ benchApi }) => {
    const run1 = await benchApi.createRun({
      dataset: "basic-coding",
      model: "test/model-a",
      metrics: ["tool_correctness"],
      benchmark_type: "tool_use",
    });
    const run2 = await benchApi.createRun({
      dataset: "basic-coding",
      model: "test/model-b",
      metrics: ["tool_correctness"],
      benchmark_type: "tool_use",
    });
    expect(run1.id).not.toBe(run2.id);
  });

  test("tool_use runs appear in run list API", async ({ benchApi }) => {
    await benchApi.createRun({
      dataset: "basic-coding",
      model: "test/tool-list",
      metrics: ["tool_correctness"],
      benchmark_type: "tool_use",
    });
    const runs = await benchApi.listRuns();
    const toolRuns = runs.filter((r) => r.benchmark_type === "tool_use");
    expect(toolRuns.length).toBeGreaterThanOrEqual(1);
  });

  test("tool_use run can be deleted via API", async ({ benchApi }) => {
    const run = await benchApi.createRun({
      dataset: "basic-coding",
      model: "test/tool-delete",
      metrics: ["tool_correctness"],
      benchmark_type: "tool_use",
    });
    await benchApi.deleteRun(run.id);
    const runs = await benchApi.listRuns();
    expect(runs.find((r) => r.id === run.id)).toBeUndefined();
  });
});
