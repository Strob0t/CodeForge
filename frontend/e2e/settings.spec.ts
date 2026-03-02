import { expect, test } from "./fixtures";

// Helper to ensure page is fully loaded before interacting.
// SolidJS resources may refetch when navigating back, detaching DOM elements.
async function gotoSettings(page: import("@playwright/test").Page) {
  await page.goto("/settings");
  await page.waitForLoadState("networkidle");
  await expect(page.locator("main h1")).toHaveText("Settings");
}

test.describe("Settings page", () => {
  test("general settings section visible", async ({ page }) => {
    await gotoSettings(page);
    await expect(page.getByText("General")).toBeVisible();
  });

  test("default provider input visible", async ({ page }) => {
    await gotoSettings(page);
    await expect(page.locator("#default-provider")).toBeVisible();
  });

  test("autonomy level select visible", async ({ page }) => {
    await gotoSettings(page);
    const select = page.locator("#default-autonomy");
    await expect(select).toBeVisible();
    await expect(select.locator("option")).toHaveCount(5);
  });

  test("auto-clone checkbox visible", async ({ page }) => {
    await gotoSettings(page);
    await expect(page.locator("#auto-clone")).toBeVisible();
  });

  test("save button works without errors", async ({ page }) => {
    await gotoSettings(page);
    await page.getByRole("button", { name: "Save Settings" }).click();
    const success = page.getByText("Settings saved");
    await expect(success).toBeVisible({ timeout: 10_000 });
  });

  test("VCS accounts section visible", async ({ page }) => {
    await gotoSettings(page);
    await expect(page.getByRole("heading", { name: "VCS Accounts" })).toBeVisible();
  });

  test("VCS provider dropdown has options", async ({ page }) => {
    await gotoSettings(page);
    const providerSelect = page.getByRole("combobox", { name: "Provider" });
    await expect(providerSelect).toBeVisible();
    await expect(providerSelect.locator("option[value='github']")).toBeAttached();
    await expect(providerSelect.locator("option[value='gitlab']")).toBeAttached();
    await expect(providerSelect.locator("option[value='gitea']")).toBeAttached();
    await expect(providerSelect.locator("option[value='bitbucket']")).toBeAttached();
  });

  test("add VCS account: fill label + token + submit", async ({ page }) => {
    await gotoSettings(page);
    await page.getByRole("textbox", { name: "Label" }).fill("Test VCS Account");
    await page.getByRole("textbox", { name: "Token" }).fill("ghp_test_token_1234");
    await page.getByRole("button", { name: "Add Account" }).click();
    const success = page.getByText("VCS account created");
    const accountLabel = page.getByText("Test VCS Account").first();
    await expect(success.or(accountLabel)).toBeVisible({ timeout: 10_000 });
  });

  test("delete VCS account removes from list", async ({ page }) => {
    await gotoSettings(page);
    const uniqueName = "VCS Del " + Date.now();
    await page.getByRole("textbox", { name: "Label" }).fill(uniqueName);
    await page.getByRole("textbox", { name: "Token" }).fill("ghp_delete_test_5678");
    await page.getByRole("button", { name: "Add Account" }).click();
    await expect(page.getByText(uniqueName)).toBeVisible({ timeout: 10_000 });
    await page.getByRole("button", { name: "Delete VCS account " + uniqueName }).click();
    await page.getByRole("button", { name: "Confirm" }).click();
    await expect(page.getByText(uniqueName)).not.toBeVisible({ timeout: 10_000 });
  });

  test("providers section visible", async ({ page }) => {
    await gotoSettings(page);
    await expect(page.getByRole("heading", { name: "Providers", exact: true })).toBeVisible();
  });

  test("shows provider registry cards", async ({ page }) => {
    await gotoSettings(page);
    await expect(page.getByText("Git Providers")).toBeVisible({ timeout: 10_000 });
    await expect(page.getByText("Agent Backends")).toBeVisible({ timeout: 10_000 });
    await expect(page.getByText("Spec Providers")).toBeVisible({ timeout: 10_000 });
    await expect(page.getByText("PM Providers")).toBeVisible({ timeout: 10_000 });
  });

  test("LLM health status visible", async ({ page }) => {
    await gotoSettings(page);
    await expect(page.getByText("LLM Proxy")).toBeVisible();
    const connected = page.getByText("LiteLLM connected");
    const unavailable = page.getByText("LiteLLM unavailable");
    const checking = page.getByText("Checking connection...");
    await expect(connected.or(unavailable).or(checking)).toBeVisible({ timeout: 10_000 });
  });

  test("API Keys section visible", async ({ page }) => {
    await gotoSettings(page);
    await expect(page.getByRole("heading", { name: "API Keys" })).toBeVisible();
  });

  test("create API key: fill name + submit -> key displayed", async ({ page }) => {
    await gotoSettings(page);
    await page.getByRole("textbox", { name: "API key name" }).fill("E2E Test Key");
    await page.getByRole("button", { name: "Create Key" }).click();
    await expect(page.getByText("Copy this key now")).toBeVisible({ timeout: 10_000 });
  });

  test("created key starts with cfk_", async ({ page }) => {
    await gotoSettings(page);
    await page.getByRole("textbox", { name: "API key name" }).fill("Prefix Test Key");
    await page.getByRole("button", { name: "Create Key" }).click();
    await expect(page.getByText("Copy this key now")).toBeVisible({ timeout: 10_000 });
    const alertCode = page.locator("[role='alert'] code");
    await expect(alertCode).toBeVisible({ timeout: 5_000 });
    const keyContent = await alertCode.textContent();
    expect(keyContent).toMatch(/^cfk_/);
  });

  test("delete API key removes from list", async ({ page }) => {
    await gotoSettings(page);
    const keyName = "Key Del " + Date.now();
    await page.getByRole("textbox", { name: "API key name" }).fill(keyName);
    await page.getByRole("button", { name: "Create Key" }).click();
    await expect(page.getByText(keyName)).toBeVisible({ timeout: 10_000 });
    const dismissBtn = page.getByRole("button", { name: "Dismiss" });
    if (await dismissBtn.isVisible().catch(() => false)) {
      await dismissBtn.click();
    }
    await page.getByRole("button", { name: "Delete API key " + keyName }).click();
    await expect(page.getByText(keyName)).not.toBeVisible({ timeout: 10_000 });
  });

  test("users table visible (admin only)", async ({ page }) => {
    await gotoSettings(page);
    await expect(page.getByText("User Management")).toBeVisible();
  });

  test("user row shows email and role badge", async ({ page }) => {
    await gotoSettings(page);
    await expect(page.getByText("admin@localhost")).toBeVisible({ timeout: 10_000 });
    const table = page.locator("table");
    await expect(table.getByText("admin", { exact: true }).first()).toBeVisible();
  });

  test("delete user button visible", async ({ page }) => {
    await gotoSettings(page);
    const deleteButtons = page.locator('[aria-label^="Delete user"]');
    await expect(deleteButtons.first()).toBeVisible({ timeout: 10_000 });
  });

  test("benchmark section visible", async ({ page }) => {
    await gotoSettings(page);
    await expect(page.getByText("Developer Tools")).toBeVisible();
    await expect(page.getByText("Prompt Benchmark")).toBeVisible();
  });

  test("run benchmark form has model, prompt, temperature fields", async ({ page }) => {
    await gotoSettings(page);
    await expect(page.locator("#bench-model")).toBeVisible();
    await expect(page.locator("#bench-prompt")).toBeVisible();
    await expect(page.locator("#bench-temp")).toBeVisible();
  });
});
