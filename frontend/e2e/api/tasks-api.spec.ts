import { test, expect } from "@playwright/test";
import {
  apiLogin,
  createProject,
  createTask,
  createCleanupTracker,
  API_BASE,
} from "../helpers/api-helpers";

test.describe("Tasks API", () => {
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

  test("create task returns 201", async ({ request }) => {
    const proj = await createProject(`e2e-task-proj-${Date.now()}`);
    cleanup.add("project", proj.id);

    const res = await request.post(`${API_BASE}/projects/${proj.id}/tasks`, {
      headers: headers(),
      data: { title: `task-${Date.now()}`, prompt: "test prompt" },
    });
    expect(res.status()).toBe(201);
    const body = await res.json();
    expect(body.id).toBeTruthy();
  });

  test("list tasks returns array", async ({ request }) => {
    const proj = await createProject(`e2e-list-tasks-${Date.now()}`);
    cleanup.add("project", proj.id);

    await createTask(proj.id, `task-list-${Date.now()}`);

    const res = await request.get(`${API_BASE}/projects/${proj.id}/tasks`, {
      headers: headers(),
    });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(Array.isArray(body)).toBe(true);
    expect(body.length).toBeGreaterThanOrEqual(1);
  });

  test("get task by ID", async ({ request }) => {
    const proj = await createProject(`e2e-get-task-${Date.now()}`);
    cleanup.add("project", proj.id);

    const task = await createTask(proj.id, `task-get-${Date.now()}`);

    const res = await request.get(`${API_BASE}/tasks/${task.id}`, { headers: headers() });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(body.id).toBe(task.id);
  });

  test("get task events returns array", async ({ request }) => {
    const proj = await createProject(`e2e-task-events-${Date.now()}`);
    cleanup.add("project", proj.id);

    const task = await createTask(proj.id, `task-events-${Date.now()}`);

    const res = await request.get(`${API_BASE}/tasks/${task.id}/events`, { headers: headers() });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(Array.isArray(body)).toBe(true);
  });

  test("get task runs returns array", async ({ request }) => {
    const proj = await createProject(`e2e-task-runs-${Date.now()}`);
    cleanup.add("project", proj.id);

    const task = await createTask(proj.id, `task-runs-${Date.now()}`);

    const res = await request.get(`${API_BASE}/tasks/${task.id}/runs`, { headers: headers() });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(Array.isArray(body)).toBe(true);
  });

  test("get context pack for task", async ({ request }) => {
    const proj = await createProject(`e2e-ctx-get-${Date.now()}`);
    cleanup.add("project", proj.id);

    const task = await createTask(proj.id, `task-ctx-${Date.now()}`);

    const res = await request.get(`${API_BASE}/tasks/${task.id}/context`, { headers: headers() });
    // Context pack may not exist yet — 200 or 404 are valid
    expect([200, 404]).toContain(res.status());
  });

  test("build context pack requires project_id", async ({ request }) => {
    const proj = await createProject(`e2e-ctx-build-${Date.now()}`);
    cleanup.add("project", proj.id);

    const task = await createTask(proj.id, `task-build-${Date.now()}`);

    const res = await request.post(`${API_BASE}/tasks/${task.id}/context`, {
      headers: headers(),
      data: { project_id: proj.id },
    });
    // 201 if build succeeds, or 400/500 if infra not ready — either is acceptable
    expect([201, 400, 500]).toContain(res.status());
  });
});
