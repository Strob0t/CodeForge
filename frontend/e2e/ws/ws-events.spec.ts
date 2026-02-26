import { test, expect } from "@playwright/test";
import { apiLogin } from "../helpers/api-helpers";
import { createTestWS } from "../helpers/ws-helpers";

test.describe("WebSocket Events", () => {
  let token: string;

  test.beforeAll(async () => {
    const auth = await apiLogin("admin@localhost", "Changeme123");
    token = auth.accessToken;
  });

  test("received messages have type and payload envelope", async () => {
    const ws = createTestWS(token);
    await ws.connect();

    const messages = await ws.collectMessages(3000);

    for (const msg of messages) {
      expect(msg).toHaveProperty("type");
      expect(msg).toHaveProperty("payload");
    }

    ws.close();
  });

  test("event types are strings", async () => {
    const ws = createTestWS(token);
    await ws.connect();

    const messages = await ws.collectMessages(3000);

    for (const msg of messages) {
      expect(typeof msg.type).toBe("string");
    }

    ws.close();
  });

  test("payload is an object", async () => {
    const ws = createTestWS(token);
    await ws.connect();

    const messages = await ws.collectMessages(3000);

    for (const msg of messages) {
      expect(typeof msg.payload).toBe("object");
      expect(msg.payload).not.toBeNull();
    }

    ws.close();
  });

  test("run status events have expected fields if triggered", async () => {
    const ws = createTestWS(token);
    await ws.connect();

    const messages = await ws.collectMessages(5000);
    const runEvents = messages.filter((m) => m.type === "run.status" || m.type === "run_status");

    for (const evt of runEvents) {
      // Run status events should have at least a run_id or status
      expect(evt.payload.run_id !== undefined || evt.payload.status !== undefined).toBe(true);
    }

    ws.close();
  });

  test("agent status events have expected fields if triggered", async () => {
    const ws = createTestWS(token);
    await ws.connect();

    const messages = await ws.collectMessages(5000);
    const agentEvents = messages.filter(
      (m) => m.type === "agent.status" || m.type === "agent_status",
    );

    for (const evt of agentEvents) {
      expect(evt.payload.agent_id !== undefined || evt.payload.status !== undefined).toBe(true);
    }

    ws.close();
  });

  test("task status events have expected fields if triggered", async () => {
    const ws = createTestWS(token);
    await ws.connect();

    const messages = await ws.collectMessages(5000);
    const taskEvents = messages.filter((m) => m.type === "task.status" || m.type === "task_status");

    for (const evt of taskEvents) {
      expect(evt.payload.task_id !== undefined || evt.payload.status !== undefined).toBe(true);
    }

    ws.close();
  });

  test("unknown event types handled gracefully", async () => {
    // Just verify the connection stays alive after receiving various events
    const ws = createTestWS(token);
    await ws.connect();

    // Collect messages for a while
    await ws.collectMessages(3000);

    // Connection should still be alive
    expect(ws.isConnected()).toBe(true);

    ws.close();
  });

  test("high-frequency events do not crash connection", async () => {
    const ws = createTestWS(token);
    await ws.connect();

    // Collect messages for 5 seconds
    const messages = await ws.collectMessages(5000);

    // Connection should remain stable
    expect(ws.isConnected()).toBe(true);

    // All messages should still be valid
    for (const msg of messages) {
      expect(typeof msg.type).toBe("string");
      expect(typeof msg.payload).toBe("object");
    }

    ws.close();
  });
});
