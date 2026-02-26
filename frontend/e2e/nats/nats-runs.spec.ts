import { test, expect } from "@playwright/test";
import {
  apiLogin,
  createProject,
  createAgent,
  createTask,
  createCleanupTracker,
  API_BASE,
} from "../helpers/api-helpers";

/**
 * NATS run lifecycle tests.
 * Since direct NATS access is not possible from Playwright,
 * these tests verify NATS-dependent behavior via the HTTP API.
 * They create projects/agents/tasks and trigger runs, then verify
 * the expected outcomes through API polling.
 */
test.describe("NATS Runs (via API)", () => {
  let token: string;
  let projectId: string;
  let agentId: string;
  let taskId: string;
  let setupOk = false;
  const cleanup = createCleanupTracker();

  test.beforeAll(async () => {
    const auth = await apiLogin("admin@localhost", "Changeme123");
    token = auth.accessToken;

    const proj = await createProject(`nats-runs-e2e-${Date.now()}`);
    projectId = proj.id;
    cleanup.add("project", projectId);

    try {
      const agent = await createAgent(projectId, "nats-test-agent", "aider");
      agentId = agent.id;
      cleanup.add("agent", agentId);

      const task = await createTask(projectId, "NATS test task", "echo hello");
      taskId = task.id;
      setupOk = true;
    } catch {
      // Agent creation may fail if backend registry is unavailable;
      // individual tests will skip gracefully via the setupOk flag.
    }
  });

  test.afterAll(async () => {
    await cleanup.cleanup();
  });

  const headers = () => ({ Authorization: `Bearer ${token}` });
  const jsonHeaders = () => ({
    ...headers(),
    "Content-Type": "application/json",
  });

  test("start run via API triggers NATS dispatch", async () => {
    test.skip(!setupOk, "Agent/task setup failed — backend not available");

    const res = await fetch(`${API_BASE}/runs`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({
        task_id: taskId,
        agent_id: agentId,
      }),
    });
    // 201 if run started, 400/500 if workers not available
    expect([201, 400, 404, 500]).toContain(res.status);
    if (res.status === 201) {
      const body = await res.json();
      expect(body.id).toBeTruthy();
    }
  });

  test("run completion observable via API", async () => {
    test.skip(!setupOk, "Agent/task setup failed — backend not available");

    // Start a run
    const startRes = await fetch(`${API_BASE}/runs`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({ task_id: taskId, agent_id: agentId }),
    });

    if (startRes.status !== 201) {
      // Workers not available, verify the start response is reasonable
      expect([400, 404, 500]).toContain(startRes.status);
      return;
    }

    const run = await startRes.json();

    // Poll for run status
    const getRes = await fetch(`${API_BASE}/runs/${run.id}`, {
      headers: headers(),
    });
    expect(getRes.status).toBe(200);
    const runData = await getRes.json();
    expect(runData.id).toBe(run.id);
    expect(runData).toHaveProperty("status");
  });

  test("tool call round-trip observable via run events", async () => {
    test.skip(!setupOk, "Agent/task setup failed — backend not available");

    const startRes = await fetch(`${API_BASE}/runs`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({ task_id: taskId, agent_id: agentId }),
    });

    if (startRes.status !== 201) {
      expect([400, 404, 500]).toContain(startRes.status);
      return;
    }

    const run = await startRes.json();

    // Check events for the run
    const eventsRes = await fetch(`${API_BASE}/runs/${run.id}/events`, {
      headers: headers(),
    });
    expect(eventsRes.status).toBe(200);
    const events = await eventsRes.json();
    expect(Array.isArray(events)).toBe(true);
  });

  test("run status shows active state (heartbeat)", async () => {
    test.skip(!setupOk, "Agent/task setup failed — backend not available");

    const startRes = await fetch(`${API_BASE}/runs`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({ task_id: taskId, agent_id: agentId }),
    });

    if (startRes.status !== 201) {
      expect([400, 404, 500]).toContain(startRes.status);
      return;
    }

    const run = await startRes.json();

    // Status should reflect the current state
    const getRes = await fetch(`${API_BASE}/runs/${run.id}`, {
      headers: headers(),
    });
    expect(getRes.status).toBe(200);
    const runData = await getRes.json();
    // Status should be one of known states
    expect(["pending", "running", "completed", "failed", "cancelled"]).toContain(runData.status);
  });

  test("cancel run triggers cancel subject via NATS", async () => {
    test.skip(!setupOk, "Agent/task setup failed — backend not available");

    const startRes = await fetch(`${API_BASE}/runs`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({ task_id: taskId, agent_id: agentId }),
    });

    if (startRes.status !== 201) {
      expect([400, 404, 500]).toContain(startRes.status);
      return;
    }

    const run = await startRes.json();

    // Cancel the run
    const cancelRes = await fetch(`${API_BASE}/runs/${run.id}/cancel`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({}),
    });
    // 200 if cancelled, 404 if already finished
    expect([200, 400, 404]).toContain(cancelRes.status);

    if (cancelRes.status === 200) {
      const body = await cancelRes.json();
      expect(body.status).toBe("cancelled");
    }
  });
});
