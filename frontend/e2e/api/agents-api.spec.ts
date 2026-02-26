import { test, expect } from "@playwright/test";
import {
  apiLogin,
  createProject,
  createAgent,
  createCleanupTracker,
  API_BASE,
} from "../helpers/api-helpers";

test.describe("Agents API", () => {
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

  test("create agent returns 201", async ({ request }) => {
    const proj = await createProject(`e2e-agent-proj-${Date.now()}`);
    cleanup.add("project", proj.id);

    const res = await request.post(`${API_BASE}/projects/${proj.id}/agents`, {
      headers: headers(),
      data: { name: `agent-${Date.now()}`, backend: "aider" },
    });
    expect(res.status()).toBe(201);
    const body = await res.json();
    expect(body.id).toBeTruthy();
    expect(body.name).toBeTruthy();
    cleanup.add("agent", body.id);
  });

  test("list agents returns array", async ({ request }) => {
    const proj = await createProject(`e2e-list-agents-${Date.now()}`);
    cleanup.add("project", proj.id);

    const agent = await createAgent(proj.id, `agent-list-${Date.now()}`);
    cleanup.add("agent", agent.id);

    const res = await request.get(`${API_BASE}/projects/${proj.id}/agents`, {
      headers: headers(),
    });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(Array.isArray(body)).toBe(true);
    expect(body.length).toBeGreaterThanOrEqual(1);
  });

  test("get agent by ID", async ({ request }) => {
    const proj = await createProject(`e2e-get-agent-${Date.now()}`);
    cleanup.add("project", proj.id);

    const agent = await createAgent(proj.id, `agent-get-${Date.now()}`);
    cleanup.add("agent", agent.id);

    const res = await request.get(`${API_BASE}/agents/${agent.id}`, { headers: headers() });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(body.id).toBe(agent.id);
  });

  test("delete agent returns 204", async ({ request }) => {
    const proj = await createProject(`e2e-del-agent-${Date.now()}`);
    cleanup.add("project", proj.id);

    const agent = await createAgent(proj.id, `agent-del-${Date.now()}`);
    const res = await request.delete(`${API_BASE}/agents/${agent.id}`, { headers: headers() });
    expect(res.status()).toBe(204);
  });

  test("delete non-existent agent returns 404", async ({ request }) => {
    const res = await request.delete(`${API_BASE}/agents/00000000-0000-0000-0000-000000000000`, {
      headers: headers(),
    });
    expect(res.status()).toBe(404);
  });

  test("dispatch task requires task_id", async ({ request }) => {
    const proj = await createProject(`e2e-dispatch-${Date.now()}`);
    cleanup.add("project", proj.id);

    const agent = await createAgent(proj.id, `agent-dispatch-${Date.now()}`);
    cleanup.add("agent", agent.id);

    const res = await request.post(`${API_BASE}/agents/${agent.id}/dispatch`, {
      headers: headers(),
      data: { task_id: "" },
    });
    expect(res.status()).toBe(400);
  });

  test("stop agent requires task_id", async ({ request }) => {
    const proj = await createProject(`e2e-stop-${Date.now()}`);
    cleanup.add("project", proj.id);

    const agent = await createAgent(proj.id, `agent-stop-${Date.now()}`);
    cleanup.add("agent", agent.id);

    const res = await request.post(`${API_BASE}/agents/${agent.id}/stop`, {
      headers: headers(),
      data: { task_id: "" },
    });
    expect(res.status()).toBe(400);
  });
});
