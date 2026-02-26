import { test, expect } from "@playwright/test";
import {
  apiLogin,
  createProject,
  createAgent,
  createTask,
  createCleanupTracker,
  API_BASE,
} from "../helpers/api-helpers";

test.describe("Runs API", () => {
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

  test("start run returns 201", async ({ request }) => {
    const proj = await createProject(`e2e-run-proj-${Date.now()}`);
    cleanup.add("project", proj.id);

    const agent = await createAgent(proj.id, `agent-run-${Date.now()}`);
    cleanup.add("agent", agent.id);

    const task = await createTask(proj.id, `task-run-${Date.now()}`);

    const res = await request.post(`${API_BASE}/runs`, {
      headers: headers(),
      data: { task_id: task.id, agent_id: agent.id, project_id: proj.id },
    });
    expect(res.status()).toBe(201);
    const body = await res.json();
    expect(body.id).toBeTruthy();
  });

  test("get run by ID", async ({ request }) => {
    const proj = await createProject(`e2e-get-run-${Date.now()}`);
    cleanup.add("project", proj.id);

    const agent = await createAgent(proj.id, `agent-get-run-${Date.now()}`);
    cleanup.add("agent", agent.id);

    const task = await createTask(proj.id, `task-get-run-${Date.now()}`);

    const startRes = await request.post(`${API_BASE}/runs`, {
      headers: headers(),
      data: { task_id: task.id, agent_id: agent.id, project_id: proj.id },
    });
    const run = await startRes.json();

    const res = await request.get(`${API_BASE}/runs/${run.id}`, { headers: headers() });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(body.id).toBe(run.id);
  });

  test("cancel run", async ({ request }) => {
    const proj = await createProject(`e2e-cancel-run-${Date.now()}`);
    cleanup.add("project", proj.id);

    const agent = await createAgent(proj.id, `agent-cancel-${Date.now()}`);
    cleanup.add("agent", agent.id);

    const task = await createTask(proj.id, `task-cancel-${Date.now()}`);

    const startRes = await request.post(`${API_BASE}/runs`, {
      headers: headers(),
      data: { task_id: task.id, agent_id: agent.id, project_id: proj.id },
    });
    const run = await startRes.json();

    const res = await request.post(`${API_BASE}/runs/${run.id}/cancel`, {
      headers: headers(),
      data: {},
    });
    expect([200, 400]).toContain(res.status());
  });

  test("get run events returns array", async ({ request }) => {
    const proj = await createProject(`e2e-run-events-${Date.now()}`);
    cleanup.add("project", proj.id);

    const agent = await createAgent(proj.id, `agent-events-${Date.now()}`);
    cleanup.add("agent", agent.id);

    const task = await createTask(proj.id, `task-events-${Date.now()}`);

    const startRes = await request.post(`${API_BASE}/runs`, {
      headers: headers(),
      data: { task_id: task.id, agent_id: agent.id, project_id: proj.id },
    });
    const run = await startRes.json();

    const res = await request.get(`${API_BASE}/runs/${run.id}/events`, { headers: headers() });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(Array.isArray(body)).toBe(true);
  });

  test("get non-existent run returns 404", async ({ request }) => {
    const res = await request.get(`${API_BASE}/runs/00000000-0000-0000-0000-000000000000`, {
      headers: headers(),
    });
    expect(res.status()).toBe(404);
  });
});
