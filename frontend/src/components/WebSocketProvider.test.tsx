import { render } from "@solidjs/testing-library";
import { createSignal } from "solid-js";
import { beforeEach, describe, expect, it, vi } from "vitest";

const wsMock = vi.hoisted(() => ({
  reconnect: vi.fn<() => void>(),
  disconnect: vi.fn<() => void>(),
}));

vi.mock("~/api/websocket", () => ({
  createCodeForgeWS: () => ({
    connected: () => false,
    onMessage: () => () => undefined,
    onAGUIEvent: () => () => undefined,
    reconnect: wsMock.reconnect,
    disconnect: wsMock.disconnect,
  }),
}));

import { WebSocketProvider } from "./WebSocketProvider";

describe("WebSocketProvider", () => {
  const [sessionUserID, setSessionUserID] = createSignal<string | null>("u-1");

  function renderProvider(): void {
    render(() => <WebSocketProvider sessionUserID={sessionUserID}>child</WebSocketProvider>);
  }

  beforeEach(() => {
    wsMock.reconnect.mockReset();
    wsMock.disconnect.mockReset();
    setSessionUserID("u-1");
  });

  it("keeps the socket of the current session open", () => {
    renderProvider();
    setSessionUserID("u-1");
    expect(wsMock.reconnect).not.toHaveBeenCalled();
    expect(wsMock.disconnect).not.toHaveBeenCalled();
  });

  it("closes the socket on logout and reopens it on the next login", () => {
    renderProvider();
    setSessionUserID(null);
    expect(wsMock.disconnect).toHaveBeenCalledTimes(1);

    setSessionUserID("u-2");
    expect(wsMock.reconnect).toHaveBeenCalledTimes(1);
  });

  it("reconnects when another user logs in without a logout", () => {
    renderProvider();
    setSessionUserID("u-2");
    expect(wsMock.reconnect).toHaveBeenCalledTimes(1);
    expect(wsMock.disconnect).not.toHaveBeenCalled();
  });

  it("does not connect before the session is known", () => {
    setSessionUserID(null);
    renderProvider();
    expect(wsMock.disconnect).toHaveBeenCalledTimes(1);
    expect(wsMock.reconnect).not.toHaveBeenCalled();
  });
});
