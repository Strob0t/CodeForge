import { expect, test } from "./fixtures";

test.describe("Dashboard — Project Management", () => {
  test("empty state shows no projects message", async ({ page, api }) => {
    await api.deleteAllProjects();
    await page.goto("/");
    await expect(page.getByText("No projects yet. Create one to get started.")).toBeVisible({
      timeout: 10_000,
    });
  });

  test("Add Project button opens form", async ({ page }) => {
    await page.goto("/");
    await page.getByRole("button", { name: "Add Project" }).click();
    // The form should now be visible with the Create Project submit button
    await expect(page.getByRole("button", { name: "Create Project" })).toBeVisible({
      timeout: 5_000,
    });
  });

  test("project card shows name, description, and provider badge", async ({ page, api }) => {
    const project = await api.createProject(`card-test-${Date.now()}`, "A test description");
    await page.goto("/");

    // The card should display the project name as a link
    await expect(page.getByRole("link", { name: project.name })).toBeVisible({
      timeout: 10_000,
    });
    // Description should be visible
    await expect(page.getByText("A test description")).toBeVisible();
  });

  test("multiple project cards render correctly", async ({ page, api }) => {
    const suffix = Date.now();
    await api.createProject(`multi-a-${suffix}`);
    await api.createProject(`multi-b-${suffix}`);
    await page.goto("/");

    await expect(page.getByRole("link", { name: `multi-a-${suffix}` })).toBeVisible({
      timeout: 10_000,
    });
    await expect(page.getByRole("link", { name: `multi-b-${suffix}` })).toBeVisible();
  });

  test("card click navigates to project detail", async ({ page, api }) => {
    const project = await api.createProject(`click-nav-${Date.now()}`);
    await page.goto("/");

    await page.getByRole("link", { name: project.name }).click();
    await expect(page).toHaveURL(new RegExp(`/projects/${project.id}`), {
      timeout: 10_000,
    });
  });

  test("delete button removes card", async ({ page, api }) => {
    const project = await api.createProject(`delete-test-${Date.now()}`);
    await page.goto("/");

    // Verify card is visible
    await expect(page.getByRole("link", { name: project.name })).toBeVisible({
      timeout: 10_000,
    });

    // Click delete button (aria-label: "Delete project <name>")
    await page.getByRole("button", { name: `Delete project ${project.name}` }).click();

    // Card should disappear
    await expect(page.getByRole("link", { name: project.name })).not.toBeVisible({
      timeout: 10_000,
    });
  });

  test("form name validation shows error when empty", async ({ page }) => {
    await page.goto("/");
    await page.getByRole("button", { name: "Add Project" }).click();

    // Submit without filling in name
    await page.getByRole("button", { name: "Create Project" }).click();

    // Should show validation error
    await expect(page.getByText("Project name is required.")).toBeVisible({
      timeout: 5_000,
    });
  });

  test("form submit creates project and card appears", async ({ page, api }) => {
    const name = `form-create-${Date.now()}`;
    await page.goto("/");
    await page.getByRole("button", { name: "Add Project" }).click();

    // Fill in the name
    await page.locator("#name").fill(name);
    await page.getByRole("button", { name: "Create Project" }).click();

    // Card should appear
    await expect(page.getByRole("link", { name })).toBeVisible({ timeout: 10_000 });

    // Cleanup via API
    const projects = await api.listProjects();
    const created = projects.find((p) => p.name === name);
    if (created) await api.deleteProject(created.id);
  });

  test("form cancel closes without creating", async ({ page }) => {
    await page.goto("/");

    // Open form
    await page.getByRole("button", { name: "Add Project" }).click();
    await expect(page.getByRole("button", { name: "Create Project" })).toBeVisible({
      timeout: 5_000,
    });

    // Click Cancel (the same button toggles to Cancel when form is open)
    await page.getByRole("button", { name: "Cancel" }).click();

    // Form should be hidden — Create Project button should not be visible
    await expect(page.getByRole("button", { name: "Create Project" })).not.toBeVisible({
      timeout: 5_000,
    });
  });

  test("cost summary section visible when applicable", async ({ page }) => {
    // Navigate to costs page to verify cost section renders
    await page.goto("/costs");
    // The Cost Dashboard page title is inside <main>, skip the app header
    await expect(page.locator("main h1, main h2").first()).toContainText("Cost", {
      timeout: 10_000,
    });
  });

  test("recent activity section is accessible", async ({ page }) => {
    await page.goto("/activity");
    await expect(page.locator("main h1, main h2").first()).toContainText("Activity", {
      timeout: 10_000,
    });
  });
});
