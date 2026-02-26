import { test, expect } from "@playwright/test";
import { apiLogin } from "../helpers/api-helpers";
import { createTestWS } from "../helpers/ws-helpers";

test.describe("WebSocket Connection", () => {
  let token: string;

  test.beforeAll(async () => {
    const auth = await apiLogin("admin@localhost", "Changeme123");
    token = auth.accessToken;
  });

  test("connect with valid JWT succeeds", async () => {
    const ws = createTestWS(token);
    await ws.connect();
    expect(ws.isConnected()).toBe(true);
    ws.close();
  });

  test("connect without token is rejected or closed", async () => {
    const ws = createTestWS("");
    let connected = false;
    let errorOccurred = false;

    try {
      await ws.connectWithoutAuth();
      connected = ws.isConnected();
      // If connection opened, wait briefly to see if server closes it
      if (connected) {
        await new Promise((resolve) => setTimeout(resolve, 2000));
        connected = ws.isConnected();
      }
    } catch {
      errorOccurred = true;
    }

    // Either connection should fail, or server should close it
    expect(errorOccurred || !connected).toBe(true);
    ws.close();
  });

  test("received messages are valid JSON with type and payload", async () => {
    const ws = createTestWS(token);
    await ws.connect();

    // Collect messages for a short period
    const messages = await ws.collectMessages(3000);

    // Every message must conform to { type, payload }
    for (const msg of messages) {
      expect(typeof msg.type).toBe("string");
      expect(msg.type.length).toBeGreaterThan(0);
      expect(typeof msg.payload).toBe("object");
    }

    ws.close();
  });

  test("multiple clients can connect simultaneously", async () => {
    const ws1 = createTestWS(token);
    const ws2 = createTestWS(token);
    const ws3 = createTestWS(token);

    await Promise.all([ws1.connect(), ws2.connect(), ws3.connect()]);

    expect(ws1.isConnected()).toBe(true);
    expect(ws2.isConnected()).toBe(true);
    expect(ws3.isConnected()).toBe(true);

    ws1.close();
    ws2.close();
    ws3.close();
  });

  test("clean close sends proper close frame", async () => {
    const ws = createTestWS(token);
    await ws.connect();
    expect(ws.isConnected()).toBe(true);

    ws.close();

    // After close, isConnected should return false
    // Give a small delay for the close handshake
    await new Promise((resolve) => setTimeout(resolve, 500));
    expect(ws.isConnected()).toBe(false);
  });
});
