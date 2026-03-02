import { expect, test } from "@playwright/test";

test.describe("War Room tab", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/");
    await page.getByRole("link", { name: /new project/i }).click();
    await page.getByLabel("Name").fill("wr-test-" + Date.now());
    await page.getByLabel("Repository URL").fill("https://github.com/test/repo");
    await page.getByRole("button", { name: /create/i }).click();
    await page.waitForURL(/\/projects\//);
  });

  test("War Room tab renders empty state", async ({ page }) => {
    const warRoomTab = page.getByRole("button", { name: "War Room" });
    await expect(warRoomTab).toBeVisible();
    await warRoomTab.click();
    await expect(page.getByText("No agents currently active")).toBeVisible();
    await expect(page.getByText("Start an agent to see live activity here")).toBeVisible();
  });

  test("War Room tab toggles active styling", async ({ page }) => {
    const warRoomTab = page.getByRole("button", { name: "War Room" });
    await warRoomTab.click();
    await expect(warRoomTab).toHaveClass(/bg-cf-accent/);
  });
});
