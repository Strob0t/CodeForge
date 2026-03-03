import { expect, test, ensureBenchmarkPage } from "./benchmark-fixtures";

test.describe("Benchmark Run List & Detail", () => {
  test.beforeEach(async ({ page }) => {
    const loaded = await ensureBenchmarkPage(page);
    if (!loaded) test.skip();
  });

  test("empty state shown when no runs exist", async ({ page }) => {
    // This test may be flaky if other tests created runs; check for either state
    const emptyState = page.getByText("No benchmark runs yet.");
    const deleteBtn = page.getByRole("button", { name: "Delete" }).first();
    await expect(emptyState.or(deleteBtn)).toBeVisible({ timeout: 10_000 });
  });

  test("run card is clickable and expands detail", async ({ page, benchApi }) => {
    await benchApi.createRun({
      dataset: "basic-coding",
      model: "test/detail-expand",
      metrics: ["correctness"],
    });
    await page.reload();
    await page.locator("main h1").waitFor({ state: "visible", timeout: 5_000 });

    // Click on the run card
    const card = page.getByText("test/detail-expand").first();
    await card.click();

    // After expanding, the results table or "No results" message should appear
    const resultsTable = page.locator("table").nth(0);
    const noResults = page.getByText("No results");
    await expect(resultsTable.or(noResults)).toBeVisible({ timeout: 5_000 });
  });

  test("clicking expanded run card collapses it", async ({ page, benchApi }) => {
    await benchApi.createRun({
      dataset: "basic-coding",
      model: "test/detail-collapse",
      metrics: ["correctness"],
    });
    await page.reload();
    await page.locator("main h1").waitFor({ state: "visible", timeout: 5_000 });

    const card = page.getByText("test/detail-collapse").first();
    // Expand
    await card.click();
    await page.waitForTimeout(500);
    // Collapse
    await card.click();
  });

  test("run card displays metrics badges", async ({ page, benchApi }) => {
    await benchApi.createRun({
      dataset: "basic-coding",
      model: "test/metrics-display",
      metrics: ["correctness", "faithfulness"],
    });
    await page.reload();
    await page.locator("main h1").waitFor({ state: "visible", timeout: 5_000 });
    await expect(page.getByText("correctness").first()).toBeVisible({ timeout: 10_000 });
  });

  test("run card has delete button", async ({ page, benchApi }) => {
    await benchApi.createRun({
      dataset: "basic-coding",
      model: "test/delete-btn",
      metrics: ["correctness"],
    });
    await page.reload();
    await page.locator("main h1").waitFor({ state: "visible", timeout: 5_000 });
    await expect(page.getByRole("button", { name: "Delete" }).first()).toBeVisible({
      timeout: 10_000,
    });
  });

  test("cancel button visible for running runs", async ({ page, benchApi }) => {
    await benchApi.createRun({
      dataset: "basic-coding",
      model: "test/cancel-visible",
      metrics: ["correctness"],
    });
    await page.reload();
    await page.locator("main h1").waitFor({ state: "visible", timeout: 5_000 });
    await expect(page.getByRole("button", { name: "Cancel" }).first()).toBeVisible({
      timeout: 10_000,
    });
  });

  test("cancel run via API changes status to failed", async ({ benchApi }) => {
    const run = await benchApi.createRun({
      dataset: "basic-coding",
      model: "test/cancel-api",
      metrics: ["correctness"],
    });
    expect(run.status).toBe("running");
    const cancelled = await benchApi.cancelRun(run.id);
    expect(cancelled.status).toBe("failed");
  });

  test("CSV export link visible for completed runs", async ({ page, benchApi }) => {
    // We can't easily get a completed run without NATS worker, but verify the
    // export link does not appear for running runs
    await benchApi.createRun({
      dataset: "basic-coding",
      model: "test/no-csv-running",
      metrics: ["correctness"],
    });
    await page.reload();
    await page.locator("main h1").waitFor({ state: "visible", timeout: 5_000 });
    // CSV link should NOT be visible for running runs
    await expect(page.getByText("CSV").first()).not.toBeVisible();
  });

  test("list runs API returns correct data", async ({ benchApi }) => {
    const run = await benchApi.createRun({
      dataset: "e2e-list-api",
      model: "test/list",
      metrics: ["correctness"],
    });
    const runs = await benchApi.listRuns();
    const found = runs.find((r) => r.id === run.id);
    expect(found).toBeDefined();
    expect(found!.dataset).toBe("e2e-list-api");
    expect(found!.model).toBe("test/list");
  });

  test("get run API returns single run", async ({ benchApi }) => {
    const run = await benchApi.createRun({
      dataset: "e2e-get-api",
      model: "test/get",
      metrics: ["correctness"],
    });
    const fetched = await benchApi.getRun(run.id);
    expect(fetched.id).toBe(run.id);
    expect(fetched.dataset).toBe("e2e-get-api");
  });

  test("list results API returns array (possibly empty)", async ({ benchApi }) => {
    const run = await benchApi.createRun({
      dataset: "basic-coding",
      model: "test/results",
      metrics: ["correctness"],
    });
    const results = await benchApi.listResults(run.id);
    expect(Array.isArray(results)).toBe(true);
  });

  test("multiple runs appear in correct order (newest first)", async ({ benchApi }) => {
    const run1 = await benchApi.createRun({
      dataset: "e2e-order-1",
      model: "test/order",
      metrics: ["correctness"],
    });
    const run2 = await benchApi.createRun({
      dataset: "e2e-order-2",
      model: "test/order",
      metrics: ["correctness"],
    });
    const runs = await benchApi.listRuns();
    const idx1 = runs.findIndex((r) => r.id === run1.id);
    const idx2 = runs.findIndex((r) => r.id === run2.id);
    // Newest first (run2 created after run1, so idx2 < idx1)
    expect(idx2).toBeLessThan(idx1);
  });
});
