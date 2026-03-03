import { expect, test, ensureBenchmarkPage, clickTab } from "./benchmark-fixtures";

test.describe("Benchmark Compare - 2-way & N-way", () => {
  test.beforeEach(async ({ page }) => {
    const loaded = await ensureBenchmarkPage(page);
    if (!loaded) test.skip();
  });

  // --- 2-Way Compare (Runs tab) ---

  test("compare section visible on Runs tab", async ({ page }) => {
    // Compare section header or empty state
    const compareText = page.getByText("Compare Runs");
    const emptyState = page.getByText("No benchmark runs yet.");
    await expect(compareText.or(emptyState)).toBeVisible({ timeout: 10_000 });
  });

  test("compare API requires 2 run IDs", async ({ benchApi }) => {
    const run = await benchApi.createRun({
      dataset: "basic-coding",
      model: "test/compare-single",
      metrics: ["correctness"],
    });
    try {
      await benchApi.compareMulti([run.id]);
      expect(true).toBe(false); // Should not reach
    } catch (err) {
      expect(String(err)).toContain("400");
    }
  });

  test("compare API works with 2 runs", async ({ benchApi }) => {
    const run1 = await benchApi.createRun({
      dataset: "basic-coding",
      model: "test/compare-a",
      metrics: ["correctness"],
    });
    const run2 = await benchApi.createRun({
      dataset: "basic-coding",
      model: "test/compare-b",
      metrics: ["correctness"],
    });
    const result = await benchApi.compareMulti([run1.id, run2.id]);
    expect(Array.isArray(result)).toBe(true);
    expect(result.length).toBe(2);
  });

  // --- Multi-Compare Tab ---

  test("Multi-Compare tab has run selector with checkboxes", async ({ page }) => {
    await clickTab(page, "Multi-Compare");
    await expect(page.getByText("Select runs to compare")).toBeVisible();
  });

  test("Multi-Compare compare button disabled with < 2 selections", async ({ page }) => {
    await clickTab(page, "Multi-Compare");
    // The compare button should be present but disabled
    const btn = page.getByRole("button", { name: /Compare Selected/i });
    await expect(btn).toBeVisible();
    await expect(btn).toBeDisabled();
  });

  test("Multi-Compare shows empty state initially", async ({ page }) => {
    await clickTab(page, "Multi-Compare");
    await expect(page.getByText("Select two or more runs")).toBeVisible();
  });

  test("compare multi API works with 3 runs", async ({ benchApi }) => {
    const run1 = await benchApi.createRun({
      dataset: "basic-coding",
      model: "test/multi-a",
      metrics: ["correctness"],
    });
    const run2 = await benchApi.createRun({
      dataset: "basic-coding",
      model: "test/multi-b",
      metrics: ["correctness"],
    });
    const run3 = await benchApi.createRun({
      dataset: "basic-coding",
      model: "test/multi-c",
      metrics: ["correctness"],
    });
    const result = await benchApi.compareMulti([run1.id, run2.id, run3.id]);
    expect(result.length).toBe(3);
  });

  test("compare multi returns run and results per entry", async ({ benchApi }) => {
    const run1 = await benchApi.createRun({
      dataset: "basic-coding",
      model: "test/entry-a",
      metrics: ["correctness"],
    });
    const run2 = await benchApi.createRun({
      dataset: "basic-coding",
      model: "test/entry-b",
      metrics: ["correctness"],
    });
    const result = await benchApi.compareMulti([run1.id, run2.id]);
    for (const entry of result as Array<{ run: { id: string }; results: unknown[] }>) {
      expect(entry.run).toBeDefined();
      expect(entry.run.id).toBeTruthy();
      expect(Array.isArray(entry.results)).toBe(true);
    }
  });

  test("compare multi with non-existent run returns error", async ({ benchApi }) => {
    const run1 = await benchApi.createRun({
      dataset: "basic-coding",
      model: "test/exists",
      metrics: ["correctness"],
    });
    try {
      await benchApi.compareMulti([run1.id, "non-existent-id"]);
      expect(true).toBe(false);
    } catch (err) {
      expect(String(err)).toContain("500");
    }
  });

  test("Multi-Compare checkbox list shows created runs", async ({ page, benchApi }) => {
    await benchApi.createRun({
      dataset: "basic-coding",
      model: "test/checkbox-vis",
      metrics: ["correctness"],
    });
    await clickTab(page, "Multi-Compare");
    await page.reload();
    await page.locator("main h1").waitFor({ state: "visible", timeout: 5_000 });
    await clickTab(page, "Multi-Compare");
    await expect(page.getByText("test/checkbox-vis").first()).toBeVisible({ timeout: 10_000 });
  });

  test("Multi-Compare shows cost and duration rows in table", async ({ benchApi }) => {
    const run1 = await benchApi.createRun({
      dataset: "basic-coding",
      model: "test/table-a",
      metrics: ["correctness"],
    });
    const run2 = await benchApi.createRun({
      dataset: "basic-coding",
      model: "test/table-b",
      metrics: ["correctness"],
    });
    const result = await benchApi.compareMulti([run1.id, run2.id]);
    // Each entry should have run with cost/duration fields
    for (const entry of result as Array<{
      run: { total_cost: number; total_duration_ms: number };
    }>) {
      expect(typeof entry.run.total_cost).toBe("number");
      expect(typeof entry.run.total_duration_ms).toBe("number");
    }
  });

  test("compare multi preserves order of input run IDs", async ({ benchApi }) => {
    const run1 = await benchApi.createRun({
      dataset: "basic-coding",
      model: "test/order-a",
      metrics: ["correctness"],
    });
    const run2 = await benchApi.createRun({
      dataset: "basic-coding",
      model: "test/order-b",
      metrics: ["correctness"],
    });
    const result = (await benchApi.compareMulti([run1.id, run2.id])) as Array<{
      run: { id: string };
    }>;
    expect(result[0].run.id).toBe(run1.id);
    expect(result[1].run.id).toBe(run2.id);
  });
});
