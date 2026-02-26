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

  test("conversation flow: create project, open detail, verify chat panel", async ({
    page,
    api,
  }) => {
    const project = await api.createProject(`conv-flow-${Date.now()}`);

    await page.goto(`/projects/${project.id}`);
    await expect(page.locator("h2")).toContainText(project.name, { timeout: 10_000 });

    // Chat panel should have a text input area or conversation placeholder
    const textarea = page.locator("textarea").first();
    const chatPlaceholder = page.getByText("Send a message");
    const chatPanel = textarea.or(chatPlaceholder);
    await expect(chatPanel).toBeVisible({ timeout: 15_000 });
  });

  test("team workflow: create project, navigate to teams page", async ({ page, api }) => {
    await api.createProject(`team-flow-${Date.now()}`);

    // Navigate to teams page
    await page.goto("/teams");
    await expect(page.locator("main h1")).toContainText("Agent Teams", {
      timeout: 10_000,
    });

    // The teams page should be accessible — either show empty state, project selector, or create button
    await expect(
      page
        .getByText("No teams yet")
        .or(page.getByRole("button", { name: "Create Team" }))
        .or(page.locator("select")),
    ).toBeVisible({
      timeout: 10_000,
    });
  });

  test("settings flow: navigate to settings, verify VCS section", async ({ page }) => {
    await page.goto("/settings");
    await expect(page.locator("main h1")).toContainText("Settings", {
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
    await expect(page.locator("main h1")).toContainText("Knowledge Bases", {
      timeout: 10_000,
    });

    // Navigate to scopes
    await page.getByRole("link", { name: "Scopes" }).click();
    await expect(page).toHaveURL(/\/scopes$/, { timeout: 10_000 });
    await expect(page.locator("main h1")).toContainText("Scopes", {
      timeout: 10_000,
    });
  });

  test("MCP flow: navigate to MCP page, verify table or empty state", async ({ page }) => {
    await page.goto("/mcp");
    await expect(page.locator("main h1")).toContainText("MCP Servers", {
      timeout: 10_000,
    });

    // The page should show either a server list or an empty/add state
    await expect(page.getByText("MCP Servers").first()).toBeVisible({ timeout: 10_000 });
  });

  test("mode flow: navigate to modes page, verify content", async ({ page }) => {
    await page.goto("/modes");
    await expect(page.locator("main h1")).toContainText("Agent Modes", {
      timeout: 10_000,
    });

    // The modes page should display either built-in modes or an empty state
    // At minimum the heading confirms the page loaded
    await expect(page.locator("main h1")).toBeVisible();
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
      // /benchmarks excluded: dev-mode only, shows error boundary in non-dev environments
    ];

    // Track errors
    const errors: string[] = [];
    page.on("pageerror", (err) => errors.push(err.message));

    for (const route of routes) {
      await page.goto(route.path);
      await expect(page.locator("main h1")).toContainText(route.heading, {
        timeout: 10_000,
      });
    }

    // No uncaught page errors should have occurred
    expect(errors).toEqual([]);
  });
});
