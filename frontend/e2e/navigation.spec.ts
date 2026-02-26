import { expect, test } from "./fixtures";

test.describe("Sidebar navigation", () => {
  test("root shows Projects heading", async ({ page }) => {
    await page.goto("/");
    await expect(page.locator("main h1")).toContainText("Projects", { timeout: 10_000 });
  });

  test("navigate to Costs page", async ({ page }) => {
    await page.goto("/");
    await page.getByRole("link", { name: "Costs" }).click();
    await expect(page).toHaveURL(/\/costs$/);
    await expect(page.locator("main h1")).toContainText("Cost Dashboard", { timeout: 10_000 });
  });

  test("navigate to Models page", async ({ page }) => {
    await page.goto("/");
    await page.getByRole("link", { name: "Models" }).click();
    await expect(page).toHaveURL(/\/models$/);
    await expect(page.locator("main h1")).toContainText("LLM Models", { timeout: 10_000 });
  });

  test("navigate back to Dashboard", async ({ page }) => {
    await page.goto("/models");
    await page.getByRole("link", { name: "Dashboard" }).click();
    await expect(page).toHaveURL(/\/$/);
    await expect(page.locator("main h1")).toContainText("Projects", { timeout: 10_000 });
  });

  test("navigate to Modes page", async ({ page }) => {
    await page.goto("/");
    await page.getByRole("link", { name: "Modes" }).click();
    await expect(page).toHaveURL(/\/modes$/);
    await expect(page.locator("main h1")).toContainText("Agent Modes", { timeout: 10_000 });
  });

  test("navigate to Activity page", async ({ page }) => {
    await page.goto("/");
    await page.getByRole("link", { name: "Activity" }).click();
    await expect(page).toHaveURL(/\/activity$/);
    await expect(page.locator("main h1")).toContainText("Activity", { timeout: 10_000 });
  });

  test("navigate to Knowledge Bases page", async ({ page }) => {
    await page.goto("/");
    await page.getByRole("link", { name: "Knowledge Bases" }).click();
    await expect(page).toHaveURL(/\/knowledge-bases$/);
    await expect(page.locator("main h1")).toContainText("Knowledge Bases", { timeout: 10_000 });
  });

  test("navigate to Scopes page", async ({ page }) => {
    await page.goto("/");
    await page.getByRole("link", { name: "Scopes" }).click();
    await expect(page).toHaveURL(/\/scopes$/);
    await expect(page.locator("main h1")).toContainText("Scopes", { timeout: 10_000 });
  });

  test("navigate to Teams page", async ({ page }) => {
    await page.goto("/");
    await page.getByRole("link", { name: "Teams" }).click();
    await expect(page).toHaveURL(/\/teams$/);
    await expect(page.locator("main h1")).toContainText("Agent Teams", { timeout: 10_000 });
  });

  test("navigate to MCP Servers page", async ({ page }) => {
    await page.goto("/");
    await page.getByRole("link", { name: "MCP Servers" }).click();
    await expect(page).toHaveURL(/\/mcp$/);
    await expect(page.locator("main h1")).toContainText("MCP Servers", { timeout: 10_000 });
  });

  test("navigate to Prompts page", async ({ page }) => {
    await page.goto("/");
    await page.getByRole("link", { name: "Prompts" }).click();
    await expect(page).toHaveURL(/\/prompts$/);
    await expect(page.locator("main h1")).toContainText("Prompt Sections", { timeout: 10_000 });
  });

  test("navigate to Settings page", async ({ page }) => {
    await page.goto("/");
    await page.getByRole("link", { name: "Settings" }).click();
    await expect(page).toHaveURL(/\/settings$/);
    await expect(page.locator("main h1")).toContainText("Settings", { timeout: 10_000 });
  });

  test("navigate to Benchmarks page", async ({ page }) => {
    await page.goto("/");
    await page.getByRole("link", { name: "Benchmarks" }).click();
    await expect(page).toHaveURL(/\/benchmarks$/, { timeout: 10_000 });
    // Benchmarks API requires APP_ENV=development; page may show error boundary.
  });

  test("nonexistent route shows Page not found", async ({ page }) => {
    await page.goto("/nonexistent");
    await expect(page.locator("h2")).toContainText("Page not found", { timeout: 10_000 });
  });

  test("404 Back to Dashboard button works", async ({ page }) => {
    await page.goto("/nonexistent");
    await expect(page.locator("h2")).toContainText("Page not found", { timeout: 10_000 });
    await page.getByRole("button", { name: "Back to Dashboard" }).click();
    await expect(page).toHaveURL(/\/$/, { timeout: 10_000 });
  });

  test("browser back and forward navigation", async ({ page }) => {
    await page.goto("/");
    await expect(page).toHaveURL(/\/$/);

    await page.getByRole("link", { name: "Costs" }).click();
    await expect(page).toHaveURL(/\/costs$/);

    await page.getByRole("link", { name: "Models" }).click();
    await expect(page).toHaveURL(/\/models$/);

    // Go back to Costs
    await page.goBack();
    await expect(page).toHaveURL(/\/costs$/, { timeout: 5_000 });

    // Go forward to Models
    await page.goForward();
    await expect(page).toHaveURL(/\/models$/, { timeout: 5_000 });
  });

  test("direct URL /projects/:id navigates correctly", async ({ page, api }) => {
    const project = await api.createProject(`nav-direct-${Date.now()}`);
    await page.goto(`/projects/${project.id}`);
    await expect(page.locator("h2")).toContainText(project.name, { timeout: 10_000 });
  });

  test("sidebar active state reflects current route", async ({ page }) => {
    await page.goto("/costs");
    // The NavLink uses activeClass "bg-cf-bg-surface-alt text-cf-accent"
    // SolidJS router A component sets aria-current="page" on active links
    const costsLink = page.getByRole("link", { name: "Costs" });
    await expect(costsLink).toHaveAttribute("aria-current", "page", { timeout: 10_000 });

    // Dashboard link should NOT be active
    const dashboardLink = page.getByRole("link", { name: "Dashboard" });
    await expect(dashboardLink).not.toHaveAttribute("aria-current", "page");
  });
});
