import { createRoot } from "solid-js";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { buildWSURL, createCodeForgeWS, parseWSMessage } from "./websocket";

const ACCESS_TOKEN = "jwt-access-token-must-not-leak";

const clientMock = vi.hoisted(() => ({
  wsTicket: vi.fn<() => Promise<{ ticket: string; expires_in: number }>>(),
  accessToken: "jwt-access-token-must-not-leak" as string | null,
}));

vi.mock("~/api/client", () => ({
  api: { auth: { wsTicket: clientMock.wsTicket } },
  getAccessToken: () => clientMock.accessToken,
}));

/** Records every URL a WebSocket is opened with instead of connecting. */
class FakeWebSocket extends EventTarget {
  static urls: string[] = [];
  constructor(public readonly url: string) {
    super();
    FakeWebSocket.urls.push(url);
  }
  close(): void {
    this.dispatchEvent(new Event("close"));
  }
}

/** Resolves once all pending promise callbacks have run. */
function flushPromises(): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, 0));
}

describe("parseWSMessage", () => {
  it("returns null for null input", () => {
    expect(parseWSMessage(null)).toBeNull();
  });
  it("returns null for non-string input", () => {
    expect(parseWSMessage(42)).toBeNull();
  });
  it("returns null for invalid JSON", () => {
    expect(parseWSMessage("{not json")).toBeNull();
  });
  it("returns null when type field is missing", () => {
    expect(parseWSMessage(JSON.stringify({ event: "test" }))).toBeNull();
  });
  it("returns null when type is not a string", () => {
    expect(parseWSMessage(JSON.stringify({ type: 42 }))).toBeNull();
  });
  it("returns message with empty payload when payload is missing", () => {
    const result = parseWSMessage(JSON.stringify({ type: "agui.run_started" }));
    expect(result).toEqual({ type: "agui.run_started", payload: {} });
  });
  it("returns message with payload when present", () => {
    const msg = { type: "agui.tool_call", payload: { call_id: "c1" } };
    expect(parseWSMessage(JSON.stringify(msg))).toEqual(msg);
  });
  it("returns null for empty string", () => {
    expect(parseWSMessage("")).toBeNull();
  });
});

describe("buildWSURL", () => {
  it("uses ws: on http and passes the ticket", () => {
    expect(buildWSURL("t-1", { protocol: "http:", host: "localhost:3000" })).toBe(
      "ws://localhost:3000/ws?ticket=t-1",
    );
  });
  it("uses wss: on https", () => {
    expect(buildWSURL("t-1", { protocol: "https:", host: "codeforge.example" })).toBe(
      "wss://codeforge.example/ws?ticket=t-1",
    );
  });
  it("url-encodes the ticket", () => {
    expect(buildWSURL("a b&token=x", { protocol: "http:", host: "h" })).toBe(
      "ws://h/ws?ticket=a%20b%26token%3Dx",
    );
  });
  it("encodes an empty ticket as an empty value", () => {
    expect(buildWSURL("", { protocol: "http:", host: "h" })).toBe("ws://h/ws?ticket=");
  });
});

describe("createCodeForgeWS", () => {
  beforeEach(() => {
    FakeWebSocket.urls = [];
    clientMock.accessToken = ACCESS_TOKEN;
    clientMock.wsTicket.mockReset();
    vi.stubGlobal("WebSocket", FakeWebSocket);
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("connects with a fresh single-use ticket, never with the access token", async () => {
    clientMock.wsTicket
      .mockResolvedValueOnce({ ticket: "ticket-1", expires_in: 30 })
      .mockResolvedValueOnce({ ticket: "ticket-2", expires_in: 30 });

    await createRoot(async (dispose) => {
      const ws = createCodeForgeWS();
      await flushPromises();
      ws.reconnect();
      await flushPromises();
      dispose();
    });

    expect(clientMock.wsTicket).toHaveBeenCalledTimes(2);
    expect(FakeWebSocket.urls).toEqual([
      buildWSURL("ticket-1", location),
      buildWSURL("ticket-2", location),
    ]);
    for (const url of FakeWebSocket.urls) {
      expect(url).not.toContain(ACCESS_TOKEN);
      expect(url).not.toContain("token=");
    }
  });

  it("does not open a socket before the user is logged in", async () => {
    clientMock.accessToken = null;

    await createRoot(async (dispose) => {
      createCodeForgeWS();
      await flushPromises();
      dispose();
    });

    expect(clientMock.wsTicket).not.toHaveBeenCalled();
    expect(FakeWebSocket.urls).toEqual([]);
  });

  it("does not open a socket when the ticket request fails", async () => {
    clientMock.wsTicket.mockRejectedValue(new Error("401 unauthorized"));

    await createRoot(async (dispose) => {
      createCodeForgeWS();
      await flushPromises();
      dispose();
    });

    expect(clientMock.wsTicket).toHaveBeenCalledTimes(1);
    expect(FakeWebSocket.urls).toEqual([]);
  });

  it("disconnect closes the socket and stays closed until reconnect", async () => {
    vi.useFakeTimers();
    try {
      clientMock.wsTicket
        .mockResolvedValueOnce({ ticket: "ticket-1", expires_in: 30 })
        .mockResolvedValueOnce({ ticket: "ticket-2", expires_in: 30 });
      const closed = vi.spyOn(FakeWebSocket.prototype, "close");

      await createRoot(async (dispose) => {
        const ws = createCodeForgeWS();
        await vi.advanceTimersByTimeAsync(0);
        ws.disconnect();
        await vi.advanceTimersByTimeAsync(5000);
        expect(closed).toHaveBeenCalledTimes(1);
        expect(clientMock.wsTicket).toHaveBeenCalledTimes(1);

        ws.reconnect();
        await vi.advanceTimersByTimeAsync(0);
        dispose();
      });

      expect(FakeWebSocket.urls).toEqual([
        buildWSURL("ticket-1", location),
        buildWSURL("ticket-2", location),
      ]);
    } finally {
      vi.useRealTimers();
    }
  });

  it("does not poll for a login while disconnected", async () => {
    vi.useFakeTimers();
    try {
      clientMock.accessToken = null;
      clientMock.wsTicket.mockResolvedValue({ ticket: "ticket-1", expires_in: 30 });

      await createRoot(async (dispose) => {
        const ws = createCodeForgeWS();
        ws.disconnect();
        clientMock.accessToken = ACCESS_TOKEN;
        await vi.advanceTimersByTimeAsync(5000);
        dispose();
      });

      expect(clientMock.wsTicket).not.toHaveBeenCalled();
      expect(FakeWebSocket.urls).toEqual([]);
    } finally {
      vi.useRealTimers();
    }
  });

  it("does not open a socket when disposed while the ticket is pending", async () => {
    let resolveTicket: (v: { ticket: string; expires_in: number }) => void = () => undefined;
    clientMock.wsTicket.mockReturnValue(
      new Promise((resolve) => {
        resolveTicket = resolve;
      }),
    );

    createRoot((dispose) => {
      createCodeForgeWS();
      dispose();
    });
    resolveTicket({ ticket: "late-ticket", expires_in: 30 });
    await flushPromises();

    expect(FakeWebSocket.urls).toEqual([]);
  });
});
