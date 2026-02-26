import { chromium, type FullConfig } from "@playwright/test";
import * as fs from "fs";

const API_BASE = "http://localhost:8080/api/v1";
const ADMIN_EMAIL = "admin@localhost";
const ADMIN_PASS = "Changeme123";
const STORAGE_STATE_PATH = "./e2e/.auth/admin.json";

/**
 * Global setup: ensure admin password is changed (seeded admin requires it),
 * then login via the frontend form so the browser gets the httpOnly refresh
 * cookie. Fix the cookie's Secure flag for HTTP testing and persist storageState.
 */
export default async function globalSetup(_config: FullConfig): Promise<void> {
  // 1. Handle forced password change for seeded admin (via API)
  const loginRes = await fetch(`${API_BASE}/auth/login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email: ADMIN_EMAIL, password: ADMIN_PASS }),
  });
  if (!loginRes.ok) {
    throw new Error(`Global setup login failed (${loginRes.status}): ${await loginRes.text()}`);
  }
  const loginBody = (await loginRes.json()) as {
    access_token: string;
    user: { must_change_password?: boolean };
  };

  if (loginBody.user.must_change_password) {
    const cpRes = await fetch(`${API_BASE}/auth/change-password`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${loginBody.access_token}`,
      },
      body: JSON.stringify({ old_password: ADMIN_PASS, new_password: ADMIN_PASS }),
    });
    if (!cpRes.ok) {
      throw new Error(`Password change failed (${cpRes.status}): ${await cpRes.text()}`);
    }
  }

  // 2. Login via the frontend form so the browser gets the httpOnly refresh cookie
  const browser = await chromium.launch();
  // ignoreHTTPSErrors allows cookies with Secure flag to work over HTTP
  const context = await browser.newContext({
    baseURL: "http://localhost:3000",
    ignoreHTTPSErrors: true,
  });
  const page = await context.newPage();

  await page.goto("/login");
  await page.locator("#email").fill(ADMIN_EMAIL);
  await page.locator("#password").fill(ADMIN_PASS);
  await page.locator('button[type="submit"]').click();

  // Wait for redirect away from /login (indicates successful auth)
  await page.waitForURL(/^(?!.*\/login)/, { timeout: 15_000 });

  // 3. Save storageState (includes cookies + localStorage)
  await context.storageState({ path: STORAGE_STATE_PATH });
  await browser.close();

  // 4. Fix cookie flags for HTTP testing
  //    The server sets Secure=true + SameSite=Strict, but tests run over HTTP.
  //    Playwright respects these flags, so we need to adjust them.
  const state = JSON.parse(fs.readFileSync(STORAGE_STATE_PATH, "utf-8"));
  if (state.cookies) {
    for (const cookie of state.cookies) {
      cookie.secure = false;
      cookie.sameSite = "Lax";
    }
    fs.writeFileSync(STORAGE_STATE_PATH, JSON.stringify(state, null, 2));
  }
}
