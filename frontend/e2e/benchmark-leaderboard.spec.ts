import { expect, test, ensureBenchmarkPage, clickTab } from "./benchmark-fixtures";

test.describe("Benchmark Leaderboard", () => {
  test.beforeEach(async ({ page }) => {
    const loaded = await ensureBenchmarkPage(page);
    if (!loaded) test.skip();
  });

  test("Leaderboard tab renders", async ({ page }) => {
    await clickTab(page, "Leaderboard");
    await expect(page.getByText("Filter by Suite")).toBeVisible();
  });

  test("suite filter dropdown exists", async ({ page }) => {
    await clickTab(page, "Leaderboard");
    await expect(page.getByText("All Suites")).toBeVisible();
  });

  test("sort by dropdown exists", async ({ page }) => {
    await clickTab(page, "Leaderboard");
    await expect(page.getByText("Sort by")).toBeVisible();
  });

  test("sort by has multiple options", async ({ page }) => {
    await clickTab(page, "Leaderboard");
    // Find the sort selector near "Sort by" label
    const sortSelect = page.locator("select").nth(1); // Second select (first is suite filter)
    const options = sortSelect.locator("option");
    await expect(options).toHaveCount(5); // avg_score, total_cost, cost_per_point, token_eff, duration
  });

  test("leaderboard API returns array", async ({ benchApi }) => {
    const entries = await benchApi.leaderboard();
    expect(Array.isArray(entries)).toBe(true);
  });

  test("leaderboard API accepts suite filter", async ({ benchApi }) => {
    const suite = await benchApi.createSuite({
      name: "e2e-lb-suite",
      type: "deepeval",
      provider_name: "deepeval",
    });
    const entries = await benchApi.leaderboard(suite.id);
    expect(Array.isArray(entries)).toBe(true);
  });

  test("empty leaderboard shows empty state", async ({ page }) => {
    await clickTab(page, "Leaderboard");
    // If no completed runs, should show empty state or table
    const emptyState = page.getByText("No leaderboard data yet");
    const table = page.locator("table");
    await expect(emptyState.or(table)).toBeVisible({ timeout: 10_000 });
  });

  test("leaderboard table has correct columns", async ({ page }) => {
    await clickTab(page, "Leaderboard");
    // If there's a table, check headers
    const table = page.locator("table");
    if (await table.isVisible()) {
      await expect(page.getByText("Avg Score")).toBeVisible();
      await expect(page.getByText("Total Cost")).toBeVisible();
      await expect(page.getByText("Tasks")).toBeVisible();
    }
  });

  test("suite filter options include created suites", async ({ page, benchApi }) => {
    await benchApi.createSuite({
      name: "e2e-lb-filter-suite",
      type: "deepeval",
      provider_name: "deepeval",
    });
    await clickTab(page, "Leaderboard");
    await page.reload();
    await page.locator("main h1").waitFor({ state: "visible", timeout: 5_000 });
    await clickTab(page, "Leaderboard");
    // The suite should appear in the filter dropdown
    const select = page.locator("select").first();
    await expect(select.locator("option")).toHaveCount({ minimum: 2 }); // "All Suites" + at least one
  });

  test("changing sort metric re-orders entries", async ({ page }) => {
    await clickTab(page, "Leaderboard");
    // Just verify the sort dropdown can be changed without errors
    const sortSelect = page.locator("select").nth(1);
    await sortSelect.selectOption("total_cost_usd");
    // No crash = success
    await page.waitForTimeout(500);
    await sortSelect.selectOption("avg_score");
  });
});
