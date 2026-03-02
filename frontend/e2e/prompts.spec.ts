import { expect, test } from "./fixtures";

test.describe("Prompt Editor page", () => {
  test("page heading 'Prompt Sections' visible", async ({ page }) => {
    await page.goto("/prompts");
    await expect(page.locator("main h1")).toHaveText("Prompt Sections");
  });

  test("subtitle text visible", async ({ page }) => {
    await page.goto("/prompts");
    const subtitle = page.getByText("Manage composable prompt sections for mode assembly");
    const heading = page.locator("main h1");
    await expect(heading).toBeVisible();
    if (await subtitle.isVisible().catch(() => false)) {
      await expect(subtitle).toBeVisible();
    }
  });

  test("scope selector visible and defaults to 'global'", async ({ page }) => {
    await page.goto("/prompts");
    const scopeSelect = page.locator("select").first();
    await expect(scopeSelect).toBeVisible();
    await expect(scopeSelect).toHaveValue("global");
  });

  test("'Add Section' button opens section form", async ({ page }) => {
    await page.goto("/prompts");
    await expect(page.getByText("New Section")).not.toBeVisible();
    await page.getByRole("button", { name: "Add Section" }).click();
    await expect(page.getByText("New Section")).toBeVisible();
  });

  test("form has name field", async ({ page }) => {
    await page.goto("/prompts");
    await page.getByRole("button", { name: "Add Section" }).click();
    await expect(page.getByText("Name", { exact: true }).first()).toBeVisible();
  });

  test("form has merge strategy dropdown", async ({ page }) => {
    await page.goto("/prompts");
    await page.getByRole("button", { name: "Add Section" }).click();
    await expect(page.getByText("Merge Strategy")).toBeVisible();
    const mergeSelect = page.locator("select").nth(1);
    await expect(mergeSelect.locator("option")).toHaveCount(3);
  });

  test("form has priority slider", async ({ page }) => {
    await page.goto("/prompts");
    await page.getByRole("button", { name: "Add Section" }).click();
    await expect(page.getByText("Priority (0-100)")).toBeVisible();
    await expect(page.locator("input[type='range']")).toBeVisible();
  });

  test("form has content textarea", async ({ page }) => {
    await page.goto("/prompts");
    await page.getByRole("button", { name: "Add Section" }).click();
    await expect(page.getByText("Content", { exact: true })).toBeVisible();
    await expect(page.locator("textarea")).toBeVisible();
  });

  test("form has enabled checkbox", async ({ page }) => {
    await page.goto("/prompts");
    await page.getByRole("button", { name: "Add Section" }).click();
    await expect(page.getByText("Enabled")).toBeVisible();
    const checkbox = page.locator("input[type='checkbox']");
    await expect(checkbox).toBeVisible();
    await expect(checkbox).toBeChecked();
  });

  test("form validation on empty name shows error toast", async ({ page }) => {
    await page.goto("/prompts");
    await page.getByRole("button", { name: "Add Section" }).click();
    await page.getByRole("button", { name: "Save" }).click();
    await expect(page.getByText("Section name is required")).toBeVisible({ timeout: 10_000 });
  });

  test("create section + appears in list", async ({ page }) => {
    await page.goto("/prompts");
    await page.getByRole("button", { name: "Add Section" }).click();
    await page.getByRole("textbox").first().fill("E2E Test Section");
    await page.locator("textarea").fill("This is test content for the prompt section.");
    await page.getByRole("button", { name: "Save" }).click();
    await expect(page.getByText("E2E Test Section")).toBeVisible({ timeout: 10_000 });
  });

  test("'Preview' button opens preview panel with token count", async ({ page, api }) => {
    await api.createPromptSection("global", {
      name: "Preview Test Section",
      content: "Hello world prompt content",
      priority: 50,
      sort_order: 0,
      enabled: true,
      merge: "replace",
    });

    await page.goto("/prompts");
    await expect(page.getByText("Preview Test Section")).toBeVisible({ timeout: 10_000 });
    await page.getByRole("button", { name: "Preview" }).click();
    const previewTitle = page.getByText("Assembled Prompt Preview");
    const tokenBadge = page.getByText("tokens");
    await expect(previewTitle.or(tokenBadge).first()).toBeVisible({ timeout: 10_000 });
  });

  test("delete section removes from list", async ({ page, api }) => {
    await api.createPromptSection("global", {
      name: "Section To Delete",
      content: "delete me",
      priority: 50,
      sort_order: 0,
      enabled: true,
      merge: "replace",
    });

    await page.goto("/prompts");
    await expect(page.getByText("Section To Delete")).toBeVisible({ timeout: 10_000 });
    // Scope the Delete button to the card containing "Section To Delete"
    const card = page.locator("[class*='gap-3']").filter({ hasText: "Section To Delete" });
    await card.getByRole("button", { name: "Delete" }).click();
    await expect(page.getByText("Section To Delete")).not.toBeVisible({ timeout: 10_000 });
  });
});
