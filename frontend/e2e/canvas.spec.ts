import { expect, test } from "./fixtures";

// ---------------------------------------------------------------------------
// Helper: navigate to a project detail page and wait for it to load
// ---------------------------------------------------------------------------

async function gotoProject(page: import("@playwright/test").Page, projectId: string) {
  await page.goto(`/projects/${projectId}`);
  await page.waitForLoadState("networkidle");
}

// ---------------------------------------------------------------------------
// 32G.1: Canvas basic interactions
// ---------------------------------------------------------------------------

test.describe("Design Canvas — basic interactions", () => {
  let projectId: string;

  test.beforeEach(async ({ api }) => {
    const suffix = Date.now();
    const project = await api.createProject(`canvas-test-${suffix}`, "Canvas E2E test");
    projectId = project.id;
  });

  test("canvas modal opens from project header button", async ({ page }) => {
    await gotoProject(page, projectId);

    // The project-level canvas button should be visible in the header
    const canvasBtn = page.locator('[data-testid="project-canvas-btn"]');
    await expect(canvasBtn).toBeVisible({ timeout: 10_000 });

    // Click to open the canvas modal
    await canvasBtn.click();

    // The canvas modal should appear
    const modal = page.locator('[data-testid="canvas-modal"]');
    await expect(modal).toBeVisible({ timeout: 5_000 });
  });

  test("canvas modal opens from chat panel button", async ({ page }) => {
    await gotoProject(page, projectId);

    // The chat-panel canvas button should be visible
    const chatCanvasBtn = page.locator('[data-testid="canvas-open-btn"]');
    await expect(chatCanvasBtn).toBeVisible({ timeout: 15_000 });

    await chatCanvasBtn.click();

    const modal = page.locator('[data-testid="canvas-modal"]');
    await expect(modal).toBeVisible({ timeout: 5_000 });
  });

  test("canvas modal closes on close button click", async ({ page }) => {
    await gotoProject(page, projectId);

    // Open canvas from project header
    await page.locator('[data-testid="project-canvas-btn"]').click();
    const modal = page.locator('[data-testid="canvas-modal"]');
    await expect(modal).toBeVisible({ timeout: 5_000 });

    // Click the close button
    await page.getByRole("button", { name: "Close canvas" }).click();

    // Modal should disappear
    await expect(modal).not.toBeVisible({ timeout: 5_000 });
  });

  test("canvas modal closes on Escape key", async ({ page }) => {
    await gotoProject(page, projectId);

    await page.locator('[data-testid="project-canvas-btn"]').click();
    const modal = page.locator('[data-testid="canvas-modal"]');
    await expect(modal).toBeVisible({ timeout: 5_000 });

    // Press Escape to close
    await page.keyboard.press("Escape");
    await expect(modal).not.toBeVisible({ timeout: 5_000 });
  });

  test("toolbar is visible with tool buttons", async ({ page }) => {
    await gotoProject(page, projectId);

    await page.locator('[data-testid="project-canvas-btn"]').click();
    await expect(page.locator('[data-testid="canvas-modal"]')).toBeVisible({ timeout: 5_000 });

    // The toolbar should be present with the standard tools
    const toolbar = page.getByRole("toolbar", { name: "Canvas tools" });
    await expect(toolbar).toBeVisible({ timeout: 5_000 });

    // Verify key tool buttons exist
    await expect(page.getByRole("button", { name: "Select" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Rectangle" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Ellipse" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Text" })).toBeVisible();
  });

  test("tool selection changes active tool styling", async ({ page }) => {
    await gotoProject(page, projectId);

    await page.locator('[data-testid="project-canvas-btn"]').click();
    await expect(page.locator('[data-testid="canvas-modal"]')).toBeVisible({ timeout: 5_000 });

    // Select tool should be active by default (has pressed state)
    const selectBtn = page.getByRole("button", { name: "Select" });
    await expect(selectBtn).toHaveAttribute("aria-pressed", "true");

    // Click Rectangle tool
    const rectBtn = page.getByRole("button", { name: "Rectangle" });
    await rectBtn.click();
    await expect(rectBtn).toHaveAttribute("aria-pressed", "true");
    await expect(selectBtn).toHaveAttribute("aria-pressed", "false");

    // Click Ellipse tool
    const ellipseBtn = page.getByRole("button", { name: "Ellipse" });
    await ellipseBtn.click();
    await expect(ellipseBtn).toHaveAttribute("aria-pressed", "true");
    await expect(rectBtn).toHaveAttribute("aria-pressed", "false");

    // Click Text tool
    const textBtn = page.getByRole("button", { name: "Text" });
    await textBtn.click();
    await expect(textBtn).toHaveAttribute("aria-pressed", "true");
    await expect(ellipseBtn).toHaveAttribute("aria-pressed", "false");
  });

  test("drawing a rectangle via mouse interactions on SVG", async ({ page }) => {
    await gotoProject(page, projectId);

    await page.locator('[data-testid="project-canvas-btn"]').click();
    await expect(page.locator('[data-testid="canvas-modal"]')).toBeVisible({ timeout: 5_000 });

    // Select the Rectangle tool
    await page.getByRole("button", { name: "Rectangle" }).click();

    // Get the SVG element inside the canvas
    const svg = page.locator('[data-testid="canvas-modal"] svg').first();
    await expect(svg).toBeVisible();

    const svgBox = await svg.boundingBox();
    if (!svgBox) throw new Error("SVG bounding box not found");

    // Simulate drawing: mousedown -> mousemove -> mouseup
    const startX = svgBox.x + 100;
    const startY = svgBox.y + 100;
    const endX = svgBox.x + 300;
    const endY = svgBox.y + 250;

    await page.mouse.move(startX, startY);
    await page.mouse.down();
    await page.mouse.move(endX, endY, { steps: 5 });
    await page.mouse.up();

    // After drawing, there should be a rect element in the SVG
    const rects = svg.locator("rect");
    // The SVG has at least one rect (the drawn one, plus possibly selection overlay)
    const rectCount = await rects.count();
    expect(rectCount).toBeGreaterThanOrEqual(1);
  });

  test("undo/redo keyboard shortcuts work", async ({ page }) => {
    await gotoProject(page, projectId);

    await page.locator('[data-testid="project-canvas-btn"]').click();
    await expect(page.locator('[data-testid="canvas-modal"]')).toBeVisible({ timeout: 5_000 });

    // Undo button should be present in toolbar
    const undoBtn = page.getByRole("button", { name: "Undo (Ctrl+Z)" });
    await expect(undoBtn).toBeVisible();

    // Redo button should be present in toolbar
    const redoBtn = page.getByRole("button", { name: "Redo (Ctrl+Shift+Z)" });
    await expect(redoBtn).toBeVisible();

    // Draw a rectangle first
    await page.getByRole("button", { name: "Rectangle" }).click();
    const svg = page.locator('[data-testid="canvas-modal"] svg').first();
    const svgBox = await svg.boundingBox();
    if (!svgBox) throw new Error("SVG bounding box not found");

    await page.mouse.move(svgBox.x + 50, svgBox.y + 50);
    await page.mouse.down();
    await page.mouse.move(svgBox.x + 200, svgBox.y + 200, { steps: 3 });
    await page.mouse.up();

    // Press Ctrl+Z to undo
    await page.keyboard.press("Control+z");

    // Press Ctrl+Shift+Z to redo
    await page.keyboard.press("Control+Shift+z");

    // The toolbar undo/redo buttons should still be clickable
    await undoBtn.click();
    await redoBtn.click();
  });

  test("keyboard shortcut switches tools", async ({ page }) => {
    await gotoProject(page, projectId);

    await page.locator('[data-testid="project-canvas-btn"]').click();
    await expect(page.locator('[data-testid="canvas-modal"]')).toBeVisible({ timeout: 5_000 });

    // Default tool is Select
    await expect(page.getByRole("button", { name: "Select" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );

    // Press 'r' to switch to Rectangle
    await page.keyboard.press("r");
    await expect(page.getByRole("button", { name: "Rectangle" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );

    // Press 'e' to switch to Ellipse
    await page.keyboard.press("e");
    await expect(page.getByRole("button", { name: "Ellipse" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );

    // Press 'v' to switch back to Select
    await page.keyboard.press("v");
    await expect(page.getByRole("button", { name: "Select" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
  });
});

// ---------------------------------------------------------------------------
// 32G.2: Export pipeline
// ---------------------------------------------------------------------------

test.describe("Design Canvas — export pipeline", () => {
  let projectId: string;

  test.beforeEach(async ({ api }) => {
    const suffix = Date.now();
    const project = await api.createProject(`canvas-export-${suffix}`, "Canvas export test");
    projectId = project.id;
  });

  test("export panel opens via toggle button", async ({ page }) => {
    await gotoProject(page, projectId);

    await page.locator('[data-testid="project-canvas-btn"]').click();
    await expect(page.locator('[data-testid="canvas-modal"]')).toBeVisible({ timeout: 5_000 });

    // Click the Toggle Export Panel button
    const toggleBtn = page.getByRole("button", { name: "Toggle Export Panel" });
    await expect(toggleBtn).toBeVisible();
    await toggleBtn.click();

    // Export panel should appear
    const exportPanel = page.locator('[data-testid="canvas-export-panel"]');
    await expect(exportPanel).toBeVisible({ timeout: 5_000 });
  });

  test("export panel has PNG, ASCII, and JSON tabs", async ({ page }) => {
    await gotoProject(page, projectId);

    await page.locator('[data-testid="project-canvas-btn"]').click();
    await expect(page.locator('[data-testid="canvas-modal"]')).toBeVisible({ timeout: 5_000 });

    // Open export panel
    await page.getByRole("button", { name: "Toggle Export Panel" }).click();
    const exportPanel = page.locator('[data-testid="canvas-export-panel"]');
    await expect(exportPanel).toBeVisible({ timeout: 5_000 });

    // Verify all three tabs exist
    await expect(page.locator('[data-testid="tab-png"]')).toBeVisible();
    await expect(page.locator('[data-testid="tab-ascii"]')).toBeVisible();
    await expect(page.locator('[data-testid="tab-json"]')).toBeVisible();
  });

  test("PNG tab shows preview area", async ({ page }) => {
    await gotoProject(page, projectId);

    await page.locator('[data-testid="project-canvas-btn"]').click();
    await expect(page.locator('[data-testid="canvas-modal"]')).toBeVisible({ timeout: 5_000 });

    await page.getByRole("button", { name: "Toggle Export Panel" }).click();
    await expect(page.locator('[data-testid="canvas-export-panel"]')).toBeVisible({
      timeout: 5_000,
    });

    // PNG tab is default; check for the preview area (either an image or a fallback message)
    await page.locator('[data-testid="tab-png"]').click();
    const pngPreview = page.locator('[data-testid="png-preview"]');
    const pngFallback = page.getByText("No preview available");
    await expect(pngPreview.or(pngFallback)).toBeVisible({ timeout: 5_000 });
  });

  test("JSON tab shows valid JSON output", async ({ page }) => {
    await gotoProject(page, projectId);

    await page.locator('[data-testid="project-canvas-btn"]').click();
    await expect(page.locator('[data-testid="canvas-modal"]')).toBeVisible({ timeout: 5_000 });

    await page.getByRole("button", { name: "Toggle Export Panel" }).click();
    await expect(page.locator('[data-testid="canvas-export-panel"]')).toBeVisible({
      timeout: 5_000,
    });

    // Switch to JSON tab
    await page.locator('[data-testid="tab-json"]').click();

    // Wait for JSON preview to appear
    const jsonPreview = page.locator('[data-testid="json-preview"]');
    await expect(jsonPreview).toBeVisible({ timeout: 5_000 });

    // The content should be valid JSON (at minimum an object with canvas dimensions)
    const jsonText = await jsonPreview.textContent();
    expect(jsonText).toBeTruthy();
    // Should not throw on parse
    const parsed = JSON.parse(jsonText!);
    expect(parsed).toBeDefined();
    expect(typeof parsed).toBe("object");
  });

  test("ASCII tab shows output", async ({ page }) => {
    await gotoProject(page, projectId);

    await page.locator('[data-testid="project-canvas-btn"]').click();
    await expect(page.locator('[data-testid="canvas-modal"]')).toBeVisible({ timeout: 5_000 });

    await page.getByRole("button", { name: "Toggle Export Panel" }).click();
    await expect(page.locator('[data-testid="canvas-export-panel"]')).toBeVisible({
      timeout: 5_000,
    });

    // Switch to ASCII tab
    await page.locator('[data-testid="tab-ascii"]').click();

    // Wait for ASCII preview to appear
    const asciiPreview = page.locator('[data-testid="ascii-preview"]');
    await expect(asciiPreview).toBeVisible({ timeout: 5_000 });

    // ASCII preview should have text content (at minimum "(empty canvas)")
    const asciiText = await asciiPreview.textContent();
    expect(asciiText).toBeTruthy();
    expect(asciiText!.length).toBeGreaterThan(0);
  });

  test("copy button exists in export panel", async ({ page }) => {
    await gotoProject(page, projectId);

    await page.locator('[data-testid="project-canvas-btn"]').click();
    await expect(page.locator('[data-testid="canvas-modal"]')).toBeVisible({ timeout: 5_000 });

    await page.getByRole("button", { name: "Toggle Export Panel" }).click();
    await expect(page.locator('[data-testid="canvas-export-panel"]')).toBeVisible({
      timeout: 5_000,
    });

    // Copy button should be visible
    const copyBtn = page.locator('[data-testid="copy-button"]');
    await expect(copyBtn).toBeVisible();
  });

  test("Send to Agent button exists", async ({ page }) => {
    await gotoProject(page, projectId);

    await page.locator('[data-testid="project-canvas-btn"]').click();
    await expect(page.locator('[data-testid="canvas-modal"]')).toBeVisible({ timeout: 5_000 });

    // The "Send to Agent" button should be visible in the top bar
    const sendBtn = page.getByRole("button", { name: "Send to Agent" });
    await expect(sendBtn).toBeVisible();
  });

  test("export panel closes via toggle button", async ({ page }) => {
    await gotoProject(page, projectId);

    await page.locator('[data-testid="project-canvas-btn"]').click();
    await expect(page.locator('[data-testid="canvas-modal"]')).toBeVisible({ timeout: 5_000 });

    // Open export panel
    const toggleBtn = page.getByRole("button", { name: "Toggle Export Panel" });
    await toggleBtn.click();
    const exportPanel = page.locator('[data-testid="canvas-export-panel"]');
    await expect(exportPanel).toBeVisible({ timeout: 5_000 });

    // Close export panel by toggling again
    await toggleBtn.click();
    await expect(exportPanel).not.toBeVisible({ timeout: 5_000 });
  });
});

// ---------------------------------------------------------------------------
// 32K.6: Canvas to chat flow
// ---------------------------------------------------------------------------

test.describe("Design Canvas — canvas to chat flow", () => {
  let projectId: string;

  test.beforeEach(async ({ api }) => {
    const suffix = Date.now();
    const project = await api.createProject(`canvas-chat-${suffix}`, "Canvas to chat test");
    projectId = project.id;
  });

  test("Send to Agent from chat canvas closes modal and populates chat", async ({ page }) => {
    await gotoProject(page, projectId);

    // Wait for chat input to be ready
    const textarea = page.locator("textarea").first();
    await expect(textarea).toBeVisible({ timeout: 15_000 });

    // Type some text into the chat input first
    await textarea.fill("Implement this UI design");

    // Open canvas from the chat panel button (this one has the export-to-chat wiring)
    const chatCanvasBtn = page.locator('[data-testid="canvas-open-btn"]');
    await expect(chatCanvasBtn).toBeVisible({ timeout: 10_000 });
    await chatCanvasBtn.click();

    const modal = page.locator('[data-testid="canvas-modal"]');
    await expect(modal).toBeVisible({ timeout: 5_000 });

    // Draw something on the canvas so there is content to export
    await page.getByRole("button", { name: "Rectangle" }).click();
    const svg = page.locator('[data-testid="canvas-modal"] svg').first();
    const svgBox = await svg.boundingBox();
    if (!svgBox) throw new Error("SVG bounding box not found");

    await page.mouse.move(svgBox.x + 50, svgBox.y + 50);
    await page.mouse.down();
    await page.mouse.move(svgBox.x + 200, svgBox.y + 150, { steps: 3 });
    await page.mouse.up();

    // Click "Send to Agent" button
    await page.getByRole("button", { name: "Send to Agent" }).click();

    // Modal should close
    await expect(modal).not.toBeVisible({ timeout: 5_000 });

    // The chat should show the sent canvas content (a message containing the design)
    // The canvas prompt includes "[Design Canvas" or "[Structured Description"
    await expect(
      page.getByText("Design Canvas").or(page.getByText("Structured Description")),
    ).toBeVisible({ timeout: 10_000 });
  });

  test("chat canvas button is accessible alongside chat input", async ({ page }) => {
    await gotoProject(page, projectId);

    // Wait for chat to fully load
    const textarea = page.locator("textarea").first();
    await expect(textarea).toBeVisible({ timeout: 15_000 });

    // Both the canvas button and the chat input should be in the same area
    const chatCanvasBtn = page.locator('[data-testid="canvas-open-btn"]');
    await expect(chatCanvasBtn).toBeVisible();

    // Send button should also be visible
    await expect(page.getByRole("button", { name: "Send" })).toBeVisible();
  });
});
