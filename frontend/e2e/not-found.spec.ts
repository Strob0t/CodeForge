import { expect, test } from "@playwright/test";

test.describe("Not Found Page", () => {
  test("nonexistent route shows Page not found heading", async ({ page }) => {
    await page.goto("/nonexistent");
    await expect(page.locator("h2")).toContainText("Page not found", {
      timeout: 10_000,
    });
  });

  test("Back to Dashboard button navigates to root", async ({ page }) => {
    await page.goto("/nonexistent");
    await expect(page.locator("h2")).toContainText("Page not found", {
      timeout: 10_000,
    });
    await page.getByRole("button", { name: "Back to Dashboard" }).click();
    await expect(page).toHaveURL(/\/$/, { timeout: 10_000 });
  });

  test("deep nested route shows 404 page", async ({ page }) => {
    await page.goto("/a/b/c/d");
    await expect(page.locator("h2")).toContainText("Page not found", {
      timeout: 10_000,
    });
  });
});
