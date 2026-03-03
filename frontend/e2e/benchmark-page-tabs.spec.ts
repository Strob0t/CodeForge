import { expect, test, ensureBenchmarkPage, clickTab } from "./benchmark-fixtures";

test.describe("Benchmark Page - Tabs & Navigation", () => {
  test.beforeEach(async ({ page }) => {
    const loaded = await ensureBenchmarkPage(page);
    if (!loaded) test.skip();
  });

  test("page loads with Benchmark Dashboard heading", async ({ page }) => {
    await expect(page.locator("main h1")).toHaveText("Benchmark Dashboard");
  });

  test("all 5 tabs are visible", async ({ page }) => {
    await expect(page.getByRole("tab", { name: "Runs" })).toBeVisible();
    await expect(page.getByRole("tab", { name: "Leaderboard" })).toBeVisible();
    await expect(page.getByRole("tab", { name: "Cost Analysis" })).toBeVisible();
    await expect(page.getByRole("tab", { name: "Multi-Compare" })).toBeVisible();
    await expect(page.getByRole("tab", { name: "Suites" })).toBeVisible();
  });

  test("Runs tab is active by default", async ({ page }) => {
    await expect(page.getByRole("tab", { name: "Runs" })).toHaveAttribute("aria-selected", "true");
  });

  test("clicking Leaderboard tab switches content", async ({ page }) => {
    await clickTab(page, "Leaderboard");
    // Leaderboard has "Filter by Suite" label
    await expect(page.getByText("Filter by Suite")).toBeVisible();
  });

  test("clicking Cost Analysis tab switches content", async ({ page }) => {
    await clickTab(page, "Cost Analysis");
    await expect(page.getByText("Select Run")).toBeVisible();
  });

  test("clicking Multi-Compare tab switches content", async ({ page }) => {
    await clickTab(page, "Multi-Compare");
    await expect(page.getByText("Select runs to compare")).toBeVisible();
  });

  test("clicking Suites tab switches content", async ({ page }) => {
    await clickTab(page, "Suites");
    await expect(page.getByRole("button", { name: "Create Suite" })).toBeVisible();
  });

  test("tab state persists in URL", async ({ page }) => {
    await clickTab(page, "Leaderboard");
    const url = new URL(page.url());
    expect(url.searchParams.get("tab")).toBe("leaderboard");
  });

  test("loading with ?tab=suites navigates to Suites tab", async ({ page }) => {
    await page.goto("/benchmarks?tab=suites");
    await page.locator("main h1").waitFor({ state: "visible", timeout: 5_000 });
    await expect(page.getByRole("button", { name: "Create Suite" })).toBeVisible();
  });

  test("returning to Runs tab shows run-related content", async ({ page }) => {
    await clickTab(page, "Leaderboard");
    await clickTab(page, "Runs");
    // Either the New Run button or empty state should be visible
    const newRunBtn = page.getByRole("button", { name: "New Run" });
    const emptyState = page.getByText("No benchmark runs yet.");
    await expect(newRunBtn.or(emptyState)).toBeVisible();
  });
});
