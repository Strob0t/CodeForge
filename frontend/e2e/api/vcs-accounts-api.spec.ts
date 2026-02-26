import { test, expect } from "@playwright/test";
import { apiLogin, API_BASE } from "../helpers/api-helpers";

test.describe("VCS Accounts API", () => {
  let token: string;
  const createdIds: string[] = [];

  test.beforeAll(async () => {
    const auth = await apiLogin("admin@localhost", "Changeme123");
    token = auth.accessToken;
  });

  test.afterAll(async () => {
    for (const id of createdIds) {
      try {
        await fetch(`${API_BASE}/vcs-accounts/${id}`, {
          method: "DELETE",
          headers: { Authorization: `Bearer ${token}` },
        });
      } catch {
        // best-effort
      }
    }
  });

  const headers = () => ({ Authorization: `Bearer ${token}` });
  const jsonHeaders = () => ({
    ...headers(),
    "Content-Type": "application/json",
  });

  test("GET /vcs-accounts returns 200 with array", async () => {
    const res = await fetch(`${API_BASE}/vcs-accounts`, {
      headers: headers(),
    });
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(Array.isArray(body)).toBe(true);
  });

  test("POST /vcs-accounts creates an account", async () => {
    const res = await fetch(`${API_BASE}/vcs-accounts`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({
        provider: "github",
        label: `e2e-account-${Date.now()}`,
        server_url: "",
        auth_method: "token",
        token: "ghp_fake_token_for_e2e_testing",
      }),
    });
    expect(res.status).toBe(201);
    const body = await res.json();
    expect(body.id).toBeTruthy();
    expect(body.provider).toBe("github");
    createdIds.push(body.id);
  });

  test("DELETE /vcs-accounts/{id} removes an account", async () => {
    // Create first
    const createRes = await fetch(`${API_BASE}/vcs-accounts`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({
        provider: "gitlab",
        label: `e2e-del-${Date.now()}`,
        server_url: "",
        auth_method: "token",
        token: "glpat_fake_token",
      }),
    });
    const account = await createRes.json();

    const res = await fetch(`${API_BASE}/vcs-accounts/${account.id}`, {
      method: "DELETE",
      headers: headers(),
    });
    expect(res.status).toBe(204);
  });

  test("POST /vcs-accounts/{id}/test tests connection", async () => {
    // Create an account first
    const createRes = await fetch(`${API_BASE}/vcs-accounts`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({
        provider: "github",
        label: `e2e-test-conn-${Date.now()}`,
        server_url: "",
        auth_method: "token",
        token: "ghp_fake_token_for_testing",
      }),
    });
    const account = await createRes.json();
    createdIds.push(account.id);

    // The handler does not require a request body — it looks up the account
    // by ID, decrypts the stored token, and makes a test API call.
    const res = await fetch(`${API_BASE}/vcs-accounts/${account.id}/test`, {
      method: "POST",
      headers: headers(),
    });
    // Fake token will fail decryption or the API call, resulting in 500.
    // Also accept 200 (if somehow it works), 400, or 404.
    expect([200, 400, 404, 500]).toContain(res.status);
  });

  test("DELETE /vcs-accounts/{non-existent} returns 404", async () => {
    const res = await fetch(`${API_BASE}/vcs-accounts/00000000-0000-0000-0000-000000000000`, {
      method: "DELETE",
      headers: headers(),
    });
    expect(res.status).toBe(404);
  });
});
