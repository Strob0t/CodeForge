import { expect, test } from "./fixtures";

test.describe("Project management", () => {
  test.beforeEach(async ({ api }) => {
    await api.deleteAllProjects();
  });

  test("empty state shows no-projects message", async ({ page }) => {
    await page.goto("/");
    await expect(page.getByText("No projects yet")).toBeVisible();
  });

  test("create project via form", async ({ page }) => {
    await page.goto("/");
    await page.getByRole("button", { name: "Add Project" }).click();
    await page.getByLabel("Name *").fill("Test Project");
    await page.getByRole("button", { name: "Create Project" }).click();

    await expect(page.getByText("Test Project")).toBeVisible();
    await expect(page.getByText("No projects yet")).not.toBeVisible();
  });

  test("navigate to project detail page", async ({ page, api }) => {
    const project = await api.createProject("Detail Test");
    await page.goto("/");

    await page.getByRole("link", { name: "Detail Test" }).click();
    await expect(page).toHaveURL(new RegExp(`/projects/${project.id}$`));
  });

  test("delete project removes card", async ({ page, api }) => {
    await api.createProject("To Delete");
    await page.goto("/");

    await expect(page.getByText("To Delete")).toBeVisible();
    await page.getByRole("button", { name: "Delete" }).click();
    await expect(page.getByText("To Delete")).not.toBeVisible();
  });

  test("validation error on empty name", async ({ page }) => {
    await page.goto("/");
    await page.getByRole("button", { name: "Add Project" }).click();
    await page.getByRole("button", { name: "Create Project" }).click();

    await expect(page.getByText("Project name is required.")).toBeVisible();
  });
});
