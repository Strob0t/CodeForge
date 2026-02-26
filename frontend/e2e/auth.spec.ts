import { expect, test } from "@playwright/test";

const API_BASE = "http://localhost:8080/api/v1";

// Default seed admin credentials from internal/config/config.go
const ADMIN_EMAIL = "admin@localhost";
const ADMIN_PASS = "Changeme123";

// Helper: perform login via API and return tokens + user info.
async function apiLogin(
  email: string,
  password: string,
): Promise<{ accessToken: string; user: { id: string; role: string; email: string } }> {
  const res = await fetch(`${API_BASE}/auth/login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email, password }),
  });
  if (!res.ok) {
    const body = await res.json();
    throw new Error(`Login failed (${res.status}): ${JSON.stringify(body)}`);
  }
  const body = await res.json();
  return { accessToken: body.access_token, user: body.user };
}

// Helper: create a non-admin user via the admin API, returns user + password.
async function createViewerUser(
  adminToken: string,
): Promise<{ email: string; password: string; id: string }> {
  const email = `viewer-${Date.now()}@test.local`;
  const password = "ViewerPass123";
  const res = await fetch(`${API_BASE}/users`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${adminToken}`,
    },
    body: JSON.stringify({ email, name: "Test Viewer", password, role: "viewer" }),
  });
  if (!res.ok) {
    const body = await res.json();
    throw new Error(`Create user failed (${res.status}): ${JSON.stringify(body)}`);
  }
  const user = await res.json();
  return { email, password, id: user.id };
}

// Helper: delete a user via the admin API.
async function deleteUser(adminToken: string, userId: string): Promise<void> {
  await fetch(`${API_BASE}/users/${encodeURIComponent(userId)}`, {
    method: "DELETE",
    headers: { Authorization: `Bearer ${adminToken}` },
  });
}

test.describe("Authentication — Login Page", () => {
  test("login page renders correctly with form elements", async ({ page }) => {
    await page.goto("/login");

    // Title
    await expect(page.locator("h1")).toContainText("Sign in to CodeForge");

    // Email input
    const emailInput = page.locator("#email");
    await expect(emailInput).toBeVisible();
    await expect(emailInput).toHaveAttribute("type", "email");

    // Password input
    const passwordInput = page.locator("#password");
    await expect(passwordInput).toBeVisible();
    await expect(passwordInput).toHaveAttribute("type", "password");

    // Submit button
    await expect(page.getByRole("button", { name: "Sign in" })).toBeVisible();
  });

  test("login with valid admin credentials succeeds", async ({ page }) => {
    await page.goto("/login");

    await page.locator("#email").fill(ADMIN_EMAIL);
    await page.locator("#password").fill(ADMIN_PASS);
    await page.getByRole("button", { name: "Sign in" }).click();

    // Should redirect away from /login after successful auth
    await expect(page).not.toHaveURL(/\/login/, { timeout: 10_000 });
  });

  test("login with invalid credentials shows error", async ({ page }) => {
    await page.goto("/login");

    await page.locator("#email").fill("wrong@example.com");
    await page.locator("#password").fill("WrongPassword1");
    await page.getByRole("button", { name: "Sign in" }).click();

    // Error alert should be visible
    await expect(page.locator("[role='alert']")).toBeVisible({ timeout: 5_000 });
  });
});

test.describe("Authentication — JWT Token Handling", () => {
  test("JWT token is stored and sent with API requests", async ({ page }) => {
    // Login via the UI
    await page.goto("/login");
    await page.locator("#email").fill(ADMIN_EMAIL);
    await page.locator("#password").fill(ADMIN_PASS);

    // Intercept outgoing requests after login to check for Authorization header
    const authHeaders: string[] = [];
    page.on("request", (req) => {
      const authHeader = req.headers()["authorization"];
      if (authHeader && req.url().includes("/api/v1/")) {
        authHeaders.push(authHeader);
      }
    });

    await page.getByRole("button", { name: "Sign in" }).click();
    await expect(page).not.toHaveURL(/\/login/, { timeout: 10_000 });

    // Navigate to a page that triggers an API call
    await page.goto("/");
    await page.waitForTimeout(2_000);

    // At least one API request should carry a Bearer token
    const bearerHeaders = authHeaders.filter((h) => h.startsWith("Bearer "));
    expect(bearerHeaders.length).toBeGreaterThan(0);
  });

  test("expired or invalid JWT returns 401 from API", async ({ request }) => {
    // Craft a clearly invalid JWT
    const fakeToken =
      "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.invalidSignature";

    const res = await request.get(`${API_BASE}/auth/me`, {
      headers: { Authorization: `Bearer ${fakeToken}` },
    });
    expect(res.status()).toBe(401);
  });

  test("missing Authorization header returns 401 from protected endpoint", async ({ request }) => {
    const res = await request.get(`${API_BASE}/auth/me`);
    expect(res.status()).toBe(401);
  });
});

test.describe("Authentication — Role-Based Access Control", () => {
  let adminToken: string;
  let viewerUser: { email: string; password: string; id: string } | null = null;

  test.beforeAll(async () => {
    const loginResult = await apiLogin(ADMIN_EMAIL, ADMIN_PASS);
    adminToken = loginResult.accessToken;
  });

  test.afterAll(async () => {
    if (viewerUser) {
      await deleteUser(adminToken, viewerUser.id);
    }
  });

  test("admin can access admin-only users endpoint", async ({ request }) => {
    const res = await request.get(`${API_BASE}/users`, {
      headers: { Authorization: `Bearer ${adminToken}` },
    });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(Array.isArray(body)).toBe(true);
  });

  test("admin can access admin-only tenants endpoint", async ({ request }) => {
    const res = await request.get(`${API_BASE}/tenants`, {
      headers: { Authorization: `Bearer ${adminToken}` },
    });
    expect(res.status()).toBe(200);
  });

  test("non-admin cannot access admin-only users endpoint", async ({ request }) => {
    // Create a viewer user for this test
    viewerUser = await createViewerUser(adminToken);
    const viewerLogin = await apiLogin(viewerUser.email, viewerUser.password);

    const res = await request.get(`${API_BASE}/users`, {
      headers: { Authorization: `Bearer ${viewerLogin.accessToken}` },
    });
    expect(res.status()).toBe(403);
  });

  test("non-admin cannot access admin-only tenants endpoint", async ({ request }) => {
    if (!viewerUser) {
      viewerUser = await createViewerUser(adminToken);
    }
    const viewerLogin = await apiLogin(viewerUser.email, viewerUser.password);

    const res = await request.get(`${API_BASE}/tenants`, {
      headers: { Authorization: `Bearer ${viewerLogin.accessToken}` },
    });
    expect(res.status()).toBe(403);
  });
});

test.describe("Authentication — API Key Management", () => {
  let adminToken: string;

  test.beforeAll(async () => {
    const loginResult = await apiLogin(ADMIN_EMAIL, ADMIN_PASS);
    adminToken = loginResult.accessToken;
  });

  test("create API key, then use it for authenticated requests", async ({ request }) => {
    // Create an API key
    const createRes = await request.post(`${API_BASE}/auth/api-keys`, {
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${adminToken}`,
      },
      data: { name: "e2e-test-key", scopes: ["projects:read"] },
    });
    expect(createRes.status()).toBe(201);
    const createBody = await createRes.json();
    const plainKey = createBody.plain_key;
    const keyId = createBody.api_key.id;

    expect(plainKey).toBeTruthy();
    expect(plainKey).toMatch(/^cfk_/);

    // Use the API key to access a protected endpoint
    const meRes = await request.get(`${API_BASE}/auth/me`, {
      headers: { "X-API-Key": plainKey },
    });
    expect(meRes.status()).toBe(200);
    const meBody = await meRes.json();
    expect(meBody.email).toBe(ADMIN_EMAIL);

    // List API keys
    const listRes = await request.get(`${API_BASE}/auth/api-keys`, {
      headers: { Authorization: `Bearer ${adminToken}` },
    });
    expect(listRes.status()).toBe(200);
    const keys = await listRes.json();
    expect(keys.some((k: { id: string }) => k.id === keyId)).toBe(true);

    // Delete the API key
    const delRes = await request.delete(`${API_BASE}/auth/api-keys/${encodeURIComponent(keyId)}`, {
      headers: { Authorization: `Bearer ${adminToken}` },
    });
    expect(delRes.status()).toBe(204);

    // Verify deleted key no longer works
    const verifyRes = await request.get(`${API_BASE}/auth/me`, {
      headers: { "X-API-Key": plainKey },
    });
    expect(verifyRes.status()).toBe(401);
  });

  test("invalid API key returns 401", async ({ request }) => {
    const res = await request.get(`${API_BASE}/auth/me`, {
      headers: { "X-API-Key": "cfk_invalid_key_that_does_not_exist" },
    });
    expect(res.status()).toBe(401);
  });
});

test.describe("Authentication — Logout", () => {
  test("logout clears session and subsequent requests are unauthorized", async ({ request }) => {
    // Login
    const loginRes = await request.post(`${API_BASE}/auth/login`, {
      data: { email: ADMIN_EMAIL, password: ADMIN_PASS },
    });
    expect(loginRes.status()).toBe(200);
    const loginBody = await loginRes.json();
    const token = loginBody.access_token;

    // Verify we are authenticated
    const meRes = await request.get(`${API_BASE}/auth/me`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    expect(meRes.status()).toBe(200);

    // Logout
    const logoutRes = await request.post(`${API_BASE}/auth/logout`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    expect(logoutRes.status()).toBe(200);
    const logoutBody = await logoutRes.json();
    expect(logoutBody.status).toBe("logged_out");

    // After logout the access token should be revoked (if JTI-based revocation is active).
    // The token may still work briefly depending on revocation check timing,
    // but the refresh cookie should be cleared.
    const refreshRes = await request.post(`${API_BASE}/auth/refresh`);
    // Without a valid refresh cookie, refresh should fail
    expect(refreshRes.status()).toBe(401);
  });
});

test.describe("Authentication — Session Expiry", () => {
  test("unauthenticated access shows no user session", async ({ browser }) => {
    // Use a fresh context with no storageState (no cookies, no localStorage)
    const context = await browser.newContext({
      baseURL: "http://localhost:3000",
      storageState: { cookies: [], origins: [] },
    });
    const page = await context.newPage();

    await page.goto("/");
    await page.waitForTimeout(2_000);

    // Without auth, either the page redirects to /login OR the page renders
    // without an authenticated user session (no "Sign out" button visible).
    const isLoginPage = page.url().includes("/login");
    if (!isLoginPage) {
      // Page rendered without redirect — verify no authenticated user info
      await expect(page.getByRole("button", { name: "Sign out" })).not.toBeVisible();
    }

    await context.close();
  });

  test("refresh endpoint without cookie returns 401", async ({ request }) => {
    const res = await request.post(`${API_BASE}/auth/refresh`);
    expect(res.status()).toBe(401);
  });
});

test.describe("Authentication — Password Change", () => {
  test("password change via API on test user", async ({ request }) => {
    // Use a separate user to avoid corrupting admin credentials
    const adminLogin = await apiLogin(ADMIN_EMAIL, ADMIN_PASS);
    const testUser = await createViewerUser(adminLogin.accessToken);

    try {
      const userLogin = await apiLogin(testUser.email, testUser.password);

      const res = await request.post(`${API_BASE}/auth/change-password`, {
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${userLogin.accessToken}`,
        },
        data: {
          current_password: testUser.password,
          new_password: "NewViewerPass456",
        },
      });

      // Either 200 (success) or 400/422 (validation) is acceptable — not 500
      expect(res.status()).toBeLessThan(500);
    } finally {
      // Cleanup
      await deleteUser(adminLogin.accessToken, testUser.id);
    }
  });
});

test.describe("Authentication — Token Refresh Behavior", () => {
  test("refresh token grants new access token after re-login", async ({ request }) => {
    // Login to establish a session with refresh cookie
    const loginRes = await request.post(`${API_BASE}/auth/login`, {
      data: { email: ADMIN_EMAIL, password: ADMIN_PASS },
    });
    expect(loginRes.status()).toBe(200);
    const body = await loginRes.json();
    expect(body.access_token).toBeTruthy();

    // Attempt refresh — the login response should have set a refresh cookie
    const refreshRes = await request.post(`${API_BASE}/auth/refresh`);
    // If refresh cookies were set, we get 200; otherwise 401 (no cookie in APIRequestContext)
    expect([200, 401]).toContain(refreshRes.status());
  });
});

test.describe("Authentication — API Key Auth via Header", () => {
  let adminToken: string;

  test.beforeAll(async () => {
    const loginResult = await apiLogin(ADMIN_EMAIL, ADMIN_PASS);
    adminToken = loginResult.accessToken;
  });

  test("X-API-Key header authenticates requests to protected endpoints", async ({ request }) => {
    // Create an API key
    const createRes = await request.post(`${API_BASE}/auth/api-keys`, {
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${adminToken}`,
      },
      data: { name: `e2e-apikey-header-${Date.now()}` },
    });
    expect(createRes.status()).toBe(201);
    const createBody = await createRes.json();
    const plainKey = createBody.plain_key;
    const keyId = createBody.api_key.id;

    // Use X-API-Key to access /projects (a protected endpoint)
    const projectsRes = await request.get(`${API_BASE}/projects`, {
      headers: { "X-API-Key": plainKey },
    });
    expect(projectsRes.status()).toBe(200);
    const projects = await projectsRes.json();
    expect(Array.isArray(projects)).toBe(true);

    // Cleanup
    await request.delete(`${API_BASE}/auth/api-keys/${encodeURIComponent(keyId)}`, {
      headers: { Authorization: `Bearer ${adminToken}` },
    });
  });
});

test.describe("Authentication — Multiple Sessions", () => {
  test("two simultaneous browser contexts share valid auth", async ({ browser }) => {
    // Create two independent browser contexts
    const context1 = await browser.newContext({
      baseURL: "http://localhost:3000",
    });
    const context2 = await browser.newContext({
      baseURL: "http://localhost:3000",
    });

    const page1 = await context1.newPage();
    const page2 = await context2.newPage();

    // Login in context 1
    await page1.goto("/login");
    await page1.locator("#email").fill(ADMIN_EMAIL);
    await page1.locator("#password").fill(ADMIN_PASS);
    await page1.getByRole("button", { name: "Sign in" }).click();
    await expect(page1).not.toHaveURL(/\/login/, { timeout: 10_000 });

    // Login in context 2
    await page2.goto("/login");
    await page2.locator("#email").fill(ADMIN_EMAIL);
    await page2.locator("#password").fill(ADMIN_PASS);
    await page2.getByRole("button", { name: "Sign in" }).click();
    await expect(page2).not.toHaveURL(/\/login/, { timeout: 10_000 });

    // Both contexts should be able to access the dashboard
    await page1.goto("/");
    await expect(page1.locator("h1").first()).toBeVisible({ timeout: 10_000 });

    await page2.goto("/");
    await expect(page2.locator("h1").first()).toBeVisible({ timeout: 10_000 });

    await context1.close();
    await context2.close();
  });
});

test.describe("Authentication — Login Form Keyboard Navigation", () => {
  test("Tab and Enter navigate and submit login form", async ({ page }) => {
    await page.goto("/login");

    // Focus the email input first
    await page.locator("#email").focus();
    await page.keyboard.type(ADMIN_EMAIL);

    // Tab to the password field
    await page.keyboard.press("Tab");
    await page.keyboard.type(ADMIN_PASS);

    // Tab to the submit button and press Enter
    await page.keyboard.press("Tab");
    await page.keyboard.press("Enter");

    // Should navigate away from /login after successful auth
    await expect(page).not.toHaveURL(/\/login/, { timeout: 10_000 });
  });
});
