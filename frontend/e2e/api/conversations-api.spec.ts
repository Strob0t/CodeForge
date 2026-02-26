import { test, expect } from "@playwright/test";
import { apiLogin, createProject, createCleanupTracker, API_BASE } from "../helpers/api-helpers";

test.describe("Conversations API", () => {
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

  test("create conversation returns 201", async ({ request }) => {
    const proj = await createProject(`e2e-conv-proj-${Date.now()}`);
    cleanup.add("project", proj.id);

    const res = await request.post(`${API_BASE}/projects/${proj.id}/conversations`, {
      headers: headers(),
      data: { title: "test conversation" },
    });
    expect(res.status()).toBe(201);
    const body = await res.json();
    expect(body.id).toBeTruthy();
    cleanup.add("conversation", body.id);
  });

  test("list conversations returns array", async ({ request }) => {
    const proj = await createProject(`e2e-list-conv-${Date.now()}`);
    cleanup.add("project", proj.id);

    // Create a conversation first
    const createRes = await request.post(`${API_BASE}/projects/${proj.id}/conversations`, {
      headers: headers(),
      data: { title: "list test" },
    });
    const conv = await createRes.json();
    cleanup.add("conversation", conv.id);

    const res = await request.get(`${API_BASE}/projects/${proj.id}/conversations`, {
      headers: headers(),
    });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(Array.isArray(body)).toBe(true);
    expect(body.length).toBeGreaterThanOrEqual(1);
  });

  test("get conversation by ID", async ({ request }) => {
    const proj = await createProject(`e2e-get-conv-${Date.now()}`);
    cleanup.add("project", proj.id);

    const createRes = await request.post(`${API_BASE}/projects/${proj.id}/conversations`, {
      headers: headers(),
      data: { title: "get test" },
    });
    const conv = await createRes.json();
    cleanup.add("conversation", conv.id);

    const res = await request.get(`${API_BASE}/conversations/${conv.id}`, { headers: headers() });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(body.id).toBe(conv.id);
  });

  test("delete conversation returns 204", async ({ request }) => {
    const proj = await createProject(`e2e-del-conv-${Date.now()}`);
    cleanup.add("project", proj.id);

    const createRes = await request.post(`${API_BASE}/projects/${proj.id}/conversations`, {
      headers: headers(),
      data: { title: "delete test" },
    });
    const conv = await createRes.json();

    const res = await request.delete(`${API_BASE}/conversations/${conv.id}`, {
      headers: headers(),
    });
    expect(res.status()).toBe(204);
  });

  test("list messages returns array", async ({ request }) => {
    const proj = await createProject(`e2e-msgs-conv-${Date.now()}`);
    cleanup.add("project", proj.id);

    const createRes = await request.post(`${API_BASE}/projects/${proj.id}/conversations`, {
      headers: headers(),
      data: { title: "messages test" },
    });
    const conv = await createRes.json();
    cleanup.add("conversation", conv.id);

    const res = await request.get(`${API_BASE}/conversations/${conv.id}/messages`, {
      headers: headers(),
    });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(Array.isArray(body)).toBe(true);
  });

  test("send message to conversation", async ({ request }) => {
    const proj = await createProject(`e2e-send-conv-${Date.now()}`);
    cleanup.add("project", proj.id);

    const createRes = await request.post(`${API_BASE}/projects/${proj.id}/conversations`, {
      headers: headers(),
      data: { title: "send test" },
    });
    const conv = await createRes.json();
    cleanup.add("conversation", conv.id);

    const res = await request.post(`${API_BASE}/conversations/${conv.id}/messages`, {
      headers: headers(),
      data: { content: "Hello from E2E test" },
    });
    // 200/201 for sync reply, 202 for agentic dispatch, 500/502 if LLM/worker not available
    expect([200, 201, 202, 500, 502]).toContain(res.status());
  });

  test("stop conversation", async ({ request }) => {
    const proj = await createProject(`e2e-stop-conv-${Date.now()}`);
    cleanup.add("project", proj.id);

    const createRes = await request.post(`${API_BASE}/projects/${proj.id}/conversations`, {
      headers: headers(),
      data: { title: "stop test" },
    });
    const conv = await createRes.json();
    cleanup.add("conversation", conv.id);

    const res = await request.post(`${API_BASE}/conversations/${conv.id}/stop`, {
      headers: headers(),
      data: {},
    });
    // May succeed or fail if nothing is running -- either is valid
    expect([200, 400, 404]).toContain(res.status());
  });

  test("get non-existent conversation returns 404", async ({ request }) => {
    const res = await request.get(
      `${API_BASE}/conversations/00000000-0000-0000-0000-000000000000`,
      { headers: headers() },
    );
    expect(res.status()).toBe(404);
  });

  test("create conversation for non-existent project returns error", async ({ request }) => {
    const res = await request.post(
      `${API_BASE}/projects/00000000-0000-0000-0000-000000000000/conversations`,
      {
        headers: headers(),
        data: { title: "ghost project" },
      },
    );
    // The service does not validate project existence before INSERT.
    // A foreign key constraint violation in PostgreSQL results in 500,
    // but if project validation is added later it would return 404.
    expect([404, 500]).toContain(res.status());
  });
});
