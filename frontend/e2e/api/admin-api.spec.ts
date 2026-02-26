import { test, expect } from "@playwright/test";
import { apiLogin, createUser, createCleanupTracker, API_BASE } from "../helpers/api-helpers";

test.describe("Admin API", () => {
  let adminToken: string;
  const cleanup = createCleanupTracker();

  test.beforeAll(async () => {
    const auth = await apiLogin("admin@localhost", "Changeme123");
    adminToken = auth.accessToken;
  });

  test.afterAll(async () => {
    await cleanup.cleanup();
  });

  const adminHeaders = () => ({ Authorization: `Bearer ${adminToken}` });

  test("list tenants returns 200 for admin", async ({ request }) => {
    const res = await request.get(`${API_BASE}/tenants`, { headers: adminHeaders() });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(Array.isArray(body)).toBe(true);
  });

  test("create tenant", async ({ request }) => {
    const slug = `e2e-tenant-${Date.now()}`;
    const res = await request.post(`${API_BASE}/tenants`, {
      headers: adminHeaders(),
      data: { name: `E2E Tenant ${Date.now()}`, slug },
    });
    expect(res.status()).toBe(201);
    const body = await res.json();
    expect(body.id).toBeTruthy();
  });

  test("list users returns 200 for admin", async ({ request }) => {
    const res = await request.get(`${API_BASE}/users`, { headers: adminHeaders() });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(Array.isArray(body)).toBe(true);
  });

  test("create user", async ({ request }) => {
    const email = `e2e-user-${Date.now()}@test.local`;
    const res = await request.post(`${API_BASE}/users`, {
      headers: adminHeaders(),
      data: {
        email,
        name: "E2E Test User",
        password: "TestPass123!",
        role: "viewer",
      },
    });
    expect(res.status()).toBe(201);
    const body = await res.json();
    expect(body.id).toBeTruthy();
    cleanup.add("user", body.id);
  });

  test("update user", async ({ request }) => {
    const user = await createUser(`e2e-upd-user-${Date.now()}@test.local`, "UpdPass123!");
    cleanup.add("user", user.id);

    const res = await request.put(`${API_BASE}/users/${user.id}`, {
      headers: adminHeaders(),
      data: { name: "Updated Name" },
    });
    expect(res.status()).toBe(200);
  });

  test("delete user returns 204", async ({ request }) => {
    const user = await createUser(`e2e-del-user-${Date.now()}@test.local`, "DelPass123!");

    const res = await request.delete(`${API_BASE}/users/${user.id}`, {
      headers: adminHeaders(),
    });
    expect(res.status()).toBe(204);
  });

  test("viewer cannot access tenants", async ({ request }) => {
    const viewerEmail = `e2e-viewer-tenant-${Date.now()}@test.local`;
    const viewerPass = "ViewerPass123!";
    const viewer = await createUser(viewerEmail, viewerPass, "viewer");
    cleanup.add("user", viewer.id);

    const viewerAuth = await apiLogin(viewerEmail, viewerPass);
    const viewerHeaders = { Authorization: `Bearer ${viewerAuth.accessToken}` };

    const res = await request.get(`${API_BASE}/tenants`, { headers: viewerHeaders });
    expect(res.status()).toBe(403);
  });

  test("viewer cannot access users", async ({ request }) => {
    const viewerEmail = `e2e-viewer-users-${Date.now()}@test.local`;
    const viewerPass = "ViewerPass456!";
    const viewer = await createUser(viewerEmail, viewerPass, "viewer");
    cleanup.add("user", viewer.id);

    const viewerAuth = await apiLogin(viewerEmail, viewerPass);
    const viewerHeaders = { Authorization: `Bearer ${viewerAuth.accessToken}` };

    const res = await request.get(`${API_BASE}/users`, { headers: viewerHeaders });
    expect(res.status()).toBe(403);
  });
});
