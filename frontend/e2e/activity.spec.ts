import { expect, test } from "./fixtures";

test.describe("Activity page", () => {
  test("page heading is visible", async ({ page }) => {
    await page.goto("/activity");
    await expect(page.locator("main h1")).toHaveText("Activity");
  });

  test("WebSocket connection status badge visible", async ({ page }) => {
    await page.goto("/activity");
    // Badge shows either "Live" (connected) or "Disconnected"
    await expect(page.getByText("Live").or(page.getByText("Disconnected"))).toBeVisible({
      timeout: 10_000,
    });
  });

  test("connection badge shows connected or disconnected", async ({ page }) => {
    await page.goto("/activity");
    // The badge with pill variant contains "Live" or "Disconnected"
    const liveBadge = page.getByText("Live");
    const disconnectedBadge = page.getByText("Disconnected");
    const isLive = await liveBadge.isVisible().catch(() => false);
    const isDisconnected = await disconnectedBadge.isVisible().catch(() => false);
    expect(isLive || isDisconnected).toBe(true);
  });

  test("activity feed area visible", async ({ page }) => {
    await page.goto("/activity");
    // The feed area has role="log" when entries exist, or empty state card is shown
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
    // Should at least have "All events" option
    await expect(filterSelect.locator("option", { hasText: "All events" })).toBeVisible();
  });

  test("empty state when no events", async ({ page }) => {
    await page.goto("/activity");
    // On fresh load with no activity, the empty state message should appear
    await expect(page.getByText("No events yet")).toBeVisible({ timeout: 10_000 });
  });
});
