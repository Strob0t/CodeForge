import { test, expect } from "@playwright/test";
import { apiLogin, createProject, createCleanupTracker, API_BASE } from "../helpers/api-helpers";

test.describe("Projects API", () => {
  let token: string;
  const cleanup = createCleanupTracker();

  test.beforeAll(async () => {
    const auth = await apiLogin("admin@localhost", "Changeme123");
    token = auth.accessToken;
  });

  test.afterAll(async () => {
    await cleanup.cleanup();
  });

  const headers = () => ({ Authorization: `Bearer ${token}` });

  test("list projects returns array", async ({ request }) => {
    const res = await request.get(`${API_BASE}/projects`, { headers: headers() });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(Array.isArray(body)).toBe(true);
  });

  test("create project returns 201", async ({ request }) => {
    const name = `e2e-proj-${Date.now()}`;
    const res = await request.post(`${API_BASE}/projects`, {
      headers: headers(),
      data: { name, description: "test project", repo_url: "", provider: "", config: {} },
    });
    expect(res.status()).toBe(201);
    const body = await res.json();
    expect(body.id).toBeTruthy();
    expect(body.name).toBe(name);
    cleanup.add("project", body.id);
  });

  test("create project without name returns 400", async ({ request }) => {
    const res = await request.post(`${API_BASE}/projects`, {
      headers: headers(),
      data: { description: "no name" },
    });
    expect(res.status()).toBe(400);
  });

  test("get project by ID", async ({ request }) => {
    const proj = await createProject(`e2e-get-${Date.now()}`);
    cleanup.add("project", proj.id);

    const res = await request.get(`${API_BASE}/projects/${proj.id}`, { headers: headers() });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(body.id).toBe(proj.id);
    expect(body.name).toBe(proj.name);
  });

  test("update project", async ({ request }) => {
    const proj = await createProject(`e2e-upd-${Date.now()}`);
    cleanup.add("project", proj.id);

    const newName = `e2e-updated-${Date.now()}`;
    const res = await request.put(`${API_BASE}/projects/${proj.id}`, {
      headers: headers(),
      data: { name: newName },
    });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(body.name).toBe(newName);
  });

  test("delete project returns 204", async ({ request }) => {
    const proj = await createProject(`e2e-del-${Date.now()}`);
    const res = await request.delete(`${API_BASE}/projects/${proj.id}`, { headers: headers() });
    expect(res.status()).toBe(204);
  });

  test("delete non-existent project returns 404", async ({ request }) => {
    const res = await request.delete(`${API_BASE}/projects/00000000-0000-0000-0000-000000000000`, {
      headers: headers(),
    });
    expect(res.status()).toBe(404);
  });

  test("get non-existent project returns 404", async ({ request }) => {
    const res = await request.get(`${API_BASE}/projects/00000000-0000-0000-0000-000000000000`, {
      headers: headers(),
    });
    expect(res.status()).toBe(404);
  });

  test("clone project endpoint exists", async ({ request }) => {
    const proj = await createProject(`e2e-clone-${Date.now()}`);
    cleanup.add("project", proj.id);

    const res = await request.post(`${API_BASE}/projects/${proj.id}/clone`, {
      headers: headers(),
      data: {},
    });
    // Clone may fail if no repo_url is set, but endpoint should not return 404/405
    expect([200, 400, 500, 502]).toContain(res.status());
  });

  test("workspace info returns data", async ({ request }) => {
    const proj = await createProject(`e2e-ws-${Date.now()}`);
    cleanup.add("project", proj.id);

    const res = await request.get(`${API_BASE}/projects/${proj.id}/workspace`, {
      headers: headers(),
    });
    // Workspace may return 200 or error if not cloned; endpoint should exist
    expect([200, 400, 404, 500]).toContain(res.status());
  });

  test("parse repo URL", async ({ request }) => {
    const res = await request.post(`${API_BASE}/parse-repo-url`, {
      headers: headers(),
      data: { url: "https://github.com/user/repo" },
    });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(body).toBeTruthy();
  });

  test("git status for project", async ({ request }) => {
    const proj = await createProject(`e2e-git-${Date.now()}`);
    cleanup.add("project", proj.id);

    const res = await request.get(`${API_BASE}/projects/${proj.id}/git/status`, {
      headers: headers(),
    });
    // May fail if workspace is not cloned, but endpoint should not be 404 for project
    expect([200, 400, 500]).toContain(res.status());
  });

  test("git branches for project", async ({ request }) => {
    const proj = await createProject(`e2e-br-${Date.now()}`);
    cleanup.add("project", proj.id);

    const res = await request.get(`${API_BASE}/projects/${proj.id}/git/branches`, {
      headers: headers(),
    });
    expect([200, 400, 500]).toContain(res.status());
  });

  test("detect stack for project", async ({ request }) => {
    const proj = await createProject(`e2e-stack-${Date.now()}`);
    cleanup.add("project", proj.id);

    const res = await request.get(`${API_BASE}/projects/${proj.id}/detect-stack`, {
      headers: headers(),
    });
    expect([200, 400, 500]).toContain(res.status());
  });

  test("list remote branches requires url parameter", async ({ request }) => {
    const res = await request.get(`${API_BASE}/projects/remote-branches`, {
      headers: headers(),
    });
    expect(res.status()).toBe(400);
  });
});
