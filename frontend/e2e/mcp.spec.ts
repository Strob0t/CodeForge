import { expect, test } from "./fixtures";

test.describe("MCP Servers page", () => {
  test("page heading 'MCP Servers' visible", async ({ page }) => {
    await page.goto("/mcp");
    await expect(page.locator("h1")).toHaveText("MCP Servers");
  });

  test("description text visible", async ({ page }) => {
    await page.goto("/mcp");
    await expect(
      page.getByText("Manage Model Context Protocol servers for agent tool access"),
    ).toBeVisible();
  });

  test("empty state when no servers", async ({ page }) => {
    await page.goto("/mcp");
    await expect(page.getByText("No MCP servers configured yet.")).toBeVisible({
      timeout: 10_000,
    });
  });

  test("'Add Server' button opens form", async ({ page }) => {
    await page.goto("/mcp");
    await expect(page.locator("#mcp-name")).not.toBeVisible();

    await page.getByRole("button", { name: "Add Server" }).click();
    await expect(page.locator("#mcp-name")).toBeVisible();
  });

  test("form has name field (required)", async ({ page }) => {
    await page.goto("/mcp");
    await page.getByRole("button", { name: "Add Server" }).click();

    const nameInput = page.locator("#mcp-name");
    await expect(nameInput).toBeVisible();
    await expect(nameInput).toHaveAttribute("aria-required", "true");
  });

  test("transport dropdown visible with options", async ({ page }) => {
    await page.goto("/mcp");
    await page.getByRole("button", { name: "Add Server" }).click();

    const transport = page.locator("#mcp-transport");
    await expect(transport).toBeVisible();

    // Check all three transport options
    await expect(transport.locator("option")).toHaveCount(3);
    await expect(transport.locator("option[value='stdio']")).toBeAttached();
    await expect(transport.locator("option[value='sse']")).toBeAttached();
    await expect(transport.locator("option[value='streamable_http']")).toBeAttached();
  });

  test("when transport=stdio: command and args fields visible", async ({ page }) => {
    await page.goto("/mcp");
    await page.getByRole("button", { name: "Add Server" }).click();

    // Default is stdio
    await expect(page.locator("#mcp-command")).toBeVisible();
    await expect(page.locator("#mcp-args")).toBeVisible();
    await expect(page.locator("#mcp-url")).not.toBeVisible();
  });

  test("when transport=sse: URL field visible, command hidden", async ({ page }) => {
    await page.goto("/mcp");
    await page.getByRole("button", { name: "Add Server" }).click();

    await page.locator("#mcp-transport").selectOption("sse");

    await expect(page.locator("#mcp-url")).toBeVisible();
    await expect(page.locator("#mcp-command")).not.toBeVisible();
    await expect(page.locator("#mcp-args")).not.toBeVisible();
  });

  test("when transport=streamable_http: URL field visible", async ({ page }) => {
    await page.goto("/mcp");
    await page.getByRole("button", { name: "Add Server" }).click();

    await page.locator("#mcp-transport").selectOption("streamable_http");

    await expect(page.locator("#mcp-url")).toBeVisible();
    await expect(page.locator("#mcp-command")).not.toBeVisible();
  });

  test("environment variables section visible", async ({ page }) => {
    await page.goto("/mcp");
    await page.getByRole("button", { name: "Add Server" }).click();

    await expect(page.getByText("Environment Variables")).toBeVisible();
    await expect(page.getByRole("button", { name: "Add Variable" })).toBeVisible();
  });

  test("enabled checkbox visible and checked by default", async ({ page }) => {
    await page.goto("/mcp");
    await page.getByRole("button", { name: "Add Server" }).click();

    const checkbox = page.locator("#mcp-enabled");
    await expect(checkbox).toBeVisible();
    await expect(checkbox).toBeChecked();
  });

  test("description field visible", async ({ page }) => {
    await page.goto("/mcp");
    await page.getByRole("button", { name: "Add Server" }).click();

    await expect(page.locator("#mcp-desc")).toBeVisible();
  });

  test("form validation on empty name shows error toast", async ({ page }) => {
    await page.goto("/mcp");
    await page.getByRole("button", { name: "Add Server" }).click();

    // Leave name empty and submit
    await page.getByRole("button", { name: "Create Server" }).click();

    await expect(page.getByText("Server name is required")).toBeVisible({ timeout: 10_000 });
  });

  test("create server via form + appears in table", async ({ page }) => {
    await page.goto("/mcp");
    await page.getByRole("button", { name: "Add Server" }).click();

    await page.locator("#mcp-name").fill("e2e-test-server");
    await page.locator("#mcp-command").fill("echo");

    // Submit — the form tests connection first, may show confirm dialog
    await page.getByRole("button", { name: "Create Server" }).click();

    // If test fails, a confirm dialog appears — click "Save Anyway"
    const saveAnyway = page.getByRole("button", { name: "Save Anyway" });
    if (await saveAnyway.isVisible({ timeout: 5_000 }).catch(() => false)) {
      await saveAnyway.click();
    }

    // Server should appear
    await expect(page.getByText("e2e-test-server")).toBeVisible({ timeout: 10_000 });
  });

  test("server table shows name, transport badge, enabled badge", async ({ page, api }) => {
    await api.createMCPServer({
      name: "table-test-server",
      transport: "stdio",
      command: "echo",
      enabled: true,
    });

    await page.goto("/mcp");

    await expect(page.getByText("table-test-server")).toBeVisible({ timeout: 10_000 });
    await expect(page.getByText("stdio")).toBeVisible();
    await expect(page.getByText("Enabled")).toBeVisible();
  });

  test("test button visible per server", async ({ page, api }) => {
    await api.createMCPServer({
      name: "test-btn-server",
      transport: "stdio",
      command: "echo",
      enabled: true,
    });

    await page.goto("/mcp");
    await expect(
      page.getByRole("button", { name: "Test connection for test-btn-server" }),
    ).toBeVisible({ timeout: 10_000 });
  });

  test("edit button visible per server", async ({ page, api }) => {
    await api.createMCPServer({
      name: "edit-btn-server",
      transport: "stdio",
      command: "echo",
      enabled: true,
    });

    await page.goto("/mcp");
    await expect(page.getByRole("button", { name: "Edit server edit-btn-server" })).toBeVisible({
      timeout: 10_000,
    });
  });

  test("delete button removes server from table", async ({ page, api }) => {
    await api.createMCPServer({
      name: "delete-me-server",
      transport: "stdio",
      command: "echo",
      enabled: true,
    });

    await page.goto("/mcp");
    await expect(page.getByText("delete-me-server")).toBeVisible({ timeout: 10_000 });

    // Click delete, then confirm dialog
    await page.getByRole("button", { name: "Delete server delete-me-server" }).click();
    await page.getByRole("button", { name: "Delete" }).last().click();

    await expect(page.getByText("delete-me-server")).not.toBeVisible({ timeout: 10_000 });
  });
});
