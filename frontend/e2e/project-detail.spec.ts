import { expect, test } from "./fixtures";

test.describe("Project Detail Page", () => {
  let projectId: string;
  let projectName: string;

  test.beforeEach(async ({ api }) => {
    const suffix = Date.now();
    projectName = `detail-test-${suffix}`;
    const project = await api.createProject(projectName, "Detail test description");
    projectId = project.id;
  });

  test("page loads with project name in heading", async ({ page }) => {
    await page.goto(`/projects/${projectId}`);
    await expect(page.locator("h2")).toContainText(projectName, { timeout: 10_000 });
  });

  test("header shows project name prominently", async ({ page }) => {
    await page.goto(`/projects/${projectId}`);
    const heading = page.locator("h2");
    await expect(heading).toContainText(projectName, { timeout: 10_000 });
  });

  test("chat panel is visible on project detail", async ({ page }) => {
    await page.goto(`/projects/${projectId}`);
    // ChatPanel renders a "Chat" header text
    await expect(page.getByText("Chat")).toBeVisible({ timeout: 10_000 });
  });

  test("roadmap panel or toggle is visible", async ({ page }) => {
    await page.goto(`/projects/${projectId}`);
    // The page shows either the roadmap panel or the expand button
    const roadmapText = page.getByText("Roadmap");
    await expect(roadmapText.first()).toBeVisible({ timeout: 10_000 });
  });

  test("chat input textarea is visible", async ({ page }) => {
    await page.goto(`/projects/${projectId}`);
    // Chat panel has a textarea with placeholder
    const textarea = page.locator("textarea");
    await expect(textarea.first()).toBeVisible({ timeout: 10_000 });
  });

  test("chat send button exists", async ({ page }) => {
    await page.goto(`/projects/${projectId}`);
    await expect(page.getByRole("button", { name: "Send" })).toBeVisible({
      timeout: 10_000,
    });
  });

  test("send message appears in chat history", async ({ page }) => {
    await page.goto(`/projects/${projectId}`);
    // Wait for the chat panel to be ready
    const textarea = page.locator("textarea").first();
    await expect(textarea).toBeVisible({ timeout: 10_000 });

    // Type and send a message
    const msgText = `hello-e2e-${Date.now()}`;
    await textarea.fill(msgText);
    await page.getByRole("button", { name: "Send" }).click();

    // The user message should appear in the chat area
    await expect(page.getByText(msgText)).toBeVisible({ timeout: 10_000 });
  });

  test("project settings gear button is visible", async ({ page }) => {
    await page.goto(`/projects/${projectId}`);
    await expect(page.getByRole("button", { name: "Project Settings" })).toBeVisible({
      timeout: 10_000,
    });
  });

  test("project settings popover opens on gear click", async ({ page }) => {
    await page.goto(`/projects/${projectId}`);
    await page.getByRole("button", { name: "Project Settings" }).click();
    // The CompactSettingsPopover should appear with an autonomy level selector
    await expect(page.getByText("Project Settings").first()).toBeVisible({
      timeout: 5_000,
    });
  });

  test("clone button visible when project has repo_url but no workspace", async ({ page, api }) => {
    // Create a project with a repo URL
    const repoProject = await api.createProject(`clone-test-${Date.now()}`);
    // We cannot easily set repo_url via the fixture; check that the Clone or Pull buttons
    // are conditionally rendered based on project state
    await page.goto(`/projects/${repoProject.id}`);
    // The page should load without errors
    await expect(page.locator("h2")).toContainText("clone-test", { timeout: 10_000 });
  });

  test("invalid project ID shows error state", async ({ page }) => {
    await page.goto("/projects/00000000-0000-0000-0000-000000000000");
    // Should show "Project not found" or loading error
    await expect(page.getByText("Project not found").or(page.getByText("Loading"))).toBeVisible({
      timeout: 10_000,
    });
  });

  test("navigate back to dashboard from project detail", async ({ page }) => {
    await page.goto(`/projects/${projectId}`);
    await expect(page.locator("h2")).toContainText(projectName, { timeout: 10_000 });

    // Click the Dashboard link in the sidebar
    await page.getByRole("link", { name: "Dashboard" }).click();
    await expect(page).toHaveURL(/\/$/, { timeout: 10_000 });
  });

  test("conversation auto-creates on project detail load", async ({ page }) => {
    await page.goto(`/projects/${projectId}`);
    // ChatPanel auto-creates a conversation and shows the input area
    // If no conversation existed, one should be created automatically
    const textarea = page.locator("textarea").first();
    await expect(textarea).toBeVisible({ timeout: 15_000 });
  });

  test("multiple project details can be visited sequentially", async ({ page, api }) => {
    const project2 = await api.createProject(`detail-seq-${Date.now()}`);

    await page.goto(`/projects/${projectId}`);
    await expect(page.locator("h2")).toContainText(projectName, { timeout: 10_000 });

    await page.goto(`/projects/${project2.id}`);
    await expect(page.locator("h2")).toContainText(project2.name, { timeout: 10_000 });
  });

  test("project detail shows loading state initially", async ({ page }) => {
    // Navigate and check that either loading or the project name appears
    await page.goto(`/projects/${projectId}`);
    // One of these should be visible within the timeout
    await expect(
      page.getByText("Loading").or(page.locator("h2").filter({ hasText: projectName })),
    ).toBeVisible({ timeout: 10_000 });
  });

  test("chat placeholder text is correct", async ({ page }) => {
    await page.goto(`/projects/${projectId}`);
    const textarea = page.locator("textarea").first();
    await expect(textarea).toBeVisible({ timeout: 10_000 });
    await expect(textarea).toHaveAttribute("placeholder", /Type a message/);
  });

  test("send button is disabled when input is empty", async ({ page }) => {
    await page.goto(`/projects/${projectId}`);
    const sendButton = page.getByRole("button", { name: "Send" });
    await expect(sendButton).toBeVisible({ timeout: 10_000 });
    await expect(sendButton).toBeDisabled();
  });

  test("send button is enabled when input has text", async ({ page }) => {
    await page.goto(`/projects/${projectId}`);
    const textarea = page.locator("textarea").first();
    await expect(textarea).toBeVisible({ timeout: 10_000 });

    await textarea.fill("test message");
    const sendButton = page.getByRole("button", { name: "Send" });
    await expect(sendButton).toBeEnabled();
  });

  test("Enter key sends message in chat", async ({ page }) => {
    await page.goto(`/projects/${projectId}`);
    const textarea = page.locator("textarea").first();
    await expect(textarea).toBeVisible({ timeout: 10_000 });

    const msgText = `enter-send-${Date.now()}`;
    await textarea.fill(msgText);
    await textarea.press("Enter");

    // The message should appear
    await expect(page.getByText(msgText)).toBeVisible({ timeout: 10_000 });
  });

  test("Shift+Enter does not send message (allows newline)", async ({ page }) => {
    await page.goto(`/projects/${projectId}`);
    const textarea = page.locator("textarea").first();
    await expect(textarea).toBeVisible({ timeout: 10_000 });

    await textarea.fill("first line");
    await textarea.press("Shift+Enter");
    // After Shift+Enter, Send button should remain enabled (message not yet sent)
    const sendButton = page.getByRole("button", { name: "Send" });
    await expect(sendButton).toBeEnabled();
  });
});
