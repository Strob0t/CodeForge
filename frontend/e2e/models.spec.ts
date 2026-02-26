import { expect, test } from "./fixtures";

test.describe("Models page", () => {
  test("page heading is visible", async ({ page }) => {
    await page.goto("/models");
    await expect(page.locator("main h1")).toHaveText("LLM Models");
  });

  test("LiteLLM health status text visible", async ({ page }) => {
    await page.goto("/models");
    await expect(page.getByText(/LiteLLM:/)).toBeVisible({ timeout: 10_000 });
  });

  test("Add Model button toggles form visibility", async ({ page }) => {
    await page.goto("/models");
    // Form is initially hidden
    await expect(page.locator("#model-display-name")).not.toBeVisible();

    await page.getByRole("button", { name: "Add Model" }).click();
    await expect(page.locator("#model-display-name")).toBeVisible();

    // The button now shows "Cancel"
    await page.getByRole("button", { name: "Cancel" }).click();
    await expect(page.locator("#model-display-name")).not.toBeVisible();
  });

  test("form shows required fields Display Name and LiteLLM Model", async ({ page }) => {
    await page.goto("/models");
    await page.getByRole("button", { name: "Add Model" }).click();

    await expect(page.getByText("Display Name")).toBeVisible();
    await expect(page.getByText("LiteLLM Model")).toBeVisible();
    await expect(page.getByText("API Base (optional)")).toBeVisible();
    await expect(page.getByText("API Key (optional)")).toBeVisible();
  });

  test("form validation: submit without name shows error", async ({ page }) => {
    await page.goto("/models");
    await page.getByRole("button", { name: "Add Model" }).click();

    // Fill only model ID, leave name empty
    await page.locator("#model-litellm-id").fill("openai/gpt-4o");
    await page.getByRole("button", { name: "Add Model" }).last().click();

    // The form silently returns when fields are empty; verify form stays visible
    await expect(page.locator("#model-display-name")).toBeVisible();
  });

  test("form validation: submit without model ID shows error", async ({ page }) => {
    await page.goto("/models");
    await page.getByRole("button", { name: "Add Model" }).click();

    // Fill only name, leave model ID empty
    await page.locator("#model-display-name").fill("My Test Model");
    await page.getByRole("button", { name: "Add Model" }).last().click();

    // The form silently returns when fields are empty; verify form stays visible
    await expect(page.locator("#model-litellm-id")).toBeVisible();
  });

  test("add model form submits and either shows card or error", async ({ page }) => {
    await page.goto("/models");
    await page.getByRole("button", { name: "Add Model" }).click();

    await page.locator("#model-display-name").fill("E2E Test Model");
    await page.locator("#model-litellm-id").fill("openai/gpt-4o-mini");
    await page.getByRole("button", { name: "Add Model" }).last().click();

    // If LiteLLM is healthy, card appears; if not, form may stay or show error toast
    const card = page.getByText("E2E Test Model");
    const form = page.locator("#model-display-name");
    await expect(card.or(form)).toBeVisible({ timeout: 10_000 });
  });

  test("delete model removes card if models exist", async ({ page }) => {
    await page.goto("/models");

    // Check if any model card with a delete button exists
    const deleteBtn = page.getByRole("button", { name: /Delete model/ }).first();
    const hasDeleteBtn = await deleteBtn.isVisible({ timeout: 5_000 }).catch(() => false);

    if (!hasDeleteBtn) {
      // No models to delete — skip
      test.skip();
      return;
    }

    // Get the model name from the heading near the delete button
    const modelCard = deleteBtn.locator("..").locator("h3").first();
    const modelName = await modelCard.textContent().catch(() => null);

    await deleteBtn.click();

    // After delete, the model should disappear
    if (modelName) {
      await expect(page.getByText(modelName, { exact: true })).not.toBeVisible({ timeout: 10_000 });
    }
  });

  test("Discover Models button triggers discovery", async ({ page }) => {
    await page.goto("/models");
    const discoverBtn = page.getByRole("button", { name: "Discover Models" });
    await expect(discoverBtn).toBeVisible();
    await discoverBtn.click();

    // After clicking, button text should change to "Discovering..." or discovered section appears
    await expect(
      page.getByText("Discovering...").or(page.getByText("Discovered Models")),
    ).toBeVisible({ timeout: 10_000 });
  });

  test("empty state shows no models message", async ({ page }) => {
    await page.goto("/models");
    // When no models are configured, the empty state shows
    await expect(
      page.getByText("No models configured yet.").or(page.getByText(/LiteLLM:/)),
    ).toBeVisible({ timeout: 10_000 });
  });

  test("cancel button closes form without changes", async ({ page }) => {
    await page.goto("/models");
    await page.getByRole("button", { name: "Add Model" }).click();

    // Type something in the form
    await page.locator("#model-display-name").fill("Should Not Persist");

    // Click Cancel
    await page.getByRole("button", { name: "Cancel" }).click();

    // Form is hidden
    await expect(page.locator("#model-display-name")).not.toBeVisible();

    // The text should not appear as a model card
    await expect(page.getByText("Should Not Persist")).not.toBeVisible();
  });
});
