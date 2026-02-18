import { expect, test } from "@playwright/test";

test.describe("Cost Dashboard", () => {
  test("heading is visible", async ({ page }) => {
    await page.goto("/costs");
    await expect(page.locator("h2")).toHaveText("Cost Dashboard");
  });

  test("shows empty state message", async ({ page }) => {
    await page.goto("/costs");
    await expect(page.getByText("No cost data yet.")).toBeVisible();
  });
});
