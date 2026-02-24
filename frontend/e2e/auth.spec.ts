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
  test("unauthenticated access to protected page redirects to /login", async ({ page }) => {
    // Clear all cookies/storage to simulate no session
    await page.context().clearCookies();

    await page.goto("/");

    // The RouteGuard should redirect to /login
    await expect(page).toHaveURL(/\/login/, { timeout: 10_000 });
  });

  test("refresh endpoint without cookie returns 401", async ({ request }) => {
    const res = await request.post(`${API_BASE}/auth/refresh`);
    expect(res.status()).toBe(401);
  });
});
