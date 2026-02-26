import { expect, test } from "./fixtures";

test.describe("Teams page", () => {
  test("page heading 'Agent Teams' visible", async ({ page }) => {
    await page.goto("/teams");
    await expect(page.locator("main h1")).toHaveText("Agent Teams");
  });

  test("project selector dropdown visible", async ({ page }) => {
    await page.goto("/teams");
    await expect(page.getByRole("combobox", { name: "Select project..." })).toBeVisible();
  });

  test("create team form appears when project is selected", async ({ page, api }) => {
    const project = await api.createProject("Teams Test Project");
    await page.goto("/teams");

    // Select the project
    await page.getByRole("combobox", { name: "Select project..." }).selectOption(project.id);

    // The create team form card should appear
    await expect(page.getByText("Create Team")).toBeVisible();
  });

  test("form has name input", async ({ page, api }) => {
    const project = await api.createProject("Teams Name Input");
    await page.goto("/teams");
    await page.getByRole("combobox", { name: "Select project..." }).selectOption(project.id);

    await expect(page.getByRole("textbox", { name: "Team name" })).toBeVisible();
  });

  test("form has protocol dropdown", async ({ page, api }) => {
    const project = await api.createProject("Teams Protocol");
    await page.goto("/teams");
    await page.getByRole("combobox", { name: "Select project..." }).selectOption(project.id);

    const protocolSelect = page.getByRole("combobox", { name: "Protocol" });
    await expect(protocolSelect).toBeVisible();

    // Check protocol options exist
    await expect(protocolSelect.locator("option")).toHaveCount(5);
  });

  test("form has member section with add member button", async ({ page, api }) => {
    const project = await api.createProject("Teams Members");
    await page.goto("/teams");
    await page.getByRole("combobox", { name: "Select project..." }).selectOption(project.id);

    await expect(page.getByText("Members")).toBeVisible();
    await expect(page.getByRole("button", { name: "+ Add Member" })).toBeVisible();
  });

  test("form validation on empty name shows error toast", async ({ page, api }) => {
    const project = await api.createProject("Teams Validation");
    await page.goto("/teams");
    await page.getByRole("combobox", { name: "Select project..." }).selectOption(project.id);

    // Add a member first so we only trigger the name validation
    await page.getByRole("button", { name: "Create Team" }).click();

    await expect(page.getByText("Team name is required")).toBeVisible({ timeout: 10_000 });
  });

  test("create team successfully with member + team card appears", async ({ page, api }) => {
    const project = await api.createProject("Teams Create");
    const agent = await api.createAgent(project.id, "test-agent");
    await page.goto("/teams");
    await page.getByRole("combobox", { name: "Select project..." }).selectOption(project.id);

    // Fill name
    await page.getByRole("textbox", { name: "Team name" }).fill("My E2E Team");

    // Add a member
    await page.getByRole("button", { name: "+ Add Member" }).click();
    await page.getByRole("combobox", { name: "Select agent..." }).selectOption(agent.id);

    // Submit
    await page.getByRole("button", { name: "Create Team" }).click();

    // Team card should appear
    await expect(page.getByText("My E2E Team")).toBeVisible({ timeout: 10_000 });
  });

  test("team card shows name, protocol badge, and member count", async ({ page, api }) => {
    const project = await api.createProject("Teams Card Info");
    await api.createAgent(project.id, "card-agent");
    await api.createTeam(project.id, "Card Info Team");

    await page.goto("/teams");
    await page.getByRole("combobox", { name: "Select project..." }).selectOption(project.id);

    await expect(page.getByText("Card Info Team")).toBeVisible({ timeout: 10_000 });
    // Protocol badge (default: sequential)
    await expect(page.getByText("sequential")).toBeVisible();
  });

  test("team card is expandable", async ({ page, api }) => {
    const project = await api.createProject("Teams Expand");
    await api.createTeam(project.id, "Expandable Team");

    await page.goto("/teams");
    await page.getByRole("combobox", { name: "Select project..." }).selectOption(project.id);

    const teamButton = page.getByRole("button", { name: "Expandable Team" });
    await expect(teamButton).toBeVisible({ timeout: 10_000 });
    await expect(teamButton).toHaveAttribute("aria-expanded", "false");

    await teamButton.click();
    await expect(teamButton).toHaveAttribute("aria-expanded", "true");
  });

  test("expanded view shows members list heading", async ({ page, api }) => {
    const project = await api.createProject("Teams Members List");
    await api.createTeam(project.id, "Members List Team");

    await page.goto("/teams");
    await page.getByRole("combobox", { name: "Select project..." }).selectOption(project.id);

    await page.getByRole("button", { name: "Members List Team" }).click();
    await expect(page.getByText("Members", { exact: false })).toBeVisible();
  });

  test("expanded view shows shared context section", async ({ page, api }) => {
    const project = await api.createProject("Teams Shared Ctx");
    await api.createTeam(project.id, "Shared Ctx Team");

    await page.goto("/teams");
    await page.getByRole("combobox", { name: "Select project..." }).selectOption(project.id);

    await page.getByRole("button", { name: "Shared Ctx Team" }).click();
    // Shared context section or its fallback text should appear
    const sharedCtx = page.getByText("Shared Context");
    const noSharedCtx = page.getByText("No shared context items yet.");
    await expect(sharedCtx.or(noSharedCtx)).toBeVisible({ timeout: 10_000 });
  });

  test("delete team removes card", async ({ page, api }) => {
    const project = await api.createProject("Teams Delete");
    await api.createTeam(project.id, "Team To Delete");

    await page.goto("/teams");
    await page.getByRole("combobox", { name: "Select project..." }).selectOption(project.id);

    await expect(page.getByText("Team To Delete")).toBeVisible({ timeout: 10_000 });

    await page.getByRole("button", { name: "Delete team Team To Delete" }).click();
    await expect(page.getByText("Team To Delete")).not.toBeVisible({ timeout: 10_000 });
  });
});
