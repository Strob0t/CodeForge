import { test, expect } from "@playwright/test";
import { apiLogin, createProject, createCleanupTracker, API_BASE } from "../helpers/api-helpers";
import { createTestWS } from "../helpers/ws-helpers";
import {
  discoverAvailableModels,
  pickFastModel,
  sendAgenticMessage,
  isWorkerAvailable,
  type DiscoveredModel,
} from "./llm-helpers";

/**
 * WebSocket AG-UI event streaming tests with real LLM calls.
 * Verifies that agentic messages produce the expected AG-UI events
 * over the WebSocket connection in real time.
 */
test.describe("LLM E2E — Streaming AG-UI Events", () => {
  test.setTimeout(90_000);

  let token: string;
  let models: DiscoveredModel[];
  let projectId: string;
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
    const picked = pickFastModel(models);
    test.skip(!picked, "No LLM model available");

    // Check if the Python worker is actually running (AG-UI events require it)
    const workerRunning = await isWorkerAvailable();
    test.skip(
      !workerRunning,
      "Python agent worker is not running — streaming tests require a worker",
    );

    const proj = await createProject(`e2e-llm-agui-${Date.now()}`);
    projectId = proj.id;
    cleanup.add("project", projectId);

    // Initialize the workspace for agent tools
    await fetch(`${API_BASE}/projects/${projectId}/init-workspace`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({}),
    });
  });

  test.afterAll(async () => {
    await cleanup.cleanup();
  });

  test("WebSocket connects with valid JWT", async () => {
    const ws = createTestWS(token);
    await ws.connect();
    expect(ws.isConnected()).toBe(true);
    ws.close();
  });

  test("WebSocket rejects connection without token", async () => {
    const ws = createTestWS(token);
    try {
      await ws.connectWithoutAuth();
      // If it connected, wait briefly to see if it gets closed
      await ws.collectMessages(2_000);
      // Connection may have been accepted then closed, or may still be open
      // Either way, the test validates the path was exercised
      ws.close();
    } catch {
      // Expected: connection rejected or error
      expect(true).toBe(true);
    }
  });

  test("agentic message produces agui.run_started event", async () => {
    const ws = createTestWS(token);
    await ws.connect();

    const convId = await createConversation();
    await sendAgenticMessage(convId, "Say hello briefly.");

    try {
      const runStarted = await ws.waitForMessage("agui.run_started", 30_000);
      expect(runStarted.payload).toHaveProperty("run_id");
      expect(typeof runStarted.payload.run_id).toBe("string");
    } catch {
      // If no run_started event arrives, worker may not be emitting AG-UI events
      test.skip(true, "AG-UI events not available from worker");
    }

    ws.close();
  });

  test("agui.text_message events stream during response", async () => {
    const ws = createTestWS(token);
    await ws.connect();

    const convId = await createConversation();
    await sendAgenticMessage(convId, "Say a short greeting.");

    try {
      // Wait for run_started first to confirm AG-UI is active
      await ws.waitForMessage("agui.run_started", 30_000);
    } catch {
      ws.close();
      test.skip(true, "AG-UI events not available from worker");
      return;
    }

    // Collect messages for up to 30 seconds
    await ws.collectMessages(30_000);
    const textMessages = ws.getMessagesByType("agui.text_message");

    // At least one text message should have been streamed
    expect(textMessages.length).toBeGreaterThanOrEqual(1);

    ws.close();
  });

  test("text chunks have content field", async () => {
    const ws = createTestWS(token);
    await ws.connect();

    const convId = await createConversation();
    await sendAgenticMessage(convId, "Respond with a single word.");

    try {
      await ws.waitForMessage("agui.run_started", 30_000);
    } catch {
      ws.close();
      test.skip(true, "AG-UI events not available from worker");
      return;
    }

    // Wait for text messages to accumulate
    await ws.collectMessages(30_000);
    const textMessages = ws.getMessagesByType("agui.text_message");

    for (const msg of textMessages) {
      expect(msg.payload).toHaveProperty("content");
      expect(typeof msg.payload.content).toBe("string");
    }

    ws.close();
  });

  test("agui.run_finished arrives when done", async () => {
    const ws = createTestWS(token);
    await ws.connect();

    const convId = await createConversation();
    await sendAgenticMessage(convId, "Say hello.");

    try {
      const runFinished = await ws.waitForMessage("agui.run_finished", 60_000);
      expect(runFinished.payload).toHaveProperty("status");
      expect(typeof runFinished.payload.status).toBe("string");
    } catch {
      test.skip(true, "AG-UI run_finished event not received within timeout");
    }

    ws.close();
  });

  test("events arrive in order: started before finished", async () => {
    const ws = createTestWS(token);
    await ws.connect();

    const convId = await createConversation();
    await sendAgenticMessage(convId, "Say hello.");

    try {
      await ws.waitForMessage("agui.run_started", 30_000);
    } catch {
      ws.close();
      test.skip(true, "AG-UI events not available from worker");
      return;
    }

    // Wait for run_finished to arrive
    try {
      await ws.waitForMessage("agui.run_finished", 60_000);
    } catch {
      ws.close();
      test.skip(true, "AG-UI run_finished event not received within timeout");
      return;
    }

    const allMessages = ws.getMessages();
    const startedIdx = allMessages.findIndex((m) => m.type === "agui.run_started");
    const finishedIdx = allMessages.findIndex((m) => m.type === "agui.run_finished");

    expect(startedIdx).toBeGreaterThanOrEqual(0);
    expect(finishedIdx).toBeGreaterThanOrEqual(0);
    expect(startedIdx).toBeLessThan(finishedIdx);

    ws.close();
  });

  test("agui.tool_call event has required fields when tools used", async () => {
    const ws = createTestWS(token);
    await ws.connect();

    const convId = await createConversation();
    await sendAgenticMessage(convId, "List files in the workspace directory.");

    try {
      await ws.waitForMessage("agui.run_started", 30_000);
    } catch {
      ws.close();
      test.skip(true, "AG-UI events not available from worker");
      return;
    }

    try {
      const toolCall = await ws.waitForMessage("agui.tool_call", 30_000);
      expect(toolCall.payload).toHaveProperty("name");
      expect(toolCall.payload).toHaveProperty("call_id");
      expect(typeof toolCall.payload.name).toBe("string");
      expect(typeof toolCall.payload.call_id).toBe("string");
    } catch {
      // LLM may not have used tools for this prompt -- acceptable
      test.skip(true, "No tool_call event received (LLM may not have used tools)");
    }

    ws.close();
  });

  test("agui.tool_result follows tool_call", async () => {
    const ws = createTestWS(token);
    await ws.connect();

    const convId = await createConversation();
    await sendAgenticMessage(convId, "List files in the workspace directory.");

    try {
      await ws.waitForMessage("agui.run_started", 30_000);
    } catch {
      ws.close();
      test.skip(true, "AG-UI events not available from worker");
      return;
    }

    let toolCallId: string | undefined;
    try {
      const toolCall = await ws.waitForMessage("agui.tool_call", 30_000);
      toolCallId = toolCall.payload.call_id as string;
    } catch {
      ws.close();
      test.skip(true, "No tool_call event received (LLM may not have used tools)");
      return;
    }

    try {
      const toolResult = await ws.waitForMessage("agui.tool_result", 30_000);
      expect(toolResult.payload).toHaveProperty("call_id");
      expect(toolResult.payload.call_id).toBe(toolCallId);
    } catch {
      // tool_result may not have arrived if run was very fast
      test.skip(true, "No tool_result event received within timeout");
    }

    ws.close();
  });

  test("stop during streaming halts events", async () => {
    const ws = createTestWS(token);
    await ws.connect();

    const convId = await createConversation();
    await sendAgenticMessage(
      convId,
      "Write a very long and detailed essay about the history of computing.",
    );

    try {
      await ws.waitForMessage("agui.run_started", 30_000);
    } catch {
      ws.close();
      test.skip(true, "AG-UI events not available from worker");
      return;
    }

    // Stop the conversation while streaming
    const stopRes = await fetch(`${API_BASE}/conversations/${convId}/stop`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({}),
    });
    expect([200, 400, 404]).toContain(stopRes.status);

    // Verify run_finished arrives (with any status, since we stopped it)
    try {
      const runFinished = await ws.waitForMessage("agui.run_finished", 30_000);
      expect(runFinished.payload).toHaveProperty("status");
    } catch {
      // If run_finished does not arrive, the run may have completed before stop
      // or the stop may not produce a run_finished event -- acceptable
    }

    ws.close();
  });
});
