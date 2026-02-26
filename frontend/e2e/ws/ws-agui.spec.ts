import { test, expect } from "@playwright/test";
import { apiLogin } from "../helpers/api-helpers";
import { createTestWS } from "../helpers/ws-helpers";

/**
 * AG-UI event type validation tests.
 * These verify the contract of AG-UI events if/when they appear on the WebSocket.
 * Since triggering actual agent runs requires workers, we validate the
 * structural contract of AG-UI events based on any messages received,
 * and also verify the known event type strings are valid.
 */
test.describe("WebSocket AG-UI Events", () => {
  let token: string;

  test.beforeAll(async () => {
    const auth = await apiLogin("admin@localhost", "Changeme123");
    token = auth.accessToken;
  });

  const AGUI_TYPES = [
    "agui.run_started",
    "agui.run_finished",
    "agui.text_message",
    "agui.tool_call",
    "agui.tool_result",
    "agui.state_delta",
    "agui.step_started",
    "agui.step_finished",
  ];

  test("AG-UI event types match known spec values", async () => {
    const ws = createTestWS(token);
    await ws.connect();

    const messages = await ws.collectMessages(5000);
    const aguiMessages = messages.filter((m) => m.type.startsWith("agui."));

    // Every agui message type should be in the known set
    for (const msg of aguiMessages) {
      expect(AGUI_TYPES).toContain(msg.type);
    }

    ws.close();
  });

  test("agui.run_started has run_id field", async () => {
    const ws = createTestWS(token);
    await ws.connect();

    const messages = await ws.collectMessages(5000);
    const runStarted = messages.filter((m) => m.type === "agui.run_started");

    for (const msg of runStarted) {
      expect(msg.payload).toHaveProperty("run_id");
      expect(typeof msg.payload.run_id).toBe("string");
    }

    ws.close();
  });

  test("agui.run_finished has status field", async () => {
    const ws = createTestWS(token);
    await ws.connect();

    const messages = await ws.collectMessages(5000);
    const runFinished = messages.filter((m) => m.type === "agui.run_finished");

    for (const msg of runFinished) {
      expect(msg.payload).toHaveProperty("status");
      expect(typeof msg.payload.status).toBe("string");
    }

    ws.close();
  });

  test("agui.text_message has role and content fields", async () => {
    const ws = createTestWS(token);
    await ws.connect();

    const messages = await ws.collectMessages(5000);
    const textMessages = messages.filter((m) => m.type === "agui.text_message");

    for (const msg of textMessages) {
      expect(msg.payload).toHaveProperty("role");
      expect(msg.payload).toHaveProperty("content");
      expect(typeof msg.payload.role).toBe("string");
      expect(typeof msg.payload.content).toBe("string");
    }

    ws.close();
  });

  test("agui.tool_call has call_id, name, and args fields", async () => {
    const ws = createTestWS(token);
    await ws.connect();

    const messages = await ws.collectMessages(5000);
    const toolCalls = messages.filter((m) => m.type === "agui.tool_call");

    for (const msg of toolCalls) {
      expect(msg.payload).toHaveProperty("call_id");
      expect(msg.payload).toHaveProperty("name");
      expect(msg.payload).toHaveProperty("args");
    }

    ws.close();
  });

  test("agui.tool_result has call_id and result fields", async () => {
    const ws = createTestWS(token);
    await ws.connect();

    const messages = await ws.collectMessages(5000);
    const toolResults = messages.filter((m) => m.type === "agui.tool_result");

    for (const msg of toolResults) {
      expect(msg.payload).toHaveProperty("call_id");
      expect(msg.payload).toHaveProperty("result");
    }

    ws.close();
  });

  test("agui.state_delta has delta field", async () => {
    const ws = createTestWS(token);
    await ws.connect();

    const messages = await ws.collectMessages(5000);
    const stateDeltas = messages.filter((m) => m.type === "agui.state_delta");

    for (const msg of stateDeltas) {
      expect(msg.payload).toHaveProperty("delta");
    }

    ws.close();
  });
});
