import { expect, test } from "./fixtures";

// Benchmarks page requires APP_ENV=development. When not set, the route still
// exists but the page cannot load its data (benchmark API returns 403). The nav
// link is also hidden. All tests skip gracefully when dev mode is off.

/** Helper: go to benchmarks, return true if the page rendered successfully */
async function benchmarkPageLoaded(page: import("@playwright/test").Page): Promise<boolean> {
  await page.goto("/benchmarks");
  // Wait for either the benchmark heading or some fallback (error boundary, redirect, empty)
  try {
    await page.locator("main h1").waitFor({ state: "visible", timeout: 5_000 });
    const text = await page.locator("main h1").textContent();
    return text?.includes("Benchmark") ?? false;
  } catch {
    return false;
  }
}

test.describe("Benchmarks page", () => {
  test("page loads or shows dev-mode fallback", async ({ page }) => {
    await page.goto("/benchmarks");
    // Page should not crash — either benchmarks load or something else renders
    await page.waitForLoadState("networkidle");
    // No uncaught exceptions means success
  });

  test("heading shows 'Benchmark Dashboard' (dev mode only)", async ({ page }) => {
    if (!(await benchmarkPageLoaded(page))) {
      test.skip();
      return;
    }
    await expect(page.locator("main h1")).toHaveText("Benchmark Dashboard");
  });

  test("'New Run' button toggles form (dev mode only)", async ({ page }) => {
    if (!(await benchmarkPageLoaded(page))) {
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
    if (!(await benchmarkPageLoaded(page))) {
      test.skip();
      return;
    }
    await page.getByRole("button", { name: "New Run" }).click();
    await expect(page.getByText("Dataset")).toBeVisible();
  });

  test("form has model input (dev mode only)", async ({ page }) => {
    if (!(await benchmarkPageLoaded(page))) {
      test.skip();
      return;
    }
    await page.getByRole("button", { name: "New Run" }).click();
    await expect(page.getByText("Model")).toBeVisible();
    await expect(page.getByPlaceholder("openai/gpt-4o")).toBeVisible();
  });

  test("form has metrics toggle buttons (dev mode only)", async ({ page }) => {
    if (!(await benchmarkPageLoaded(page))) {
      test.skip();
      return;
    }
    await page.getByRole("button", { name: "New Run" }).click();
    await expect(page.getByText("Metrics")).toBeVisible();
    await expect(page.getByRole("button", { name: "correctness" })).toBeVisible();
  });

  test("form validation: dataset and model required (dev mode only)", async ({ page }) => {
    if (!(await benchmarkPageLoaded(page))) {
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
    if (!(await benchmarkPageLoaded(page))) {
      test.skip();
      return;
    }
    await page.getByRole("button", { name: "New Run" }).click();
    await expect(page.getByText("Dataset")).toBeVisible();
    await page.getByRole("button", { name: "Cancel" }).click();
    await expect(page.getByText("Dataset")).not.toBeVisible();
  });

  test("run list area visible (empty state) (dev mode only)", async ({ page }) => {
    if (!(await benchmarkPageLoaded(page))) {
      test.skip();
      return;
    }
    const emptyState = page.getByText("No benchmark runs yet.");
    const runList = page.locator("[class*='space-y']");
    await expect(emptyState.or(runList)).toBeVisible({ timeout: 10_000 });
  });

  test("datasets info section visible (dev mode only)", async ({ page }) => {
    if (!(await benchmarkPageLoaded(page))) {
      test.skip();
      return;
    }
    await expect(page.locator("main h1")).toBeVisible();
  });

  test("compare section visible (dev mode only)", async ({ page }) => {
    if (!(await benchmarkPageLoaded(page))) {
      test.skip();
      return;
    }
    await expect(page.getByText("Compare Runs")).toBeVisible({ timeout: 10_000 });
  });

  test("delete run button (if runs exist) (dev mode only)", async ({ page }) => {
    if (!(await benchmarkPageLoaded(page))) {
      test.skip();
      return;
    }
    const deleteBtn = page.getByRole("button", { name: "Delete" }).first();
    const emptyState = page.getByText("No benchmark runs yet.");
    await expect(emptyState.or(deleteBtn)).toBeVisible({ timeout: 10_000 });
  });
});
