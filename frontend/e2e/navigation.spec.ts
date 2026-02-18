import { expect, test } from "@playwright/test";

test.describe("Sidebar navigation", () => {
  test("root shows Projects heading", async ({ page }) => {
    await page.goto("/");
    await expect(page.locator("h2")).toHaveText("Projects");
  });

  test("navigate to Costs page", async ({ page }) => {
    await page.goto("/");
    await page.getByRole("link", { name: "Costs" }).click();
    await expect(page).toHaveURL(/\/costs$/);
    await expect(page.locator("h2")).toHaveText("Cost Dashboard");
  });

  test("navigate to Models page", async ({ page }) => {
    await page.goto("/");
    await page.getByRole("link", { name: "Models" }).click();
    await expect(page).toHaveURL(/\/models$/);
    await expect(page.locator("h2")).toContainText("LLM Models");
  });

  test("navigate back to Dashboard", async ({ page }) => {
    await page.goto("/models");
    await page.getByRole("link", { name: "Dashboard" }).click();
    await expect(page).toHaveURL(/\/$/);
    await expect(page.locator("h2")).toHaveText("Projects");
  });
});
