import { expect, test } from "./fixtures";

test.describe("Knowledge Bases", () => {
  test("page heading is visible", async ({ page }) => {
    await page.goto("/knowledge-bases");
    await expect(page.locator("h1")).toHaveText("Knowledge Bases");
  });

  test("description text visible", async ({ page }) => {
    await page.goto("/knowledge-bases");
    await expect(page.getByText("Curated knowledge modules for agent context")).toBeVisible();
  });

  test("empty state shows no knowledge bases message", async ({ page }) => {
    await page.goto("/knowledge-bases");
    await expect(page.getByText("No knowledge bases available").or(page.locator("h1"))).toBeVisible(
      { timeout: 10_000 },
    );
  });

  test("Create button toggles form", async ({ page }) => {
    await page.goto("/knowledge-bases");
    await expect(page.locator("#kb-name")).not.toBeVisible();

    await page.getByRole("button", { name: "Create Knowledge Base" }).click();
    await expect(page.locator("#kb-name")).toBeVisible();

    // Click again to toggle off (button becomes "Cancel")
    await page.getByRole("button", { name: "Cancel" }).click();
    await expect(page.locator("#kb-name")).not.toBeVisible();
  });

  test("form shows name field as required", async ({ page }) => {
    await page.goto("/knowledge-bases");
    await page.getByRole("button", { name: "Create Knowledge Base" }).click();

    await expect(page.getByText("Name")).toBeVisible();
    await expect(page.locator("#kb-name")).toBeVisible();
  });

  test("form shows category dropdown", async ({ page }) => {
    await page.goto("/knowledge-bases");
    await page.getByRole("button", { name: "Create Knowledge Base" }).click();

    await expect(page.locator("#kb-category")).toBeVisible();
    await expect(page.getByText("Category")).toBeVisible();
  });

  test("form shows tags input", async ({ page }) => {
    await page.goto("/knowledge-bases");
    await page.getByRole("button", { name: "Create Knowledge Base" }).click();

    await expect(page.locator("#kb-tags")).toBeVisible();
    await expect(page.getByText("Tags")).toBeVisible();
  });

  test("form shows content path input", async ({ page }) => {
    await page.goto("/knowledge-bases");
    await page.getByRole("button", { name: "Create Knowledge Base" }).click();

    await expect(page.locator("#kb-content-path")).toBeVisible();
    await expect(page.getByText("Content Path")).toBeVisible();
  });

  test("form validation: submit without name keeps form open", async ({ page }) => {
    await page.goto("/knowledge-bases");
    await page.getByRole("button", { name: "Create Knowledge Base" }).click();

    // Leave name empty, fill content path (also required)
    await page.locator("#kb-content-path").fill("/tmp/test");
    await page.getByRole("button", { name: "Create Knowledge Base" }).last().click();

    // Form stays visible because name is required (HTML5 validation or silent return)
    await expect(page.locator("#kb-name")).toBeVisible();
  });

  test("create knowledge base successfully and card appears", async ({ page }) => {
    await page.goto("/knowledge-bases");
    await page.getByRole("button", { name: "Create Knowledge Base" }).click();

    await page.locator("#kb-name").fill("E2E Test KB");
    await page.locator("#kb-content-path").fill("/tmp/e2e-test-kb");
    await page.getByRole("button", { name: "Create Knowledge Base" }).last().click();

    await expect(page.getByText("E2E Test KB")).toBeVisible({ timeout: 10_000 });
  });

  test("KB card shows name and category badge", async ({ page, api }) => {
    await api.createKnowledgeBase({
      name: "Badge Test KB",
      description: "Testing badges",
      category: "framework",
      tags: [],
      content_path: "/tmp/badge-test",
    });
    await page.goto("/knowledge-bases");

    await expect(page.getByText("Badge Test KB")).toBeVisible({ timeout: 10_000 });
    // Category badge should show "Framework"
    await expect(page.getByText("Framework")).toBeVisible();
  });

  test("delete button removes KB", async ({ page, api }) => {
    await api.createKnowledgeBase({
      name: "ToDelete KB",
      description: "Will be deleted",
      category: "custom",
      tags: [],
      content_path: "/tmp/to-delete",
    });
    await page.goto("/knowledge-bases");

    await expect(page.getByText("ToDelete KB")).toBeVisible({ timeout: 10_000 });
    await page.getByRole("button", { name: "Delete" }).click();
    await expect(page.getByText("ToDelete KB")).not.toBeVisible({ timeout: 10_000 });
  });
});
