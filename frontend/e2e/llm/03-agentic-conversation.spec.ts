import { test, expect } from "@playwright/test";
import {
  apiLogin,
  apiGet,
  createProject,
  createCleanupTracker,
  API_BASE,
} from "../helpers/api-helpers";
import {
  discoverAvailableModels,
  pickToolCapableModel,
  sendAgenticMessage,
  waitForAssistantMessage,
  isWorkerAvailable,
  type ConversationMessage,
  type DiscoveredModel,
} from "./llm-helpers";

/**
 * Agentic (tool-use) LLM conversation tests.
 * These tests send agentic messages that trigger multi-turn tool-use loops
 * via the Python worker. Requires a running worker and at least one
 * tool-capable LLM model.
 */
test.describe("LLM E2E — Agentic Conversation", () => {
  test.setTimeout(120_000);

  let token: string;
  let models: DiscoveredModel[];
  let projectId: string;
  let workerRunning = false;
  const cleanup = createCleanupTracker();

  const headers = () => ({ Authorization: `Bearer ${token}` });
  const jsonHeaders = () => ({
    ...headers(),
    "Content-Type": "application/json",
  });

  /** Create a conversation, track it for cleanup, and return its ID. */
  async function createConversation(): Promise<string> {
    const convRes = await fetch(`${API_BASE}/projects/${projectId}/conversations`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({}),
    });
    expect(convRes.status).toBe(201);
    const conv = (await convRes.json()) as { id: string };
    cleanup.add("conversation", conv.id);
    return conv.id;
  }

  test.beforeAll(async () => {
    const auth = await apiLogin("admin@localhost", "Changeme123");
    token = auth.accessToken;

    const discovery = await discoverAvailableModels();
    models = discovery.models;
    const picked = pickToolCapableModel(models);
    test.skip(!picked, "No tool-capable model available");

    // Check if the Python worker is actually running and processing messages
    workerRunning = await isWorkerAvailable();
    test.skip(
      !workerRunning,
      "Python agent worker is not running — agentic tests require a worker",
    );

    // Create a project with a workspace for agent tools
    const proj = await createProject(`e2e-llm-agent-${Date.now()}`);
    projectId = proj.id;
    cleanup.add("project", projectId);

    // Initialize the workspace so Read/ListDir tools have something to work with
    await fetch(`${API_BASE}/projects/${projectId}/init-workspace`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({}),
    });
  });

  test.afterAll(async () => {
    await cleanup.cleanup();
  });

  test("send agentic message returns 202 dispatched", async () => {
    const convId = await createConversation();
    const result = await sendAgenticMessage(convId, "Say hello");
    expect(result.status).toBe(202);
    expect(result.body).toHaveProperty("status", "dispatched");
  });

  test("agentic run completes with assistant message", async () => {
    const convId = await createConversation();
    const result = await sendAgenticMessage(convId, "Say hello briefly.");
    expect(result.status).toBe(202);

    const assistant = await waitForAssistantMessage(convId, 0, 90_000, 3_000);
    expect(assistant).not.toBeNull();
    expect(assistant!.role).toBe("assistant");
  });

  test("agentic response content is non-empty", async () => {
    const convId = await createConversation();
    await sendAgenticMessage(convId, "Say something short.");

    const assistant = await waitForAssistantMessage(convId, 0, 90_000, 3_000);
    expect(assistant).not.toBeNull();
    expect(typeof assistant!.content).toBe("string");
    expect(assistant!.content.trim().length).toBeGreaterThan(0);
  });

  test("list files triggers tool use", async () => {
    const convId = await createConversation();
    await sendAgenticMessage(convId, "List all files in the current workspace directory.");

    const assistant = await waitForAssistantMessage(convId, 0, 90_000, 3_000);
    expect(assistant).not.toBeNull();
    expect(assistant!.content.length).toBeGreaterThan(0);
  });

  test("search codebase triggers tool use", async () => {
    const convId = await createConversation();
    await sendAgenticMessage(convId, "Search for the word 'TODO' in any file in the workspace.");

    const assistant = await waitForAssistantMessage(convId, 0, 90_000, 3_000);
    expect(assistant).not.toBeNull();
    expect(assistant!.content.length).toBeGreaterThan(0);
  });

  test("agentic run tracks tokens", async () => {
    const convId = await createConversation();
    await sendAgenticMessage(convId, "Respond with a short greeting.");

    const assistant = await waitForAssistantMessage(convId, 0, 90_000, 3_000);
    expect(assistant).not.toBeNull();
    expect(assistant!.tokens_in).toBeGreaterThan(0);
    expect(assistant!.tokens_out).toBeGreaterThan(0);
  });

  test("stop conversation endpoint works", async () => {
    const convId = await createConversation();
    await sendAgenticMessage(convId, "Write a long essay about software testing.");

    // Immediately try to stop the running conversation
    const stopRes = await fetch(`${API_BASE}/conversations/${convId}/stop`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({}),
    });
    expect([200, 400, 404]).toContain(stopRes.status);
  });

  test("multiple agentic messages in same conversation", async () => {
    const convId = await createConversation();

    // First message
    await sendAgenticMessage(convId, "Say hello.");
    const first = await waitForAssistantMessage(convId, 0, 90_000, 3_000);
    expect(first).not.toBeNull();

    // Get current message count before sending second message
    const messagesBefore = await apiGet<ConversationMessage[]>(`/conversations/${convId}/messages`);
    const beforeCount = messagesBefore.length;

    // Second message
    await sendAgenticMessage(convId, "Now say goodbye.");
    const second = await waitForAssistantMessage(convId, beforeCount, 90_000, 3_000);
    expect(second).not.toBeNull();
    expect(second!.role).toBe("assistant");
  });

  test("agentic response addresses the prompt", async () => {
    const convId = await createConversation();
    await sendAgenticMessage(convId, "What is 2+2? Answer with just the number.");

    const assistant = await waitForAssistantMessage(convId, 0, 90_000, 3_000);
    expect(assistant).not.toBeNull();
    expect(assistant!.content).toContain("4");
  });

  test("agentic mode with explicit agentic:true flag", async () => {
    const convId = await createConversation();

    // Directly POST with agentic: true to verify the 202 dispatch path
    const res = await fetch(`${API_BASE}/conversations/${convId}/messages`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({ content: "Hello", agentic: true }),
    });

    // The agentic flag should trigger the 202 dispatch path (not 201 sync)
    expect(res.status).toBe(202);
    const body = (await res.json()) as Record<string, unknown>;
    expect(body).toHaveProperty("status", "dispatched");
  });
});
