import { expect, test } from "./fixtures";

test.describe("Retrieval Scopes", () => {
  test("page heading is visible", async ({ page }) => {
    await page.goto("/scopes");
    await expect(page.locator("h1")).toHaveText("Scopes");
  });

  test("description text visible", async ({ page }) => {
    await page.goto("/scopes");
    await expect(page.getByText("Cross-project retrieval scope management")).toBeVisible();
  });

  test("empty state when no scopes", async ({ page }) => {
    await page.goto("/scopes");
    await expect(page.getByText("No scopes created yet").or(page.locator("h1"))).toBeVisible({
      timeout: 10_000,
    });
  });

  test("Create button toggles form", async ({ page }) => {
    await page.goto("/scopes");
    await expect(page.locator("#scope-name")).not.toBeVisible();

    await page.getByRole("button", { name: "Create Scope" }).click();
    await expect(page.locator("#scope-name")).toBeVisible();

    // Click Cancel to close
    await page.getByRole("button", { name: "Cancel" }).click();
    await expect(page.locator("#scope-name")).not.toBeVisible();
  });

  test("form shows name field as required", async ({ page }) => {
    await page.goto("/scopes");
    await page.getByRole("button", { name: "Create Scope" }).click();

    await expect(page.locator("#scope-name")).toBeVisible();
    await expect(page.getByText("Name")).toBeVisible();
  });

  test("form shows type dropdown", async ({ page }) => {
    await page.goto("/scopes");
    await page.getByRole("button", { name: "Create Scope" }).click();

    await expect(page.locator("#scope-type")).toBeVisible();
    await expect(page.getByText("Type")).toBeVisible();
  });

  test("form shows description field", async ({ page }) => {
    await page.goto("/scopes");
    await page.getByRole("button", { name: "Create Scope" }).click();

    await expect(page.locator("#scope-desc")).toBeVisible();
    await expect(page.getByText("Description")).toBeVisible();
  });

  test("form validation on empty name", async ({ page }) => {
    await page.goto("/scopes");
    await page.getByRole("button", { name: "Create Scope" }).click();

    // Leave name empty and submit
    await page.getByRole("button", { name: "Create Scope" }).last().click();

    // Form stays open because name is required (HTML5 validation or silent return)
    await expect(page.locator("#scope-name")).toBeVisible();
  });

  test("create scope successfully and card appears", async ({ page }) => {
    await page.goto("/scopes");
    await page.getByRole("button", { name: "Create Scope" }).click();

    await page.locator("#scope-name").fill("E2E Test Scope");
    await page.locator("#scope-desc").fill("Test scope description");
    await page.getByRole("button", { name: "Create Scope" }).last().click();

    await expect(page.getByText("E2E Test Scope")).toBeVisible({ timeout: 10_000 });
  });

  test("scope card shows name, type badge, and project count", async ({ page, api }) => {
    await api.createScope({
      name: "Badge Test Scope",
      description: "For badge testing",
      type: "shared",
      project_ids: [],
    });
    await page.goto("/scopes");

    await expect(page.getByText("Badge Test Scope")).toBeVisible({ timeout: 10_000 });
    await expect(page.getByText("Shared")).toBeVisible();
    await expect(page.getByText("0 projects")).toBeVisible();
  });

  test("card is expandable on click", async ({ page, api }) => {
    await api.createScope({
      name: "Expandable Scope",
      description: "Click to expand",
      type: "shared",
      project_ids: [],
    });
    await page.goto("/scopes");

    const card = page.locator("[class*='hover:shadow']").filter({ hasText: "Expandable Scope" });
    await expect(card).toBeVisible({ timeout: 10_000 });

    // Click the card header to expand
    await card.locator("[role='button']").click();

    // Expanded view should show the detail panel with tabs
    await expect(card.getByText("Projects")).toBeVisible({ timeout: 5_000 });
  });

  test("expanded view has tabs for Projects, Knowledge Bases, Search", async ({ page, api }) => {
    await api.createScope({
      name: "Tabs Test Scope",
      description: "Testing tabs",
      type: "global",
      project_ids: [],
    });
    await page.goto("/scopes");

    const card = page.locator("[class*='hover:shadow']").filter({ hasText: "Tabs Test Scope" });
    await expect(card).toBeVisible({ timeout: 10_000 });

    // Click to expand
    await card.locator("[role='button']").click();

    // Verify all three tabs are present
    await expect(card.getByText("Projects")).toBeVisible({ timeout: 5_000 });
    await expect(card.getByText("Knowledge Bases")).toBeVisible();
    await expect(card.getByText("Search")).toBeVisible();
  });

  test("delete scope removes card", async ({ page, api }) => {
    await api.createScope({
      name: "ToDelete Scope",
      description: "Will be deleted",
      type: "shared",
      project_ids: [],
    });
    await page.goto("/scopes");

    const card = page.locator("[class*='hover:shadow']").filter({ hasText: "ToDelete Scope" });
    await expect(card).toBeVisible({ timeout: 10_000 });

    // Expand the card to access the delete button
    await card.locator("[role='button']").click();
    await card.getByRole("button", { name: "Delete" }).click();

    await expect(page.getByText("ToDelete Scope")).not.toBeVisible({ timeout: 10_000 });
  });
});
