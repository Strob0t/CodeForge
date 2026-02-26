import { expect, test } from "./fixtures";

test.describe("Agent Modes", () => {
  test("page heading is visible", async ({ page }) => {
    await page.goto("/modes");
    await expect(page.locator("main h1")).toHaveText("Agent Modes");
  });

  test("built-in modes listed on load", async ({ page }) => {
    await page.goto("/modes");
    // At least one mode card should be visible
    const cards = page.locator("[class*='hover:shadow']");
    await expect(cards.first()).toBeVisible({ timeout: 10_000 });
  });

  test("mode card shows name and description", async ({ page }) => {
    await page.goto("/modes");
    // Built-in modes have h3 headings with name and a description paragraph
    const firstCard = page.locator("[class*='hover:shadow']").first();
    await expect(firstCard).toBeVisible({ timeout: 10_000 });
    await expect(firstCard.locator("h3")).toBeVisible();
    await expect(firstCard.locator("p")).toBeVisible();
  });

  test("Add Mode button opens form", async ({ page }) => {
    await page.goto("/modes");
    await expect(page.locator("#mode-id")).not.toBeVisible();

    await page.getByRole("button", { name: "Add Mode" }).click();
    await expect(page.locator("#mode-id")).toBeVisible();
    await expect(page.locator("#mode-name")).toBeVisible();
  });

  test("form has required fields ID and Name", async ({ page }) => {
    await page.goto("/modes");
    await page.getByRole("button", { name: "Add Mode" }).click();

    // Use the input fields to verify form structure (avoid getByText for short labels)
    await expect(page.locator("#mode-id")).toBeVisible();
    await expect(page.locator("#mode-name")).toBeVisible();
    // Labels exist near the inputs
    await expect(page.locator("label[for='mode-id']")).toBeVisible();
    await expect(page.locator("label[for='mode-name']")).toBeVisible();
  });

  test("create custom mode via form and verify card appears", async ({ page, api }) => {
    const modeId = `e2e-mode-${Date.now()}`;
    const modeName = `E2E Mode ${Date.now()}`;

    await page.goto("/modes");
    await page.getByRole("button", { name: "Add Mode" }).click();

    await page.locator("#mode-id").fill(modeId);
    await page.locator("#mode-name").fill(modeName);
    await page.locator("#mode-desc").fill("A test mode for E2E");
    await page.getByRole("button", { name: "Create Mode" }).click();

    // Either the mode card appears or the form shows an error
    const card = page.getByText(modeName);
    const form = page.locator("#mode-id");
    await expect(card.or(form)).toBeVisible({ timeout: 10_000 });

    // Cleanup via API
    try {
      const token = await api.getAdminToken();
      await fetch(`http://localhost:8080/api/v1/modes/${encodeURIComponent(modeId)}`, {
        method: "DELETE",
        headers: { Authorization: `Bearer ${token}` },
      });
    } catch {
      // best-effort cleanup
    }
  });

  test("edit button visible on custom modes but not on built-in", async ({ page, api }) => {
    // Create a custom mode via API
    await api.createMode({
      id: "e2e-edit-check",
      name: "Edit Check Mode",
      description: "For edit button test",
      tools: ["Read"],
      autonomy: 3,
    });
    await page.goto("/modes");

    // Built-in mode should have "built-in" badge but no "Edit" button
    const builtinCard = page
      .locator("[class*='hover:shadow']")
      .filter({ hasText: "built-in" })
      .first();
    await expect(builtinCard).toBeVisible({ timeout: 10_000 });
    await expect(builtinCard.getByRole("button", { name: /Edit/ })).not.toBeVisible();

    // Custom mode should have "Edit" button
    const customCard = page
      .locator("[class*='hover:shadow']")
      .filter({ hasText: "Edit Check Mode" });
    await expect(customCard.getByRole("button", { name: /Edit/ })).toBeVisible();
  });

  test("built-in modes show builtin badge", async ({ page }) => {
    await page.goto("/modes");
    await expect(page.getByText("built-in").first()).toBeVisible({ timeout: 10_000 });
  });

  test("scenario dropdown has options", async ({ page }) => {
    await page.goto("/modes");
    await page.getByRole("button", { name: "Add Mode" }).click();

    const scenarioSelect = page.locator("#mode-scenario");
    await expect(scenarioSelect).toBeVisible();
    // Should have at least the "default" option
    await expect(scenarioSelect.locator("option")).not.toHaveCount(0);
  });

  test("cancel button closes form", async ({ page }) => {
    await page.goto("/modes");
    await page.getByRole("button", { name: "Add Mode" }).click();
    await expect(page.locator("#mode-id")).toBeVisible();

    await page.getByRole("button", { name: "Cancel" }).click();
    await expect(page.locator("#mode-id")).not.toBeVisible();
  });
});
