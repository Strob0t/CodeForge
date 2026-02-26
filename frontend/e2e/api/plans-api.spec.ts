import { test, expect } from "@playwright/test";
import {
  apiLogin,
  createProject,
  createAgent,
  createTask,
  createCleanupTracker,
  API_BASE,
} from "../helpers/api-helpers";

test.describe("Plans API", () => {
  let token: string;
  let projectId: string;
  const cleanup = createCleanupTracker();

  test.beforeAll(async () => {
    const auth = await apiLogin("admin@localhost", "Changeme123");
    token = auth.accessToken;
    const proj = await createProject(`plans-e2e-${Date.now()}`);
    projectId = proj.id;
    cleanup.add("project", projectId);
  });

  test.afterAll(async () => {
    await cleanup.cleanup();
  });

  const headers = () => ({ Authorization: `Bearer ${token}` });
  const jsonHeaders = () => ({
    ...headers(),
    "Content-Type": "application/json",
  });

  test("POST /projects/{id}/plans creates a plan", async () => {
    // CreatePlanRequest: name, description, project_id (set by handler),
    //   team_id, protocol, max_parallel, steps[]
    // CreateStepRequest: task_id, agent_id, policy_profile, mode_id, deliver_mode, depends_on
    // Plan validation requires at least one step with task_id and agent_id
    const agent = await createAgent(projectId, `plan-agent-${Date.now()}`);
    cleanup.add("agent", agent.id);
    const task = await createTask(projectId, `plan-task-${Date.now()}`);

    const res = await fetch(`${API_BASE}/projects/${projectId}/plans`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({
        name: "E2E Test Plan",
        description: "A plan created by e2e tests",
        protocol: "sequential",
        max_parallel: 1,
        steps: [{ task_id: task.id, agent_id: agent.id }],
      }),
    });
    expect(res.status).toBe(201);
    const body = await res.json();
    expect(body.id).toBeTruthy();
    expect(body.name).toBe("E2E Test Plan");
  });

  test("GET /projects/{id}/plans lists plans", async () => {
    const res = await fetch(`${API_BASE}/projects/${projectId}/plans`, {
      headers: headers(),
    });
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(Array.isArray(body)).toBe(true);
  });

  test("GET /plans/{id} returns a specific plan", async () => {
    // Create agent + task for the step (plan requires at least one step)
    const agent = await createAgent(projectId, `get-plan-agent-${Date.now()}`);
    cleanup.add("agent", agent.id);
    const task = await createTask(projectId, `get-plan-task-${Date.now()}`);

    // Create a plan first
    const createRes = await fetch(`${API_BASE}/projects/${projectId}/plans`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({
        name: "Get Plan Test",
        description: "For get test",
        protocol: "sequential",
        max_parallel: 1,
        steps: [{ task_id: task.id, agent_id: agent.id }],
      }),
    });
    expect(createRes.status).toBe(201);
    const plan = await createRes.json();

    const res = await fetch(`${API_BASE}/plans/${plan.id}`, {
      headers: headers(),
    });
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body.id).toBe(plan.id);
    expect(body.name).toBe("Get Plan Test");
  });

  test("POST /plans/{id}/start starts a plan", async () => {
    const agent = await createAgent(projectId, `start-plan-agent-${Date.now()}`);
    cleanup.add("agent", agent.id);
    const task = await createTask(projectId, `start-plan-task-${Date.now()}`);

    const createRes = await fetch(`${API_BASE}/projects/${projectId}/plans`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({
        name: "Start Plan Test",
        description: "For start test",
        protocol: "sequential",
        max_parallel: 1,
        steps: [{ task_id: task.id, agent_id: agent.id }],
      }),
    });
    expect(createRes.status).toBe(201);
    const plan = await createRes.json();

    const res = await fetch(`${API_BASE}/plans/${plan.id}/start`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({}),
    });
    // May return 200 or 400 depending on whether workers are available
    expect([200, 400]).toContain(res.status);
  });

  test("POST /plans/{id}/cancel cancels a plan", async () => {
    const agent = await createAgent(projectId, `cancel-plan-agent-${Date.now()}`);
    cleanup.add("agent", agent.id);
    const task = await createTask(projectId, `cancel-plan-task-${Date.now()}`);

    const createRes = await fetch(`${API_BASE}/projects/${projectId}/plans`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({
        name: "Cancel Plan Test",
        description: "For cancel test",
        protocol: "sequential",
        max_parallel: 1,
        steps: [{ task_id: task.id, agent_id: agent.id }],
      }),
    });
    expect(createRes.status).toBe(201);
    const plan = await createRes.json();

    const res = await fetch(`${API_BASE}/plans/${plan.id}/cancel`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({}),
    });
    // May succeed or fail depending on plan state
    expect([200, 400]).toContain(res.status);
  });

  test("POST /projects/{id}/decompose decomposes a feature", async () => {
    const res = await fetch(`${API_BASE}/projects/${projectId}/decompose`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({
        feature: "Add user authentication",
        context: "A web application",
      }),
    });
    // May fail if meta-agent/LLM not available, but should return valid HTTP status
    expect([201, 400, 502, 504]).toContain(res.status);
  });

  test("POST /projects/{id}/plan-feature plans a feature", async () => {
    const res = await fetch(`${API_BASE}/projects/${projectId}/plan-feature`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({
        feature: "Add dark mode",
        context: "Frontend application",
      }),
    });
    // May fail if task planner/LLM not available
    expect([201, 400, 502, 504]).toContain(res.status);
  });

  test("POST /projects/{id}/plans with missing name returns 400", async () => {
    const res = await fetch(`${API_BASE}/projects/${projectId}/plans`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({
        description: "No name provided",
        protocol: "sequential",
      }),
    });
    expect(res.status).toBe(400);
  });
});
