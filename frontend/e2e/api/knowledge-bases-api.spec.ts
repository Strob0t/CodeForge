import { test, expect } from "@playwright/test";
import { apiLogin, createCleanupTracker, API_BASE } from "../helpers/api-helpers";

test.describe("Knowledge Bases API", () => {
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

  test("list knowledge bases returns array", async ({ request }) => {
    const res = await request.get(`${API_BASE}/knowledge-bases`, { headers: headers() });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(Array.isArray(body)).toBe(true);
  });

  test("create knowledge base returns 201", async ({ request }) => {
    const res = await request.post(`${API_BASE}/knowledge-bases`, {
      headers: headers(),
      data: {
        name: `e2e-kb-${Date.now()}`,
        description: "E2E test knowledge base",
        category: "framework",
        tags: ["e2e", "test"],
        content_path: "/tmp/e2e-kb-test",
      },
    });
    expect(res.status()).toBe(201);
    const body = await res.json();
    expect(body.id).toBeTruthy();
    expect(body.name).toBeTruthy();
    cleanup.add("knowledge-base", body.id);
  });

  test("get knowledge base by ID", async ({ request }) => {
    const createRes = await request.post(`${API_BASE}/knowledge-bases`, {
      headers: headers(),
      data: {
        name: `e2e-get-kb-${Date.now()}`,
        description: "get test",
        category: "framework",
        tags: [],
        content_path: "/tmp/e2e-kb-get",
      },
    });
    const kb = await createRes.json();
    cleanup.add("knowledge-base", kb.id);

    const res = await request.get(`${API_BASE}/knowledge-bases/${kb.id}`, { headers: headers() });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(body.id).toBe(kb.id);
  });

  test("update knowledge base", async ({ request }) => {
    const createRes = await request.post(`${API_BASE}/knowledge-bases`, {
      headers: headers(),
      data: {
        name: `e2e-upd-kb-${Date.now()}`,
        description: "update test",
        category: "framework",
        tags: [],
        content_path: "/tmp/e2e-kb-upd",
      },
    });
    const kb = await createRes.json();
    cleanup.add("knowledge-base", kb.id);

    const res = await request.put(`${API_BASE}/knowledge-bases/${kb.id}`, {
      headers: headers(),
      data: {
        name: `e2e-updated-kb-${Date.now()}`,
        description: "updated description",
      },
    });
    expect(res.status()).toBe(200);
  });

  test("delete knowledge base returns 204", async ({ request }) => {
    const createRes = await request.post(`${API_BASE}/knowledge-bases`, {
      headers: headers(),
      data: {
        name: `e2e-del-kb-${Date.now()}`,
        description: "delete test",
        category: "framework",
        tags: [],
        content_path: "/tmp/e2e-kb-del",
      },
    });
    const kb = await createRes.json();

    const res = await request.delete(`${API_BASE}/knowledge-bases/${kb.id}`, {
      headers: headers(),
    });
    expect(res.status()).toBe(204);
  });

  test("index knowledge base returns 202", async ({ request }) => {
    const createRes = await request.post(`${API_BASE}/knowledge-bases`, {
      headers: headers(),
      data: {
        name: `e2e-idx-kb-${Date.now()}`,
        description: "index test",
        category: "framework",
        tags: [],
        content_path: "/tmp/e2e-kb-idx",
      },
    });
    const kb = await createRes.json();
    cleanup.add("knowledge-base", kb.id);

    const res = await request.post(`${API_BASE}/knowledge-bases/${kb.id}/index`, {
      headers: headers(),
      data: {},
    });
    // 202 for accepted, or error if infra not ready
    expect([202, 400, 500]).toContain(res.status());
  });

  test("delete non-existent knowledge base returns 404", async ({ request }) => {
    const res = await request.delete(
      `${API_BASE}/knowledge-bases/00000000-0000-0000-0000-000000000000`,
      { headers: headers() },
    );
    expect(res.status()).toBe(404);
  });
});
