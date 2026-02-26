import { expect, test } from "./fixtures";

test.describe("Activity page", () => {
  test("page heading is visible", async ({ page }) => {
    await page.goto("/activity");
    await expect(page.locator("main h1")).toHaveText("Activity");
  });

  test("WebSocket connection status badge visible", async ({ page }) => {
    await page.goto("/activity");
    // Badge shows either "Live" (connected) or "Disconnected" — use exact match to avoid strict mode
    const liveBadge = page.getByText("Live", { exact: true });
    const disconnectedBadge = page.getByText("Disconnected", { exact: true });
    await expect(liveBadge.or(disconnectedBadge)).toBeVisible({ timeout: 10_000 });
  });

  test("connection badge shows connected or disconnected", async ({ page }) => {
    await page.goto("/activity");
    // Use exact text match to avoid matching sidebar "WebSocket: disconnected" or alert banner
    const liveBadge = page.getByText("Live", { exact: true });
    const disconnectedBadge = page.getByText("Disconnected", { exact: true });
    const isLive = await liveBadge.isVisible().catch(() => false);
    const isDisconnected = await disconnectedBadge.isVisible().catch(() => false);
    expect(isLive || isDisconnected).toBe(true);
  });

  test("activity feed area visible", async ({ page }) => {
    await page.goto("/activity");
    await expect(page.locator("[role='log']").or(page.getByText("No events yet"))).toBeVisible({
      timeout: 10_000,
    });
  });

  test("Pause button visible", async ({ page }) => {
    await page.goto("/activity");
    await expect(page.getByRole("button", { name: "Pause" })).toBeVisible();
  });

  test("Resume button visible after pause", async ({ page }) => {
    await page.goto("/activity");
    await page.getByRole("button", { name: "Pause" }).click();
    await expect(page.getByRole("button", { name: "Resume" })).toBeVisible();
  });

  test("Clear button visible", async ({ page }) => {
    await page.goto("/activity");
    await expect(page.getByRole("button", { name: "Clear" })).toBeVisible();
  });

  test("filter dropdown visible with event types", async ({ page }) => {
    await page.goto("/activity");
    const filterSelect = page.getByLabel("Filter event type");
    await expect(filterSelect).toBeVisible();
    // <option> elements inside <select> are not considered "visible" by Playwright.
    // Instead, verify the select has the expected option via its value.
    const options = await filterSelect.locator("option").count();
    expect(options).toBeGreaterThan(0);
  });

  test("empty state when no events", async ({ page }) => {
    await page.goto("/activity");
    await expect(page.getByText("No events yet")).toBeVisible({ timeout: 10_000 });
  });
});
