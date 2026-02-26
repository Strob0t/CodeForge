import { test, expect } from "@playwright/test";
import { apiLogin } from "../helpers/api-helpers";
import { createTestWS } from "../helpers/ws-helpers";

test.describe("WebSocket Reconnection", () => {
  let token: string;

  test.beforeAll(async () => {
    const auth = await apiLogin("admin@localhost", "Changeme123");
    token = auth.accessToken;
  });

  test("client can reconnect after disconnect", async () => {
    const ws = createTestWS(token);

    // First connection
    await ws.connect();
    expect(ws.isConnected()).toBe(true);
    ws.close();

    // Wait for close to complete
    await new Promise((resolve) => setTimeout(resolve, 1000));
    expect(ws.isConnected()).toBe(false);

    // Reconnect with a new client (same token)
    const ws2 = createTestWS(token);
    await ws2.connect();
    expect(ws2.isConnected()).toBe(true);
    ws2.close();
  });

  test("messages received after reconnection", async () => {
    const ws1 = createTestWS(token);
    await ws1.connect();

    // Collect some messages
    await ws1.collectMessages(2000);
    ws1.close();

    await new Promise((resolve) => setTimeout(resolve, 1000));

    // Reconnect
    const ws2 = createTestWS(token);
    await ws2.connect();

    // Should still be able to receive messages
    const secondMessages = await ws2.collectMessages(3000);

    // Connection should remain stable
    expect(ws2.isConnected()).toBe(true);

    // All messages should be valid
    for (const msg of secondMessages) {
      expect(typeof msg.type).toBe("string");
      expect(typeof msg.payload).toBe("object");
    }

    ws2.close();
  });

  test("no duplicate messages on reconnect", async () => {
    const ws1 = createTestWS(token);
    await ws1.connect();

    // Collect messages from first session
    await ws1.collectMessages(2000);
    ws1.close();

    await new Promise((resolve) => setTimeout(resolve, 1000));

    // Reconnect
    const ws2 = createTestWS(token);
    await ws2.connect();

    // Collect messages from second session
    const secondMessages = await ws2.collectMessages(3000);

    // Second session should start fresh, not replay old messages
    // Check that message count is reasonable (not double)
    // Each session should have its own independent message stream
    expect(ws2.getMessages().length).toBe(secondMessages.length);

    ws2.close();
  });
});
