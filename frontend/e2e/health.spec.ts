import { expect, test } from "@playwright/test";

test.describe("Health checks", () => {
  test("frontend loads and shows CodeForge heading", async ({ page }) => {
    await page.goto("/");
    await expect(page.locator("h1")).toHaveText("CodeForge");
  });

  test("sidebar shows API ok status", async ({ page }) => {
    await page.goto("/");
    await expect(page.getByText("API ok")).toBeVisible({ timeout: 10_000 });
  });

  test("Go backend health endpoint returns ok", async ({ request }) => {
    const res = await request.get("http://localhost:8080/health");
    expect(res.ok()).toBe(true);
    const body = await res.json();
    expect(body).toEqual({ status: "ok" });
  });
});
