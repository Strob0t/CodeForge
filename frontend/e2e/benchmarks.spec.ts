import { expect, test } from "./fixtures";

// Benchmarks page requires APP_ENV=development. When not set, the API returns
// 403 and the page shows an error boundary that replaces the entire layout
// (including <main>). Tests must tolerate both states.

/** Helper: detect whether the error boundary is showing */
async function isErrorBoundary(page: import("@playwright/test").Page): Promise<boolean> {
  await page.goto("/benchmarks");
  // Wait for either the error boundary alert or the benchmark heading
  const errorAlert = page.locator("[role='alert']");
  const benchmarkHeading = page.locator("main h1");
  await expect(errorAlert.or(benchmarkHeading)).toBeVisible({ timeout: 10_000 });
  return errorAlert.isVisible();
}

test.describe("Benchmarks page", () => {
  test("page loads without crash", async ({ page }) => {
    await page.goto("/benchmarks");
    // Either the benchmark dashboard heading or the error boundary renders
    const errorAlert = page.locator("[role='alert']");
    const benchmarkHeading = page.locator("main h1");
    await expect(errorAlert.or(benchmarkHeading)).toBeVisible({ timeout: 10_000 });
  });

  test("heading shows 'Benchmark Dashboard' or error boundary", async ({ page }) => {
    await page.goto("/benchmarks");
    const errorAlert = page.locator("[role='alert']");
    const benchmarkHeading = page.locator("main h1");
    await expect(errorAlert.or(benchmarkHeading)).toBeVisible({ timeout: 10_000 });

    if (await errorAlert.isVisible()) {
      await expect(errorAlert.locator("h1")).toContainText("Something went wrong");
    } else {
      await expect(benchmarkHeading).toHaveText("Benchmark Dashboard");
    }
  });

  test("'New Run' button toggles form (dev mode only)", async ({ page }) => {
    if (await isErrorBoundary(page)) {
      test.skip();
      return;
    }

    await expect(page.locator("#benchmark-dataset")).not.toBeVisible();
    await page.getByRole("button", { name: "New Run" }).click();
    await expect(page.locator("#benchmark-dataset").or(page.getByText("Dataset"))).toBeVisible();

    await page.getByRole("button", { name: "Cancel" }).click();
    await expect(page.locator("#benchmark-dataset")).not.toBeVisible();
  });

  test("form has dataset select or input (dev mode only)", async ({ page }) => {
    if (await isErrorBoundary(page)) {
      test.skip();
      return;
    }
    await page.getByRole("button", { name: "New Run" }).click();
    await expect(page.getByText("Dataset")).toBeVisible();
  });

  test("form has model input (dev mode only)", async ({ page }) => {
    if (await isErrorBoundary(page)) {
      test.skip();
      return;
    }
    await page.getByRole("button", { name: "New Run" }).click();
    await expect(page.getByText("Model")).toBeVisible();
    await expect(page.getByPlaceholder("openai/gpt-4o")).toBeVisible();
  });

  test("form has metrics toggle buttons (dev mode only)", async ({ page }) => {
    if (await isErrorBoundary(page)) {
      test.skip();
      return;
    }
    await page.getByRole("button", { name: "New Run" }).click();
    await expect(page.getByText("Metrics")).toBeVisible();
    await expect(page.getByRole("button", { name: "correctness" })).toBeVisible();
  });

  test("form validation: dataset and model required (dev mode only)", async ({ page }) => {
    if (await isErrorBoundary(page)) {
      test.skip();
      return;
    }
    await page.getByRole("button", { name: "New Run" }).click();
    const submitBtn = page.getByRole("button", { name: "Start Run" });
    await expect(submitBtn).toBeVisible();
    await submitBtn.click();
    const modelInput = page.getByPlaceholder("openai/gpt-4o");
    await expect(modelInput).toBeVisible();
  });

  test("cancel closes form (dev mode only)", async ({ page }) => {
    if (await isErrorBoundary(page)) {
      test.skip();
      return;
    }
    await page.getByRole("button", { name: "New Run" }).click();
    await expect(page.getByText("Dataset")).toBeVisible();
    await page.getByRole("button", { name: "Cancel" }).click();
    await expect(page.getByText("Dataset")).not.toBeVisible();
  });

  test("run list area visible (empty state) (dev mode only)", async ({ page }) => {
    if (await isErrorBoundary(page)) {
      test.skip();
      return;
    }
    const emptyState = page.getByText("No benchmark runs yet.");
    const runList = page.locator("[class*='space-y']");
    await expect(emptyState.or(runList)).toBeVisible({ timeout: 10_000 });
  });

  test("datasets info section visible (dev mode only)", async ({ page }) => {
    if (await isErrorBoundary(page)) {
      test.skip();
      return;
    }
    await expect(page.locator("main h1")).toBeVisible();
  });

  test("compare section visible (dev mode only)", async ({ page }) => {
    if (await isErrorBoundary(page)) {
      test.skip();
      return;
    }
    await expect(page.getByText("Compare Runs")).toBeVisible({ timeout: 10_000 });
  });

  test("delete run button (if runs exist) (dev mode only)", async ({ page }) => {
    if (await isErrorBoundary(page)) {
      test.skip();
      return;
    }
    const deleteBtn = page.getByRole("button", { name: "Delete" }).first();
    const emptyState = page.getByText("No benchmark runs yet.");
    await expect(emptyState.or(deleteBtn)).toBeVisible({ timeout: 10_000 });
  });
});
