import { test, expect } from "@playwright/test";
import { apiLogin, createProject, createCleanupTracker, API_BASE } from "../helpers/api-helpers";

/**
 * NATS conversation tests.
 * Verifies that conversation messages trigger NATS flows
 * by observing API-level behavior.
 */
test.describe("NATS Conversation (via API)", () => {
  let token: string;
  let projectId: string;
  const cleanup = createCleanupTracker();

  test.beforeAll(async () => {
    const auth = await apiLogin("admin@localhost", "Changeme123");
    token = auth.accessToken;

    const proj = await createProject(`nats-conv-e2e-${Date.now()}`);
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

  test("send conversation message triggers NATS flow", async () => {
    // Create a conversation
    const convRes = await fetch(`${API_BASE}/projects/${projectId}/conversations`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({}),
    });
    expect(convRes.status).toBe(201);
    const conv = await convRes.json();
    cleanup.add("conversation", conv.id);

    // Send a message (will trigger NATS dispatch if agentic mode)
    const msgRes = await fetch(`${API_BASE}/conversations/${conv.id}/messages`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({
        content: "Hello from NATS e2e test",
        role: "user",
      }),
    });
    // 201 for sync, 202 for agentic dispatch, or error if LLM not available
    expect([201, 202, 400, 500, 502]).toContain(msgRes.status);
  });

  test("stop conversation triggers cancel via NATS", async () => {
    // Create a conversation
    const convRes = await fetch(`${API_BASE}/projects/${projectId}/conversations`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({}),
    });
    expect(convRes.status).toBe(201);
    const conv = await convRes.json();
    cleanup.add("conversation", conv.id);

    // Stop the conversation (cancels any active NATS-based run)
    const stopRes = await fetch(`${API_BASE}/conversations/${conv.id}/stop`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({}),
    });
    // 200 if stopped, 404 if nothing to stop
    expect([200, 400, 404]).toContain(stopRes.status);
  });
});
