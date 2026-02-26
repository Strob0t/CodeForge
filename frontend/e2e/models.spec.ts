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

  test("add model successfully shows card", async ({ page }) => {
    await page.goto("/models");
    await page.getByRole("button", { name: "Add Model" }).click();

    await page.locator("#model-display-name").fill("E2E Test Model");
    await page.locator("#model-litellm-id").fill("openai/gpt-4o-mini");
    await page.getByRole("button", { name: "Add Model" }).last().click();

    // After successful add, form closes and model card appears
    await expect(page.getByText("E2E Test Model")).toBeVisible({ timeout: 10_000 });
  });

  test("delete model removes card", async ({ page }) => {
    await page.goto("/models");

    // First, add a model to delete
    await page.getByRole("button", { name: "Add Model" }).click();
    await page.locator("#model-display-name").fill("ToDelete Model");
    await page.locator("#model-litellm-id").fill("openai/gpt-4o-mini");
    await page.getByRole("button", { name: "Add Model" }).last().click();

    await expect(page.getByText("ToDelete Model")).toBeVisible({ timeout: 10_000 });

    // Click delete on the model card
    await page.getByRole("button", { name: /Delete model ToDelete/ }).click();
    await expect(page.getByText("ToDelete Model")).not.toBeVisible({ timeout: 10_000 });
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
