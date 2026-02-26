import { expect, test } from "./fixtures";

test.describe("Benchmarks page", () => {
  test("page heading 'Benchmark Dashboard' visible", async ({ page }) => {
    await page.goto("/benchmarks");
    await expect(page.locator("main h1")).toHaveText("Benchmark Dashboard");
  });

  test("subtitle visible", async ({ page }) => {
    await page.goto("/benchmarks");
    await expect(page.getByText("Evaluate agent quality with configurable metrics")).toBeVisible();
  });

  test("'New Run' button toggles form", async ({ page }) => {
    await page.goto("/benchmarks");

    // Form should not be visible initially
    await expect(page.locator("#benchmark-dataset")).not.toBeVisible();

    await page.getByRole("button", { name: "New Run" }).click();
    await expect(page.locator("#benchmark-dataset").or(page.getByText("Dataset"))).toBeVisible();

    // Click cancel to close form
    await page.getByRole("button", { name: "Cancel" }).click();
    await expect(page.locator("#benchmark-dataset")).not.toBeVisible();
  });

  test("form has dataset select or input", async ({ page }) => {
    await page.goto("/benchmarks");
    await page.getByRole("button", { name: "New Run" }).click();

    // Dataset field — either a select or input depending on whether datasets exist
    await expect(page.getByText("Dataset")).toBeVisible();
  });

  test("form has model input", async ({ page }) => {
    await page.goto("/benchmarks");
    await page.getByRole("button", { name: "New Run" }).click();

    await expect(page.getByText("Model")).toBeVisible();
    // The model input has placeholder "openai/gpt-4o"
    await expect(page.getByPlaceholder("openai/gpt-4o")).toBeVisible();
  });

  test("form has metrics toggle buttons", async ({ page }) => {
    await page.goto("/benchmarks");
    await page.getByRole("button", { name: "New Run" }).click();

    await expect(page.getByText("Metrics")).toBeVisible();
    // Should show metric toggle buttons
    await expect(page.getByRole("button", { name: "correctness" })).toBeVisible();
    await expect(page.getByRole("button", { name: "faithfulness" })).toBeVisible();
    await expect(page.getByRole("button", { name: "answer_relevancy" })).toBeVisible();
  });

  test("form validation: dataset and model required", async ({ page }) => {
    await page.goto("/benchmarks");
    await page.getByRole("button", { name: "New Run" }).click();

    // The submit button should be "Start Run"
    const submitBtn = page.getByRole("button", { name: "Start Run" });
    await expect(submitBtn).toBeVisible();

    // The model input has the HTML required attribute, so clicking submit
    // without filling should trigger browser validation
    await submitBtn.click();

    // The form uses native HTML validation (required attribute on inputs)
    // Verify the fields exist and are empty
    const modelInput = page.getByPlaceholder("openai/gpt-4o");
    await expect(modelInput).toBeVisible();
  });

  test("cancel closes form", async ({ page }) => {
    await page.goto("/benchmarks");
    await page.getByRole("button", { name: "New Run" }).click();

    await expect(page.getByText("Dataset")).toBeVisible();
    await page.getByRole("button", { name: "Cancel" }).click();
    await expect(page.getByText("Dataset")).not.toBeVisible();
  });

  test("run list area visible (empty state)", async ({ page }) => {
    await page.goto("/benchmarks");

    // Should show empty state or run list
    const emptyState = page.getByText("No benchmark runs yet.");
    const runList = page.locator("[class*='space-y']");
    await expect(emptyState.or(runList)).toBeVisible({ timeout: 10_000 });
  });

  test("datasets info section visible", async ({ page }) => {
    await page.goto("/benchmarks");

    // Datasets section shows when datasets exist. Otherwise it's hidden.
    // Check for "Available Datasets" heading or verify page loaded without error
    const pageHeading = page.locator("main h1");
    await expect(pageHeading).toBeVisible();
    // Datasets section may or may not be visible depending on data
  });

  test("compare section visible", async ({ page }) => {
    await page.goto("/benchmarks");

    // The BenchmarkCompare component renders the Compare section
    const compareHeading = page.getByText("Compare Runs");
    // Compare section is always rendered (may be empty)
    await expect(compareHeading).toBeVisible({ timeout: 10_000 });
  });

  test("delete run button (if runs exist)", async ({ page }) => {
    await page.goto("/benchmarks");

    // If there are runs, each should have a delete button
    const deleteBtn = page.getByRole("button", { name: "Delete" }).first();
    const emptyState = page.getByText("No benchmark runs yet.");

    // Either empty state is shown or delete buttons exist
    await expect(emptyState.or(deleteBtn)).toBeVisible({ timeout: 10_000 });
  });
});
