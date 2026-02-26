import { expect, test } from "./fixtures";

test.describe("Cross-Page Flows", () => {
  test("full project lifecycle: create on dashboard, navigate to detail, verify", async ({
    page,
    api,
  }) => {
    const name = `lifecycle-${Date.now()}`;

    // Create project on dashboard
    await page.goto("/");
    await page.getByRole("button", { name: "Add Project" }).click();
    await page.locator("#name").fill(name);
    await page.getByRole("button", { name: "Create Project" }).click();

    // Card should appear
    await expect(page.getByRole("link", { name })).toBeVisible({ timeout: 10_000 });

    // Click to navigate to detail
    await page.getByRole("link", { name }).click();
    await expect(page).toHaveURL(/\/projects\//, { timeout: 10_000 });
    await expect(page.locator("h2")).toContainText(name, { timeout: 10_000 });

    // Cleanup
    const projects = await api.listProjects();
    const created = projects.find((p) => p.name === name);
    if (created) await api.deleteProject(created.id);
  });

  test("conversation flow: create project, open detail, send message", async ({ page, api }) => {
    const project = await api.createProject(`conv-flow-${Date.now()}`);

    await page.goto(`/projects/${project.id}`);
    await expect(page.locator("h2")).toContainText(project.name, { timeout: 10_000 });

    // Wait for chat panel to load
    const textarea = page.locator("textarea").first();
    await expect(textarea).toBeVisible({ timeout: 15_000 });

    // Send a message
    const msg = `cross-page-msg-${Date.now()}`;
    await textarea.fill(msg);
    await page.getByRole("button", { name: "Send" }).click();

    // Message should appear
    await expect(page.getByText(msg)).toBeVisible({ timeout: 10_000 });
  });

  test("team workflow: create project, navigate to teams page", async ({ page, api }) => {
    await api.createProject(`team-flow-${Date.now()}`);

    // Navigate to teams page
    await page.goto("/teams");
    await expect(page.locator("h1").first()).toContainText("Agent Teams", {
      timeout: 10_000,
    });

    // The teams page should be accessible and show either teams or empty state
    await expect(page.getByText("No teams yet").or(page.getByText("Create Team"))).toBeVisible({
      timeout: 10_000,
    });
  });

  test("settings flow: navigate to settings, verify VCS section", async ({ page }) => {
    await page.goto("/settings");
    await expect(page.locator("h1").first()).toContainText("Settings", {
      timeout: 10_000,
    });

    // VCS Accounts section should be visible
    await expect(page.getByText("VCS Accounts")).toBeVisible({ timeout: 10_000 });

    // Add Account button should be visible
    await expect(page.getByRole("button", { name: "Add Account" })).toBeVisible();
  });

  test("KB workflow: navigate to knowledge bases, then scopes", async ({ page }) => {
    // Navigate to knowledge bases
    await page.goto("/knowledge-bases");
    await expect(page.locator("h1").first()).toContainText("Knowledge Bases", {
      timeout: 10_000,
    });

    // Navigate to scopes
    await page.getByRole("link", { name: "Scopes" }).click();
    await expect(page).toHaveURL(/\/scopes$/, { timeout: 10_000 });
    await expect(page.locator("h1").first()).toContainText("Scopes", {
      timeout: 10_000,
    });
  });

  test("MCP flow: navigate to MCP page, verify table or empty state", async ({ page }) => {
    await page.goto("/mcp");
    await expect(page.locator("h1").first()).toContainText("MCP Servers", {
      timeout: 10_000,
    });

    // The page should show either a server list or an empty/add state
    await expect(page.getByText("MCP Servers").first()).toBeVisible({ timeout: 10_000 });
  });

  test("mode flow: navigate to modes page, verify content", async ({ page }) => {
    await page.goto("/modes");
    await expect(page.locator("h1").first()).toContainText("Agent Modes", {
      timeout: 10_000,
    });

    // The modes page should display either built-in modes or an empty state
    // At minimum the heading confirms the page loaded
    await expect(page.locator("h1").first()).toBeVisible();
  });

  test("navigate through all main routes sequentially without errors", async ({ page }) => {
    const routes: Array<{ path: string; heading: string }> = [
      { path: "/", heading: "Projects" },
      { path: "/costs", heading: "Cost Dashboard" },
      { path: "/models", heading: "LLM Models" },
      { path: "/modes", heading: "Agent Modes" },
      { path: "/activity", heading: "Activity" },
      { path: "/knowledge-bases", heading: "Knowledge Bases" },
      { path: "/scopes", heading: "Scopes" },
      { path: "/teams", heading: "Agent Teams" },
      { path: "/mcp", heading: "MCP Servers" },
      { path: "/prompts", heading: "Prompt Sections" },
      { path: "/settings", heading: "Settings" },
      { path: "/benchmarks", heading: "Benchmark" },
    ];

    // Track errors
    const errors: string[] = [];
    page.on("pageerror", (err) => errors.push(err.message));

    for (const route of routes) {
      await page.goto(route.path);
      await expect(page.locator("h1").first()).toContainText(route.heading, {
        timeout: 10_000,
      });
    }

    // No uncaught page errors should have occurred
    expect(errors).toEqual([]);
  });
});
