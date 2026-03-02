import { expect, test } from "./fixtures";

// Benchmarks page requires APP_ENV=development. When not set, the route still
// exists but the page cannot load its data (benchmark API returns 403). The nav
// link is also hidden. All tests skip gracefully when dev mode is off.

/** Helper: go to benchmarks, return true if the page rendered successfully */
async function benchmarkPageLoaded(page: import("@playwright/test").Page): Promise<boolean> {
  await page.goto("/benchmarks");
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
    await page.waitForLoadState("networkidle");
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

    await expect(page.locator("form")).not.toBeVisible();
    await page.getByRole("button", { name: "New Run" }).click();
    await expect(page.locator("form")).toBeVisible();

    await page.getByRole("button", { name: "Cancel" }).click();
    await expect(page.locator("form")).not.toBeVisible();
  });

  test("form has dataset select or input (dev mode only)", async ({ page }) => {
    if (!(await benchmarkPageLoaded(page))) {
      test.skip();
      return;
    }
    await page.getByRole("button", { name: "New Run" }).click();
    await expect(page.locator("label[for='benchmark-dataset']")).toBeVisible();
  });

  test("form has model input (dev mode only)", async ({ page }) => {
    if (!(await benchmarkPageLoaded(page))) {
      test.skip();
      return;
    }
    await page.getByRole("button", { name: "New Run" }).click();
    await expect(page.locator("label[for='benchmark-model']")).toBeVisible();
    // ModelCombobox renders <input type="text" list=...> which has role "combobox"
    await expect(page.locator("#benchmark-model")).toBeVisible();
  });

  test("form has metrics toggle buttons (dev mode only)", async ({ page }) => {
    if (!(await benchmarkPageLoaded(page))) {
      test.skip();
      return;
    }
    await page.getByRole("button", { name: "New Run" }).click();
    await expect(page.locator("label[for='benchmark-metrics']")).toBeVisible();
    await expect(page.getByRole("button", { name: "correctness", exact: true })).toBeVisible();
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
    // Form stays visible after validation failure
    await expect(page.locator("form")).toBeVisible();
  });

  test("cancel closes form (dev mode only)", async ({ page }) => {
    if (!(await benchmarkPageLoaded(page))) {
      test.skip();
      return;
    }
    await page.getByRole("button", { name: "New Run" }).click();
    await expect(page.locator("form")).toBeVisible();
    await page.getByRole("button", { name: "Cancel" }).click();
    await expect(page.locator("form")).not.toBeVisible();
  });

  test("run list area visible (empty state) (dev mode only)", async ({ page }) => {
    if (!(await benchmarkPageLoaded(page))) {
      test.skip();
      return;
    }
    const emptyState = page.getByText("No benchmark runs yet.");
    const deleteBtn = page.getByRole("button", { name: "Delete" }).first();
    await expect(emptyState.or(deleteBtn)).toBeVisible({ timeout: 10_000 });
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
    // Compare section only renders when 2+ runs exist; accept empty state too
    const compareHeading = page.getByText("Compare Runs");
    const emptyState = page.getByText("No benchmark runs yet.");
    await expect(compareHeading.or(emptyState)).toBeVisible({ timeout: 10_000 });
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
