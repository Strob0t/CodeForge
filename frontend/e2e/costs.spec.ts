import { expect, test } from "./fixtures";

test.describe("Cost Dashboard", () => {
  test("page heading is visible", async ({ page }) => {
    await page.goto("/costs");
    await expect(page.locator("main h1")).toHaveText("Cost Dashboard");
  });

  test("summary cards show Total Cost, Tokens In, Tokens Out, Total Runs", async ({ page }) => {
    await page.goto("/costs");
    await expect(page.getByText("Total Cost")).toBeVisible();
    // Use first() to avoid strict mode violation (summary card + table header both match)
    await expect(page.getByText("Tokens In").first()).toBeVisible();
    await expect(page.getByText("Tokens Out").first()).toBeVisible();
    await expect(page.getByText("Total Runs").first()).toBeVisible();
  });

  test("empty state or table body visible", async ({ page }) => {
    await page.goto("/costs");
    // Either no cost data message or the table body with project rows
    const emptyState = page.getByText("No cost data yet.");
    const tableBody = page.locator("table tbody");
    await expect(emptyState.or(tableBody)).toBeVisible({ timeout: 5_000 });
  });

  test("project table columns visible when data exists", async ({ page, api }) => {
    await api.createProject("Cost Test Project");
    await page.goto("/costs");
    await expect(page.getByText("Cost by Project")).toBeVisible();
    // Use role-based selector to target specific column header
    await expect(page.getByRole("columnheader", { name: "Project" })).toBeVisible();
  });

  test("project link in table navigates to project detail", async ({ page, api }) => {
    const project = await api.createProject("CostNav Project");
    await page.goto("/costs");
    const link = page.getByRole("link", { name: "CostNav Project" });
    const hasLink = await link.isVisible().catch(() => false);
    if (hasLink) {
      await link.click();
      await expect(page).toHaveURL(new RegExp(`/projects/${project.id}`));
    } else {
      await expect(page.getByText("No cost data yet.")).toBeVisible();
    }
  });

  test("project detail page loads with settings gear", async ({ page, api }) => {
    const project = await api.createProject("Cost Detail Test");
    await page.goto(`/projects/${project.id}`);
    // The project detail page has a settings gear icon that opens a popover with cost info
    await expect(page.getByRole("button", { name: /Settings/i })).toBeVisible({ timeout: 10_000 });
  });

  test("cost dashboard table has expected column headers", async ({ page }) => {
    await page.goto("/costs");
    await expect(page.getByRole("columnheader", { name: "Cost" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Tokens In" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Tokens Out" })).toBeVisible();
    await expect(page.getByRole("columnheader", { name: "Runs" })).toBeVisible();
  });

  test("cost values display numeric format", async ({ page }) => {
    await page.goto("/costs");
    // Total cost displays with dollar sign
    await expect(page.getByText(/\$[\d.]+/)).toBeVisible();
  });
});
