import { render, screen, waitFor } from "@solidjs/testing-library";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { ChannelMessageRecord } from "~/api/types";
import type { WSMessage } from "~/api/websocket";

const ws = vi.hoisted(() => {
  const handlers = new Set<(msg: WSMessage) => void>();
  return {
    handlers,
    emit(msg: WSMessage): void {
      for (const h of handlers) h(msg);
    },
  };
});

const apiMock = vi.hoisted(() => ({
  get: vi.fn<(id: string) => Promise<unknown>>(),
  messages: vi.fn<(id: string) => Promise<ChannelMessageRecord[]>>(),
}));

// jsdom has no matchMedia; the UI modules read it when they are loaded.
vi.hoisted(() => {
  Object.defineProperty(window, "matchMedia", {
    writable: true,
    value: (query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: () => undefined,
      removeListener: () => undefined,
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
      dispatchEvent: () => false,
    }),
  });
});

vi.mock("@solidjs/router", () => ({
  A: (props: Record<string, unknown>) => props,
  useNavigate: () => () => undefined,
  useLocation: () => ({ pathname: "/" }),
  useParams: () => ({ id: "ch-1" }),
}));

vi.mock("~/api/client", () => ({
  api: { channels: { get: apiMock.get, messages: apiMock.messages } },
}));

vi.mock("~/components/WebSocketProvider", () => ({
  useWebSocket: () => ({
    onMessage: (handler: (msg: WSMessage) => void) => {
      ws.handlers.add(handler);
      return () => ws.handlers.delete(handler);
    },
  }),
}));

import ChannelView from "./ChannelView";
import ThreadPanel from "./ThreadPanel";

function record(
  id: string,
  content: string,
  parentId = "",
  channelId = "ch-1",
): ChannelMessageRecord {
  return {
    id,
    channel_id: channelId,
    sender_type: "user",
    sender_name: "bob",
    content,
    parent_id: parentId,
    created_at: "2026-09-30T10:00:00Z",
  };
}

function channelMessage(m: ChannelMessageRecord): WSMessage {
  return { type: "channel.message", payload: { channel_id: m.channel_id, message: m } };
}

beforeEach(() => {
  ws.handlers.clear();
  Element.prototype.scrollIntoView = () => undefined;
  apiMock.get.mockReset().mockResolvedValue({
    id: "ch-1",
    name: "general",
    type: "project",
    description: "",
    project_id: "",
    created_at: "",
  });
  apiMock.messages.mockReset().mockResolvedValue([record("m-1", "first message")]);
});

// KI-42: messages from other users, agents and webhooks appear without a reload.
describe("ChannelView live updates", () => {
  it("appends a channel.message of this channel once", async () => {
    render(() => <ChannelView />);
    await screen.findByText("first message");

    const incoming = record("m-2", "hello from another user");
    ws.emit(channelMessage(incoming));
    ws.emit(channelMessage(incoming));

    await screen.findByText("hello from another user");
    expect(screen.getAllByText("hello from another user")).toHaveLength(1);
    expect(apiMock.messages).toHaveBeenCalledTimes(1);
  });

  it("ignores messages of other channels", async () => {
    render(() => <ChannelView />);
    await screen.findByText("first message");

    ws.emit(channelMessage(record("m-3", "elsewhere", "", "ch-2")));

    await waitFor(() => expect(screen.queryByText("elsewhere")).toBeNull());
  });

  it("stops listening when unmounted", async () => {
    const { unmount } = render(() => <ChannelView />);
    await screen.findByText("first message");
    expect(ws.handlers.size).toBe(1);
    unmount();
    expect(ws.handlers.size).toBe(0);
  });
});

describe("ThreadPanel live updates", () => {
  const parent = {
    id: "m-1",
    sender_type: "user",
    sender_name: "alice",
    content: "parent",
    created_at: "2026-09-30T10:00:00Z",
  };

  it("appends replies to its parent message only, in order", async () => {
    apiMock.messages.mockResolvedValue([
      record("r-2", "second reply", "m-1"),
      record("r-1", "first reply", "m-1"),
      record("m-1", "parent"),
    ]);
    render(() => (
      <ThreadPanel channelId="ch-1" parentMessage={parent} visible onClose={() => undefined} />
    ));
    await screen.findByText("second reply");

    ws.emit(channelMessage(record("r-3", "live reply", "m-1")));
    ws.emit(channelMessage(record("r-3", "live reply", "m-1")));
    ws.emit(channelMessage(record("x-1", "other thread", "m-9")));
    ws.emit(channelMessage(record("x-2", "top-level message")));

    await screen.findByText("live reply");
    expect(screen.getAllByText("live reply")).toHaveLength(1);
    expect(screen.queryByText("other thread")).toBeNull();
    expect(screen.queryByText("top-level message")).toBeNull();

    const order = ["first reply", "second reply", "live reply"].map((text) =>
      screen.getByText(text),
    );
    expect(
      order[0].compareDocumentPosition(order[1]) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    expect(
      order[1].compareDocumentPosition(order[2]) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });
});
