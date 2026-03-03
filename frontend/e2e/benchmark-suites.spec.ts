import { expect, test, ensureBenchmarkPage, clickTab } from "./benchmark-fixtures";

test.describe("Benchmark Suites - CRUD", () => {
  test.beforeEach(async ({ page }) => {
    const loaded = await ensureBenchmarkPage(page);
    if (!loaded) test.skip();
  });

  test("Suites tab shows Create Suite button", async ({ page }) => {
    await clickTab(page, "Suites");
    await expect(page.getByRole("button", { name: "Create Suite" })).toBeVisible();
  });

  test("Create Suite button toggles form", async ({ page }) => {
    await clickTab(page, "Suites");
    await page.getByRole("button", { name: "Create Suite" }).click();
    await expect(page.locator("form")).toBeVisible();
    await page.getByRole("button", { name: "Cancel" }).click();
    await expect(page.locator("form")).not.toBeVisible();
  });

  test("suite form has all required fields", async ({ page }) => {
    await clickTab(page, "Suites");
    await page.getByRole("button", { name: "Create Suite" }).click();
    await expect(page.locator("#suite-name")).toBeVisible();
    await expect(page.locator("#suite-desc")).toBeVisible();
    await expect(page.locator("#suite-type")).toBeVisible();
    await expect(page.locator("#suite-provider")).toBeVisible();
  });

  test("create suite via API", async ({ benchApi }) => {
    const suite = await benchApi.createSuite({
      name: "e2e-test-suite",
      description: "E2E test suite",
      type: "deepeval",
      provider_name: "deepeval",
    });
    expect(suite.id).toBeTruthy();
    expect(suite.name).toBe("e2e-test-suite");
  });

  test("list suites via API", async ({ benchApi }) => {
    await benchApi.createSuite({
      name: "e2e-list-suite",
      type: "deepeval",
      provider_name: "deepeval",
    });
    const suites = await benchApi.listSuites();
    expect(suites.length).toBeGreaterThanOrEqual(1);
    expect(suites.find((s) => s.name === "e2e-list-suite")).toBeDefined();
  });

  test("suite appears in Suites tab UI", async ({ page, benchApi }) => {
    await benchApi.createSuite({
      name: "e2e-ui-suite",
      type: "deepeval",
      provider_name: "deepeval",
    });
    await clickTab(page, "Suites");
    await page.reload();
    await page.locator("main h1").waitFor({ state: "visible", timeout: 5_000 });
    await clickTab(page, "Suites");
    await expect(page.getByText("e2e-ui-suite").first()).toBeVisible({ timeout: 10_000 });
  });

  test("update suite via API", async ({ benchApi }) => {
    const suite = await benchApi.createSuite({
      name: "e2e-update-suite",
      type: "deepeval",
      provider_name: "deepeval",
    });
    const updated = await benchApi.updateSuite(suite.id, {
      name: "e2e-updated-suite",
      description: "Updated description",
    });
    expect(updated.name).toBe("e2e-updated-suite");
    expect(updated.description).toBe("Updated description");
  });

  test("edit button visible on suite card", async ({ page, benchApi }) => {
    await benchApi.createSuite({
      name: "e2e-edit-btn-suite",
      type: "deepeval",
      provider_name: "deepeval",
    });
    await clickTab(page, "Suites");
    await page.reload();
    await page.locator("main h1").waitFor({ state: "visible", timeout: 5_000 });
    await clickTab(page, "Suites");
    await expect(page.getByText("e2e-edit-btn-suite").first()).toBeVisible({ timeout: 10_000 });
    await expect(page.getByRole("button", { name: "Edit" }).first()).toBeVisible();
  });

  test("delete suite via API", async ({ benchApi }) => {
    const suite = await benchApi.createSuite({
      name: "e2e-delete-suite",
      type: "deepeval",
      provider_name: "deepeval",
    });
    await benchApi.deleteSuite(suite.id);
    const suites = await benchApi.listSuites();
    expect(suites.find((s) => s.id === suite.id)).toBeUndefined();
  });

  test("delete button visible on suite card", async ({ page, benchApi }) => {
    await benchApi.createSuite({
      name: "e2e-delete-btn-suite",
      type: "deepeval",
      provider_name: "deepeval",
    });
    await clickTab(page, "Suites");
    await page.reload();
    await page.locator("main h1").waitFor({ state: "visible", timeout: 5_000 });
    await clickTab(page, "Suites");
    await expect(page.getByText("e2e-delete-btn-suite").first()).toBeVisible({ timeout: 10_000 });
    await expect(page.getByRole("button", { name: "Delete" }).first()).toBeVisible();
  });

  test("suite card shows type and provider badges", async ({ page, benchApi }) => {
    await benchApi.createSuite({
      name: "e2e-badge-suite",
      type: "custom_type",
      provider_name: "custom_provider",
    });
    await clickTab(page, "Suites");
    await page.reload();
    await page.locator("main h1").waitFor({ state: "visible", timeout: 5_000 });
    await clickTab(page, "Suites");
    await expect(page.getByText("custom_type").first()).toBeVisible({ timeout: 10_000 });
    await expect(page.getByText("custom_provider").first()).toBeVisible();
  });
});
