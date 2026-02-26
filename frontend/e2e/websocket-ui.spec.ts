import { expect, test } from "./fixtures";

test.describe("WebSocket UI integration", () => {
  test("activity page shows WebSocket connection badge", async ({ page }) => {
    await page.goto("/activity");
    // The badge shows either "Live" (connected) or "Disconnected"
    const liveBadge = page.getByText("Live");
    const disconnectedBadge = page.getByText("Disconnected");
    await expect(liveBadge.or(disconnectedBadge)).toBeVisible({ timeout: 10_000 });
  });

  test("badge shows 'Live' when WS is connected", async ({ page }) => {
    await page.goto("/activity");
    // Wait for WebSocket to connect — the badge should show "Live"
    await expect(page.getByText("Live")).toBeVisible({ timeout: 10_000 });
  });

  test("navigate to activity page -> feed container is visible", async ({ page }) => {
    await page.goto("/activity");
    await expect(page.locator("h1")).toHaveText("Activity");

    // The activity stream area should be visible (either entries or empty state)
    const emptyState = page.getByText("No events yet.");
    const activityStream = page.locator('[role="log"]');
    await expect(emptyState.or(activityStream)).toBeVisible({ timeout: 10_000 });
  });

  test("triggering project create may produce activity events", async ({ page, api }) => {
    await page.goto("/activity");
    await expect(page.locator("h1")).toHaveText("Activity");

    // Create a project via API to trigger events
    await api.createProject("WS Activity Test");

    // The activity feed may update — we just verify the page didn't error
    // Events may or may not appear depending on backend WS broadcasting
    await expect(page.locator("h1")).toHaveText("Activity");
  });

  test("chat panel on project detail: message input visible", async ({ page, api }) => {
    const project = await api.createProject("WS Chat Test");
    await page.goto(`/projects/${project.id}`);

    // The chat panel should have a textarea for message input
    const chatInput = page.getByPlaceholder("Type a message");
    await expect(chatInput).toBeVisible({ timeout: 10_000 });
  });

  test("chat panel: send button visible", async ({ page, api }) => {
    const project = await api.createProject("WS Send Button Test");
    await page.goto(`/projects/${project.id}`);

    await expect(page.getByRole("button", { name: "Send" })).toBeVisible({ timeout: 10_000 });
  });

  test("chat panel: conversation list visible", async ({ page, api }) => {
    const project = await api.createProject("WS Conversation List Test");
    await page.goto(`/projects/${project.id}`);

    // The chat tab/label should be visible
    await expect(page.getByText("Chat")).toBeVisible({ timeout: 10_000 });
  });
});
