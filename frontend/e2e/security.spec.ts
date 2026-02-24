import { expect, test } from "@playwright/test";

const API_BASE = "http://localhost:8080/api/v1";
const BACKEND_BASE = "http://localhost:8080";

// Default seed admin credentials from internal/config/config.go
const ADMIN_EMAIL = "admin@localhost";
const ADMIN_PASS = "Changeme123";

// Helper: perform login via API and return access token + user.
async function apiLogin(
  email: string,
  password: string,
): Promise<{ accessToken: string; user: { id: string; role: string; tenant_id: string } }> {
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

// Helper: create a user via admin API.
async function createUser(
  adminToken: string,
  role: "admin" | "editor" | "viewer",
  suffix: string,
): Promise<{ email: string; password: string; id: string }> {
  const email = `${role}-${suffix}-${Date.now()}@test.local`;
  const password = `${role.charAt(0).toUpperCase() + role.slice(1)}Pass1234`;
  const res = await fetch(`${API_BASE}/users`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${adminToken}`,
    },
    body: JSON.stringify({ email, name: `Test ${role}`, password, role }),
  });
  if (!res.ok) {
    const body = await res.json();
    throw new Error(`Create user failed (${res.status}): ${JSON.stringify(body)}`);
  }
  const user = await res.json();
  return { email, password, id: user.id };
}

// Helper: delete a user via admin API.
async function deleteUser(adminToken: string, userId: string): Promise<void> {
  await fetch(`${API_BASE}/users/${encodeURIComponent(userId)}`, {
    method: "DELETE",
    headers: { Authorization: `Bearer ${adminToken}` },
  });
}

// ---------------------------------------------------------------------------
// WSTG-INPV-001 / WSTG-INPV-002 — Injection Testing
// ---------------------------------------------------------------------------
test.describe("WSTG-INPV-001/002 — Injection (SQL Injection & XSS)", () => {
  let adminToken: string;

  test.beforeAll(async () => {
    const loginResult = await apiLogin(ADMIN_EMAIL, ADMIN_PASS);
    adminToken = loginResult.accessToken;
  });

  test("SQL injection in project name is rejected or safely handled", async ({ request }) => {
    const sqlPayloads = [
      "'; DROP TABLE projects; --",
      "1' OR '1'='1",
      "' UNION SELECT * FROM users --",
      "Robert'); DROP TABLE projects;--",
    ];

    for (const payload of sqlPayloads) {
      const res = await request.post(`${API_BASE}/projects`, {
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${adminToken}`,
        },
        data: {
          name: payload,
          description: "",
          repo_url: "",
          provider: "",
          config: {},
        },
      });

      // The server should either reject the input (400) or safely store it
      // without executing the SQL. Either way, no 500 server error.
      expect(res.status()).not.toBe(500);

      // If it was created, clean it up and verify the name is stored verbatim (not interpreted)
      if (res.ok()) {
        const project = await res.json();
        expect(project.name).toBe(payload);
        // Clean up
        await request.delete(`${API_BASE}/projects/${encodeURIComponent(project.id)}`, {
          headers: { Authorization: `Bearer ${adminToken}` },
        });
      }
    }
  });

  test("SQL injection in search/filter query params does not cause server error", async ({
    request,
  }) => {
    const payloads = ["' OR 1=1 --", "'; DROP TABLE projects; --", "1 UNION SELECT NULL--"];

    for (const payload of payloads) {
      // Try injection in URL query parameters
      const res = await request.get(`${API_BASE}/projects?search=${encodeURIComponent(payload)}`, {
        headers: { Authorization: `Bearer ${adminToken}` },
      });
      // Should not cause a 500 server error
      expect(res.status()).not.toBe(500);
    }
  });

  test("XSS in project name is stored as plain text, not executed", async ({ request }) => {
    const xssPayloads = [
      '<script>alert("xss")</script>',
      '<img src=x onerror=alert("xss")>',
      '"><svg/onload=alert("xss")>',
      "javascript:alert(1)",
    ];

    for (const payload of xssPayloads) {
      const res = await request.post(`${API_BASE}/projects`, {
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${adminToken}`,
        },
        data: {
          name: payload,
          description: "XSS test project",
          repo_url: "",
          provider: "",
          config: {},
        },
      });

      expect(res.status()).not.toBe(500);

      if (res.ok()) {
        const project = await res.json();
        // The payload should be stored verbatim (for API clients to handle),
        // not interpreted or stripped
        expect(project.name).toBe(payload);

        // Fetch back and verify the name is not modified
        const getRes = await request.get(`${API_BASE}/projects/${encodeURIComponent(project.id)}`, {
          headers: { Authorization: `Bearer ${adminToken}` },
        });
        if (getRes.ok()) {
          const fetched = await getRes.json();
          expect(fetched.name).toBe(payload);
        }

        // Clean up
        await request.delete(`${API_BASE}/projects/${encodeURIComponent(project.id)}`, {
          headers: { Authorization: `Bearer ${adminToken}` },
        });
      }
    }
  });

  test("XSS payload in project description does not render as HTML in browser", async ({
    page,
  }) => {
    const { accessToken } = await apiLogin(ADMIN_EMAIL, ADMIN_PASS);
    const xssPayload = '<img src=x onerror=alert("xss")>';

    // Create a project with XSS in description via API
    const createRes = await fetch(`${API_BASE}/projects`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${accessToken}`,
      },
      body: JSON.stringify({
        name: "XSS Desc Test",
        description: xssPayload,
        repo_url: "",
        provider: "",
        config: {},
      }),
    });

    if (createRes.ok) {
      const project = await createRes.json();

      // Listen for any dialog (alert) — XSS would trigger this
      let alertFired = false;
      page.on("dialog", () => {
        alertFired = true;
      });

      // Visit dashboard and project detail
      await page.goto("/");
      await page.waitForTimeout(2_000);

      // No alert should have fired
      expect(alertFired).toBe(false);

      // Clean up
      await fetch(`${API_BASE}/projects/${encodeURIComponent(project.id)}`, {
        method: "DELETE",
        headers: { Authorization: `Bearer ${accessToken}` },
      });
    }
  });
});

// ---------------------------------------------------------------------------
// WSTG-ATHN-001 — Broken Authentication (Brute Force Protection)
// ---------------------------------------------------------------------------
test.describe("WSTG-ATHN-001 — Broken Authentication (Brute Force)", () => {
  let adminToken: string;
  let testUser: { email: string; password: string; id: string } | null = null;

  test.beforeAll(async () => {
    const loginResult = await apiLogin(ADMIN_EMAIL, ADMIN_PASS);
    adminToken = loginResult.accessToken;
    testUser = await createUser(adminToken, "viewer", "bruteforce");
  });

  test.afterAll(async () => {
    if (testUser) {
      await deleteUser(adminToken, testUser.id);
    }
  });

  test("account locks after multiple failed login attempts", async ({ request }) => {
    if (!testUser) return;

    // MaxFailedAttempts is 5 (from user domain model)
    const attempts = 6;
    let lastStatus = 0;

    for (let i = 0; i < attempts; i++) {
      const res = await request.post(`${API_BASE}/auth/login`, {
        data: { email: testUser.email, password: "WrongPassword1" },
      });
      lastStatus = res.status();
    }

    // After exceeding MaxFailedAttempts, should still return 401 (lockout message)
    expect(lastStatus).toBe(401);

    // Even with correct password, login should fail due to lockout
    const correctRes = await request.post(`${API_BASE}/auth/login`, {
      data: { email: testUser.email, password: testUser.password },
    });
    expect(correctRes.status()).toBe(401);

    const body = await correctRes.json();
    // The error message should indicate temporary lock
    expect(body.error).toMatch(/locked|invalid/i);
  });

  test("credential stuffing with common passwords returns 401", async ({ request }) => {
    const commonPasswords = ["password123", "admin12345", "letmein1234", "qwerty123456"];

    for (const pw of commonPasswords) {
      const res = await request.post(`${API_BASE}/auth/login`, {
        data: { email: "admin@localhost", password: pw },
      });
      expect(res.status()).toBe(401);
    }
  });

  test("login error messages do not reveal whether email exists", async ({ request }) => {
    // Non-existent email
    const res1 = await request.post(`${API_BASE}/auth/login`, {
      data: { email: "nonexistent@example.com", password: "SomePassword1" },
    });
    const body1 = await res1.json();

    // Existing email with wrong password
    const res2 = await request.post(`${API_BASE}/auth/login`, {
      data: { email: ADMIN_EMAIL, password: "WrongPassword1" },
    });
    const body2 = await res2.json();

    // Both should return the same generic error message
    expect(body1.error).toBe(body2.error);
  });
});

// ---------------------------------------------------------------------------
// WSTG-ATHZ-001 — Broken Access Control (IDOR & Privilege Escalation)
// ---------------------------------------------------------------------------
test.describe("WSTG-ATHZ-001 — Broken Access Control (IDOR / Privilege Escalation)", () => {
  let adminToken: string;
  let viewerUser: { email: string; password: string; id: string } | null = null;
  let viewerToken: string;

  test.beforeAll(async () => {
    const loginResult = await apiLogin(ADMIN_EMAIL, ADMIN_PASS);
    adminToken = loginResult.accessToken;
    viewerUser = await createUser(adminToken, "viewer", "idor");
    const viewerLogin = await apiLogin(viewerUser.email, viewerUser.password);
    viewerToken = viewerLogin.accessToken;
  });

  test.afterAll(async () => {
    if (viewerUser) {
      await deleteUser(adminToken, viewerUser.id);
    }
  });

  test("non-admin cannot list users (privilege escalation)", async ({ request }) => {
    const res = await request.get(`${API_BASE}/users`, {
      headers: { Authorization: `Bearer ${viewerToken}` },
    });
    expect(res.status()).toBe(403);
  });

  test("non-admin cannot create users (privilege escalation)", async ({ request }) => {
    const res = await request.post(`${API_BASE}/users`, {
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${viewerToken}`,
      },
      data: {
        email: "hacker@evil.com",
        name: "Hacker",
        password: "HackerPass123",
        role: "admin",
      },
    });
    expect(res.status()).toBe(403);
  });

  test("non-admin cannot delete users (privilege escalation)", async ({ request }) => {
    if (!viewerUser) return;

    const res = await request.delete(`${API_BASE}/users/${encodeURIComponent(viewerUser.id)}`, {
      headers: { Authorization: `Bearer ${viewerToken}` },
    });
    expect(res.status()).toBe(403);
  });

  test("non-admin cannot access tenants endpoint", async ({ request }) => {
    const res = await request.get(`${API_BASE}/tenants`, {
      headers: { Authorization: `Bearer ${viewerToken}` },
    });
    expect(res.status()).toBe(403);
  });

  test("non-admin cannot create tenants", async ({ request }) => {
    const res = await request.post(`${API_BASE}/tenants`, {
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${viewerToken}`,
      },
      data: { name: "Evil Tenant" },
    });
    expect(res.status()).toBe(403);
  });

  test("IDOR: accessing project with fabricated ID returns 404, not another tenant's data", async ({
    request,
  }) => {
    const fakeId = "00000000-aaaa-bbbb-cccc-000000000000";
    const res = await request.get(`${API_BASE}/projects/${fakeId}`, {
      headers: { Authorization: `Bearer ${viewerToken}` },
    });
    // Should be 404 (not found), not 200 (data leak)
    expect([404, 400]).toContain(res.status());
  });

  test("IDOR: accessing agent with fabricated ID returns 404", async ({ request }) => {
    const fakeId = "00000000-aaaa-bbbb-cccc-111111111111";
    const res = await request.get(`${API_BASE}/agents/${fakeId}`, {
      headers: { Authorization: `Bearer ${viewerToken}` },
    });
    expect([404, 400]).toContain(res.status());
  });
});

// ---------------------------------------------------------------------------
// WSTG-CONF-002 — CORS Configuration
// ---------------------------------------------------------------------------
test.describe("WSTG-CONF-002 — CORS Headers", () => {
  test("CORS allows configured origin", async ({ request }) => {
    const res = await request.fetch(`${API_BASE}/projects`, {
      method: "OPTIONS",
      headers: {
        Origin: "http://localhost:3000",
        "Access-Control-Request-Method": "GET",
        "Access-Control-Request-Headers": "Authorization",
      },
    });

    const allowOrigin = res.headers()["access-control-allow-origin"];
    expect(allowOrigin).toBeTruthy();
    // The configured origin should be allowed
    expect(allowOrigin).toBe("http://localhost:3000");
  });

  test("CORS does not use wildcard (*) when credentials are allowed", async ({ request }) => {
    const res = await request.fetch(`${API_BASE}/projects`, {
      method: "OPTIONS",
      headers: {
        Origin: "http://localhost:3000",
        "Access-Control-Request-Method": "GET",
      },
    });

    const allowOrigin = res.headers()["access-control-allow-origin"];
    const allowCredentials = res.headers()["access-control-allow-credentials"];

    // If credentials are allowed, origin must not be *
    if (allowCredentials === "true") {
      expect(allowOrigin).not.toBe("*");
    }
  });

  test("CORS restricts disallowed origins", async ({ request }) => {
    const res = await request.fetch(`${API_BASE}/projects`, {
      method: "OPTIONS",
      headers: {
        Origin: "http://evil.attacker.com",
        "Access-Control-Request-Method": "GET",
      },
    });

    const allowOrigin = res.headers()["access-control-allow-origin"];
    // Should either not be present or not reflect the attacker origin
    // Note: the current implementation uses a static configured origin,
    // so it will always reflect that static origin (not the request origin).
    if (allowOrigin) {
      expect(allowOrigin).not.toBe("http://evil.attacker.com");
    }
  });
});

// ---------------------------------------------------------------------------
// WSTG-CONF-005 — Security Headers
// ---------------------------------------------------------------------------
test.describe("WSTG-CONF-005 — HTTP Security Headers", () => {
  test("X-Content-Type-Options is set to nosniff", async ({ request }) => {
    const res = await request.get(`${BACKEND_BASE}/health`);
    expect(res.headers()["x-content-type-options"]).toBe("nosniff");
  });

  test("X-Frame-Options is set to DENY", async ({ request }) => {
    const res = await request.get(`${BACKEND_BASE}/health`);
    expect(res.headers()["x-frame-options"]).toBe("DENY");
  });

  test("Content-Security-Policy is present and restrictive", async ({ request }) => {
    const res = await request.get(`${BACKEND_BASE}/health`);
    const csp = res.headers()["content-security-policy"];
    expect(csp).toBeTruthy();
    expect(csp).toContain("default-src 'self'");
    expect(csp).toContain("frame-ancestors 'none'");
  });

  test("Referrer-Policy is set", async ({ request }) => {
    const res = await request.get(`${BACKEND_BASE}/health`);
    const policy = res.headers()["referrer-policy"];
    expect(policy).toBeTruthy();
    expect(policy).toBe("strict-origin-when-cross-origin");
  });

  test("Permissions-Policy restricts sensitive APIs", async ({ request }) => {
    const res = await request.get(`${BACKEND_BASE}/health`);
    const policy = res.headers()["permissions-policy"];
    expect(policy).toBeTruthy();
    expect(policy).toContain("camera=()");
    expect(policy).toContain("microphone=()");
    expect(policy).toContain("geolocation=()");
  });

  test("X-XSS-Protection is set to 0 (modern CSP replaces it)", async ({ request }) => {
    const res = await request.get(`${BACKEND_BASE}/health`);
    // X-XSS-Protection: 0 is recommended when CSP is in place
    // (the old 1; mode=block can introduce vulnerabilities)
    expect(res.headers()["x-xss-protection"]).toBe("0");
  });

  test("Server header does not leak implementation details", async ({ request }) => {
    const res = await request.get(`${BACKEND_BASE}/health`);
    const server = res.headers()["server"];
    // Server header should either be absent or not reveal specific versions
    if (server) {
      expect(server).not.toMatch(/Go|golang|chi|Apache|nginx/i);
    }
  });
});

// ---------------------------------------------------------------------------
// WSTG-SESS-001 — CSRF Protection
// ---------------------------------------------------------------------------
test.describe("WSTG-SESS-001 — CSRF Protection", () => {
  test("refresh token cookie has SameSite=Strict and HttpOnly attributes", async ({ request }) => {
    // Login to get the refresh cookie
    const loginRes = await request.post(`${API_BASE}/auth/login`, {
      data: { email: ADMIN_EMAIL, password: ADMIN_PASS },
    });
    expect(loginRes.status()).toBe(200);

    const setCookieHeaders = loginRes
      .headersArray()
      .filter((h) => h.name.toLowerCase() === "set-cookie");
    const refreshCookie = setCookieHeaders.find((h) => h.value.includes("codeforge_refresh"));

    expect(refreshCookie).toBeTruthy();
    if (refreshCookie) {
      const value = refreshCookie.value.toLowerCase();
      expect(value).toContain("httponly");
      expect(value).toContain("samesite=strict");
      // Path should be restricted to auth endpoints
      expect(value).toContain("path=/api/v1/auth");
    }
  });

  test("state-changing POST without valid auth is rejected", async ({ request }) => {
    // Try to create a project without any authentication (simulating CSRF from another origin)
    const res = await request.post(`${API_BASE}/projects`, {
      data: {
        name: "CSRF Test Project",
        description: "Should not be created",
        repo_url: "",
        provider: "",
        config: {},
      },
    });
    expect(res.status()).toBe(401);
  });

  test("CORS preflight for state-changing methods returns proper headers", async ({ request }) => {
    const res = await request.fetch(`${API_BASE}/projects`, {
      method: "OPTIONS",
      headers: {
        Origin: "http://localhost:3000",
        "Access-Control-Request-Method": "POST",
        "Access-Control-Request-Headers": "Content-Type, Authorization",
      },
    });

    // Preflight should return 204 with allowed methods
    expect([200, 204]).toContain(res.status());
    const allowMethods = res.headers()["access-control-allow-methods"];
    expect(allowMethods).toBeTruthy();
    expect(allowMethods).toContain("POST");
  });
});

// ---------------------------------------------------------------------------
// WSTG-ATHZ-003 — Path Traversal
// ---------------------------------------------------------------------------
test.describe("WSTG-ATHZ-003 — Path Traversal", () => {
  let adminToken: string;

  test.beforeAll(async () => {
    const loginResult = await apiLogin(ADMIN_EMAIL, ADMIN_PASS);
    adminToken = loginResult.accessToken;
  });

  test("path traversal in project ID is rejected", async ({ request }) => {
    const traversalPayloads = [
      "../../../etc/passwd",
      "..%2F..%2F..%2Fetc%2Fpasswd",
      "....//....//....//etc/passwd",
      "%2e%2e%2f%2e%2e%2f%2e%2e%2fetc%2fpasswd",
    ];

    for (const payload of traversalPayloads) {
      const res = await request.get(`${API_BASE}/projects/${encodeURIComponent(payload)}`, {
        headers: { Authorization: `Bearer ${adminToken}` },
      });
      // Should return 400 or 404, never 200 with file contents or 500
      expect(res.status()).not.toBe(500);
      expect(res.status()).not.toBe(200);
    }
  });

  test("path traversal in workspace paths does not expose system files", async ({ request }) => {
    const traversalPayloads = [
      "../../../etc/passwd",
      "/etc/passwd",
      "..\\..\\..\\windows\\system32\\config\\sam",
    ];

    for (const payload of traversalPayloads) {
      // Try path traversal in detect-stack endpoint which takes a file path
      const res = await request.post(`${API_BASE}/detect-stack`, {
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${adminToken}`,
        },
        data: { path: payload },
      });

      // Should not return 200 with system file contents
      if (res.ok()) {
        const body = await res.json();
        // Body should not contain sensitive system file markers
        const bodyStr = JSON.stringify(body);
        expect(bodyStr).not.toContain("root:x:0:0");
        expect(bodyStr).not.toContain("[boot loader]");
      }
      expect(res.status()).not.toBe(500);
    }
  });

  test("path traversal in agent/task IDs is rejected", async ({ request }) => {
    const payloads = ["../admin", "..%2Fadmin", "../../secret"];

    for (const payload of payloads) {
      const res = await request.get(`${API_BASE}/agents/${encodeURIComponent(payload)}`, {
        headers: { Authorization: `Bearer ${adminToken}` },
      });
      expect(res.status()).not.toBe(500);
      expect(res.status()).not.toBe(200);
    }
  });
});

// ---------------------------------------------------------------------------
// Additional: Password Policy & Token Security
// ---------------------------------------------------------------------------
test.describe("Password Policy & Token Security", () => {
  test("weak passwords are rejected during user creation", async ({ request }) => {
    const { accessToken } = await apiLogin(ADMIN_EMAIL, ADMIN_PASS);

    const weakPasswords = [
      "short", // too short
      "alllowercase1", // no uppercase
      "ALLUPPERCASE1", // no lowercase
      "NoDigitsHere", // no digit
      "12345678", // no letters
    ];

    for (const pw of weakPasswords) {
      const res = await request.post(`${API_BASE}/users`, {
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${accessToken}`,
        },
        data: {
          email: `weak-${Date.now()}@test.local`,
          name: "Weak User",
          password: pw,
          role: "viewer",
        },
      });
      // Should be rejected with 400
      expect(res.status()).toBe(400);
    }
  });

  test("password hash is never exposed in API responses", async ({ request }) => {
    const { accessToken } = await apiLogin(ADMIN_EMAIL, ADMIN_PASS);

    // Check /auth/me
    const meRes = await request.get(`${API_BASE}/auth/me`, {
      headers: { Authorization: `Bearer ${accessToken}` },
    });
    const meBody = await meRes.json();
    expect(meBody.password_hash).toBeUndefined();
    expect(meBody.passwordHash).toBeUndefined();

    // Check /users (admin list)
    const usersRes = await request.get(`${API_BASE}/users`, {
      headers: { Authorization: `Bearer ${accessToken}` },
    });
    if (usersRes.ok()) {
      const users = await usersRes.json();
      for (const u of users) {
        expect(u.password_hash).toBeUndefined();
        expect(u.passwordHash).toBeUndefined();
      }
    }
  });

  test("login response does not leak password hash", async ({ request }) => {
    const res = await request.post(`${API_BASE}/auth/login`, {
      data: { email: ADMIN_EMAIL, password: ADMIN_PASS },
    });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(body.user.password_hash).toBeUndefined();
    expect(body.user.passwordHash).toBeUndefined();
    // Should have access_token
    expect(body.access_token).toBeTruthy();
  });
});
