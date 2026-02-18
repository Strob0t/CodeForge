import { expect, test } from "@playwright/test";

test.describe("Models page", () => {
  test("heading is visible", async ({ page }) => {
    await page.goto("/models");
    await expect(page.locator("h2")).toContainText("LLM Models");
  });

  test("Add Model button toggles form", async ({ page }) => {
    await page.goto("/models");
    await expect(page.getByText("Display Name *")).not.toBeVisible();

    await page.getByRole("button", { name: "Add Model" }).click();
    await expect(page.getByText("Display Name *")).toBeVisible();

    await page.getByRole("button", { name: "Cancel" }).click();
    await expect(page.getByText("Display Name *")).not.toBeVisible();
  });

  test("LiteLLM status text is shown", async ({ page }) => {
    await page.goto("/models");
    await expect(page.getByText(/LiteLLM:/)).toBeVisible({ timeout: 10_000 });
  });
});
