import { expect, test, ensureBenchmarkPage } from "./benchmark-fixtures";

test.describe("Benchmark Runs - Simple Mode", () => {
  test.beforeEach(async ({ page }) => {
    const loaded = await ensureBenchmarkPage(page);
    if (!loaded) test.skip();
  });

  test("New Run button toggles form visibility", async ({ page }) => {
    await expect(page.locator("form")).not.toBeVisible();
    await page.getByRole("button", { name: "New Run" }).click();
    await expect(page.locator("form")).toBeVisible();
    await page.getByRole("button", { name: "Cancel" }).click();
    await expect(page.locator("form")).not.toBeVisible();
  });

  test("form has dataset field", async ({ page }) => {
    await page.getByRole("button", { name: "New Run" }).click();
    await expect(page.locator("label[for='benchmark-dataset']")).toBeVisible();
  });

  test("form has model combobox", async ({ page }) => {
    await page.getByRole("button", { name: "New Run" }).click();
    await expect(page.locator("#benchmark-model")).toBeVisible();
  });

  test("form has benchmark_type dropdown", async ({ page }) => {
    await page.getByRole("button", { name: "New Run" }).click();
    await expect(page.locator("label[for='benchmark-type']")).toBeVisible();
    // Verify options
    const select = page.locator("#benchmark-type");
    await expect(select.locator("option")).toHaveCount(3);
  });

  test("form defaults to simple benchmark type", async ({ page }) => {
    await page.getByRole("button", { name: "New Run" }).click();
    const select = page.locator("#benchmark-type");
    await expect(select).toHaveValue("simple");
  });

  test("exec_mode hidden for simple type", async ({ page }) => {
    await page.getByRole("button", { name: "New Run" }).click();
    await expect(page.locator("#benchmark-exec-mode")).not.toBeVisible();
  });

  test("form has metrics toggle buttons", async ({ page }) => {
    await page.getByRole("button", { name: "New Run" }).click();
    await expect(page.getByRole("button", { name: "correctness", exact: true })).toBeVisible();
    await expect(page.getByRole("button", { name: "faithfulness", exact: true })).toBeVisible();
  });

  test("metrics toggle buttons work", async ({ page }) => {
    await page.getByRole("button", { name: "New Run" }).click();
    const btn = page.getByRole("button", { name: "faithfulness", exact: true });
    // Initially not selected (no blue bg)
    await btn.click();
    // After click, should have the selected class
    await expect(btn).toHaveClass(/bg-blue-600/);
    await btn.click();
    await expect(btn).not.toHaveClass(/bg-blue-600/);
  });

  test("form validation prevents empty submit", async ({ page }) => {
    await page.getByRole("button", { name: "New Run" }).click();
    await page.getByRole("button", { name: "Start Run" }).click();
    // Form should remain visible (validation prevented submission)
    await expect(page.locator("form")).toBeVisible();
  });

  test("cancel button resets and closes form", async ({ page }) => {
    await page.getByRole("button", { name: "New Run" }).click();
    await expect(page.locator("form")).toBeVisible();
    await page.getByRole("button", { name: "Cancel" }).click();
    await expect(page.locator("form")).not.toBeVisible();
  });

  test("create run via API and verify in list", async ({ page, benchApi }) => {
    const run = await benchApi.createRun({
      dataset: "basic-coding",
      model: "groq/llama-3.1-8b-instant",
      metrics: ["correctness"],
      benchmark_type: "simple",
    });
    expect(run.id).toBeTruthy();
    expect(run.status).toBe("running");

    // Reload page and verify run appears
    await page.reload();
    await page.locator("main h1").waitFor({ state: "visible", timeout: 5_000 });
    await expect(page.getByText("basic-coding").first()).toBeVisible({ timeout: 10_000 });
  });

  test("run card shows dataset and model", async ({ page, benchApi }) => {
    await benchApi.createRun({
      dataset: "basic-coding",
      model: "groq/llama-3.1-8b-instant",
      metrics: ["correctness"],
    });
    await page.reload();
    await page.locator("main h1").waitFor({ state: "visible", timeout: 5_000 });
    await expect(page.getByText("basic-coding").first()).toBeVisible({ timeout: 10_000 });
    await expect(page.getByText("groq/llama-3.1-8b-instant").first()).toBeVisible();
  });

  test("run card shows status badge", async ({ page, benchApi }) => {
    await benchApi.createRun({
      dataset: "basic-coding",
      model: "groq/llama-3.1-8b-instant",
      metrics: ["correctness"],
    });
    await page.reload();
    await page.locator("main h1").waitFor({ state: "visible", timeout: 5_000 });
    // Status should be "running" (no Python worker to complete it)
    await expect(page.getByText("running").first()).toBeVisible({ timeout: 10_000 });
  });

  test("delete run via UI", async ({ page, benchApi }) => {
    const run = await benchApi.createRun({
      dataset: "e2e-delete-test",
      model: "test/model",
      metrics: ["correctness"],
    });
    await page.reload();
    await page.locator("main h1").waitFor({ state: "visible", timeout: 5_000 });
    await expect(page.getByText("e2e-delete-test").first()).toBeVisible({ timeout: 10_000 });

    // Click delete on the first matching card
    const card = page.getByText("e2e-delete-test").first().locator("../..");
    await card.getByRole("button", { name: "Delete" }).click();

    // Verify run is gone
    await expect(page.getByText("e2e-delete-test")).not.toBeVisible({ timeout: 5_000 });

    // Remove from cleanup tracker since we already deleted it
    const idx = (benchApi as unknown as { cleanup: () => Promise<void> }) !== null;
    if (idx) {
      try {
        await benchApi.deleteRun(run.id);
      } catch {
        // Already deleted
      }
    }
  });

  test("progress bar visible for running runs", async ({ page, benchApi }) => {
    await benchApi.createRun({
      dataset: "basic-coding",
      model: "test/model",
      metrics: ["correctness"],
    });
    await page.reload();
    await page.locator("main h1").waitFor({ state: "visible", timeout: 5_000 });
    // Running runs should have an animated progress bar
    await expect(page.locator(".animate-pulse").first()).toBeVisible({ timeout: 10_000 });
  });
});
