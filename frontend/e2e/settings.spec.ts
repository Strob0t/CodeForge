import { expect, test } from "./fixtures";

test.describe("Settings page", () => {
  // -- General section (5 tests) --

  test("general settings section visible", async ({ page }) => {
    await page.goto("/settings");
    await expect(page.locator("main h1")).toHaveText("Settings");
    await expect(page.getByText("General")).toBeVisible();
  });

  test("default provider input visible", async ({ page }) => {
    await page.goto("/settings");
    await expect(page.locator("#default-provider")).toBeVisible();
  });

  test("autonomy level select visible", async ({ page }) => {
    await page.goto("/settings");
    const select = page.locator("#default-autonomy");
    await expect(select).toBeVisible();
    // Should have 5 autonomy levels
    await expect(select.locator("option")).toHaveCount(5);
  });

  test("auto-clone checkbox visible", async ({ page }) => {
    await page.goto("/settings");
    await expect(page.locator("#auto-clone")).toBeVisible();
  });

  test("save button works without errors", async ({ page }) => {
    await page.goto("/settings");

    await page.getByRole("button", { name: "Save Settings" }).click();

    // Should show success toast or at least no error
    const success = page.getByText("Settings saved");
    await expect(success).toBeVisible({ timeout: 10_000 });
  });

  // -- VCS Accounts section (4 tests) --

  test("VCS accounts section visible", async ({ page }) => {
    await page.goto("/settings");
    await expect(page.getByText("VCS Accounts")).toBeVisible();
  });

  test("VCS provider dropdown has options", async ({ page }) => {
    await page.goto("/settings");
    const providerSelect = page.getByRole("combobox", { name: "Provider" });
    await expect(providerSelect).toBeVisible();

    // Check for the 4 provider options
    await expect(providerSelect.locator("option[value='github']")).toBeAttached();
    await expect(providerSelect.locator("option[value='gitlab']")).toBeAttached();
    await expect(providerSelect.locator("option[value='gitea']")).toBeAttached();
    await expect(providerSelect.locator("option[value='bitbucket']")).toBeAttached();
  });

  test("add VCS account: fill label + token + submit", async ({ page }) => {
    await page.goto("/settings");

    await page.getByRole("textbox", { name: "Label" }).fill("Test VCS Account");
    await page.getByRole("textbox", { name: "Token" }).fill("ghp_test_token_1234");

    await page.getByRole("button", { name: "Add Account" }).click();

    // Should show success toast or the account in the list
    const success = page.getByText("VCS account created");
    const accountLabel = page.getByText("Test VCS Account");
    await expect(success.or(accountLabel)).toBeVisible({ timeout: 10_000 });
  });

  test("delete VCS account removes from list", async ({ page }) => {
    await page.goto("/settings");

    // First create an account
    await page.getByRole("textbox", { name: "Label" }).fill("VCS To Delete");
    await page.getByRole("textbox", { name: "Token" }).fill("ghp_delete_test_5678");
    await page.getByRole("button", { name: "Add Account" }).click();

    await expect(page.getByText("VCS To Delete")).toBeVisible({ timeout: 10_000 });

    // Delete it — click delete button, then confirm
    await page.getByRole("button", { name: "Delete VCS account VCS To Delete" }).click();
    // Confirm dialog
    const confirmDelete = page.getByRole("button", { name: "Delete" }).last();
    if (await confirmDelete.isVisible({ timeout: 3_000 }).catch(() => false)) {
      await confirmDelete.click();
    }

    await expect(page.getByText("VCS To Delete")).not.toBeVisible({ timeout: 10_000 });
  });

  // -- Providers section (2 tests) --

  test("providers section visible", async ({ page }) => {
    await page.goto("/settings");
    await expect(page.getByText("Providers")).toBeVisible();
  });

  test("shows provider registry cards", async ({ page }) => {
    await page.goto("/settings");

    // Check for the 4 provider category cards
    await expect(page.getByText("Git Providers")).toBeVisible({ timeout: 10_000 });
    await expect(page.getByText("Agent Backends")).toBeVisible({ timeout: 10_000 });
    await expect(page.getByText("Spec Providers")).toBeVisible({ timeout: 10_000 });
    await expect(page.getByText("PM Providers")).toBeVisible({ timeout: 10_000 });
  });

  // -- LLM Health section (1 test) --

  test("LLM health status visible", async ({ page }) => {
    await page.goto("/settings");
    await expect(page.getByText("LLM Proxy")).toBeVisible();

    // Should show either "LiteLLM connected" or "LiteLLM unavailable" or "Checking connection..."
    const connected = page.getByText("LiteLLM connected");
    const unavailable = page.getByText("LiteLLM unavailable");
    const checking = page.getByText("Checking connection...");
    await expect(connected.or(unavailable).or(checking)).toBeVisible({ timeout: 10_000 });
  });

  // -- API Keys section (4 tests) --

  test("API Keys section visible", async ({ page }) => {
    await page.goto("/settings");
    await expect(page.getByText("API Keys")).toBeVisible();
  });

  test("create API key: fill name + submit -> key displayed", async ({ page }) => {
    await page.goto("/settings");

    await page.getByRole("textbox", { name: "API key name" }).fill("E2E Test Key");
    await page.getByRole("button", { name: "Create Key" }).click();

    // Should show the created key alert
    await expect(page.getByText("Copy this key now")).toBeVisible({ timeout: 10_000 });
  });

  test("created key starts with cfk_", async ({ page }) => {
    await page.goto("/settings");

    await page.getByRole("textbox", { name: "API key name" }).fill("Prefix Test Key");
    await page.getByRole("button", { name: "Create Key" }).click();

    // The key should be visible in a code element starting with cfk_
    await expect(page.getByText("Copy this key now")).toBeVisible({ timeout: 10_000 });
    const keyText = page.locator("code");
    await expect(keyText).toBeVisible();
    const keyContent = await keyText.textContent();
    expect(keyContent).toMatch(/^cfk_/);
  });

  test("delete API key removes from list", async ({ page }) => {
    await page.goto("/settings");

    // Create a key first
    await page.getByRole("textbox", { name: "API key name" }).fill("Key To Delete");
    await page.getByRole("button", { name: "Create Key" }).click();

    await expect(page.getByText("Key To Delete")).toBeVisible({ timeout: 10_000 });

    // Dismiss the created key alert if visible
    const dismissBtn = page.getByRole("button", { name: "Dismiss" });
    if (await dismissBtn.isVisible().catch(() => false)) {
      await dismissBtn.click();
    }

    // Delete the key
    await page.getByRole("button", { name: "Delete API key Key To Delete" }).click();

    await expect(page.getByText("Key To Delete")).not.toBeVisible({ timeout: 10_000 });
  });

  // -- User Management section (3 tests) --

  test("users table visible (admin only)", async ({ page }) => {
    await page.goto("/settings");
    await expect(page.getByText("User Management")).toBeVisible();
  });

  test("user row shows email and role badge", async ({ page }) => {
    await page.goto("/settings");

    // The admin user should be in the table
    await expect(page.getByText("admin@localhost")).toBeVisible({ timeout: 10_000 });
    await expect(page.getByText("admin", { exact: true })).toBeVisible();
  });

  test("delete user button visible", async ({ page }) => {
    await page.goto("/settings");

    // At least one delete button should be visible in the user table
    const deleteButtons = page.locator('[aria-label^="Delete user"]');
    await expect(deleteButtons.first()).toBeVisible({ timeout: 10_000 });
  });

  // -- Dev Tools section (2 tests) --

  test("benchmark section visible", async ({ page }) => {
    await page.goto("/settings");
    await expect(page.getByText("Developer Tools")).toBeVisible();
    await expect(page.getByText("Prompt Benchmark")).toBeVisible();
  });

  test("run benchmark form has model, prompt, temperature fields", async ({ page }) => {
    await page.goto("/settings");

    await expect(page.locator("#bench-model")).toBeVisible();
    await expect(page.locator("#bench-prompt")).toBeVisible();
    await expect(page.locator("#bench-temp")).toBeVisible();
  });
});
