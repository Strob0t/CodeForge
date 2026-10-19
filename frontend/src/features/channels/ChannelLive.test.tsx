import { fireEvent, render, screen, waitFor } from "@solidjs/testing-library";
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
  markRead: vi.fn<(id: string, messageId: string) => Promise<unknown>>(),
  regenerateWebhookKey: vi.fn<(id: string) => Promise<{ webhook_key: string }>>(),
  list: vi.fn<() => Promise<unknown[]>>(),
  send: vi.fn<(id: string, content: string, name: string) => Promise<ChannelMessageRecord>>(),
  sendThreadReply:
    vi.fn<(id: string, parentId: string, data: unknown) => Promise<ChannelMessageRecord>>(),
}));

const auth = vi.hoisted(() => ({ admin: false, editor: false, userId: "u-me" }));

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
  api: {
    channels: {
      get: apiMock.get,
      messages: apiMock.messages,
      markRead: apiMock.markRead,
      regenerateWebhookKey: apiMock.regenerateWebhookKey,
      list: apiMock.list,
      send: apiMock.send,
      sendThreadReply: apiMock.sendThreadReply,
    },
  },
}));

vi.mock("~/components/AuthProvider", () => ({
  useAuth: () => ({
    hasRole: (...roles: string[]) =>
      (auth.admin && roles.includes("admin")) || (auth.editor && roles.includes("editor")),
    user: () => ({ id: auth.userId }),
  }),
}));

vi.mock("~/components/ConfirmProvider", () => ({
  useConfirm: () => ({ confirm: () => Promise.resolve(true) }),
}));

vi.mock("~/components/SidebarProvider", () => ({
  useSidebar: () => ({ collapsed: () => false }),
}));

vi.mock("~/components/WebSocketProvider", () => ({
  useWebSocket: () => ({
    onMessage: (handler: (msg: WSMessage) => void) => {
      ws.handlers.add(handler);
      return () => ws.handlers.delete(handler);
    },
  }),
}));

import ChannelList from "./ChannelList";
import { followedChannel, noteUntrackedRead, setFollowedChannel } from "./channelReading";
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

/** Gives the jsdom message list a scroll position (jsdom has no layout). */
function scrollMessages(scrollTop: number): void {
  const list = screen.getByTestId("channel-messages");
  Object.defineProperty(list, "scrollHeight", { configurable: true, value: 2000 });
  Object.defineProperty(list, "clientHeight", { configurable: true, value: 500 });
  Object.defineProperty(list, "scrollTop", { configurable: true, value: scrollTop });
  fireEvent.scroll(list);
}

beforeEach(() => {
  ws.handlers.clear();
  auth.admin = false;
  auth.editor = false;
  setFollowedChannel(undefined);
  Element.prototype.scrollIntoView = () => undefined;
  apiMock.get.mockReset().mockResolvedValue({
    id: "ch-1",
    name: "general",
    type: "project",
    description: "",
    project_id: "",
    created_at: "",
    has_webhook_key: false,
    unread_count: 0,
  });
  apiMock.messages.mockReset().mockResolvedValue([record("m-1", "first message")]);
  apiMock.markRead.mockReset().mockResolvedValue({});
  apiMock.regenerateWebhookKey.mockReset().mockResolvedValue({ webhook_key: "k".repeat(64) });
  apiMock.list.mockReset().mockResolvedValue([]);
  apiMock.send.mockReset();
  apiMock.sendThreadReply.mockReset();
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

  it("keeps a message that arrives while the list is still loading", async () => {
    let resolveList: (list: ChannelMessageRecord[]) => void = () => undefined;
    apiMock.messages.mockReturnValue(
      new Promise((resolve) => {
        resolveList = resolve;
      }),
    );
    render(() => <ChannelView />);
    await waitFor(() => expect(ws.handlers.size).toBe(1));

    ws.emit(channelMessage(record("m-2", "posted during load")));
    resolveList([record("m-1", "first message")]);

    await screen.findByText("first message");
    expect(screen.getByText("posted during load")).toBeTruthy();
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

// KI-73: threads open in the thread panel, the newest message is marked read,
// admins create the webhook key, and the channel list shows unread counts.
describe("ChannelView threads and read state", () => {
  it("shows top-level messages with their reply count and opens the thread", async () => {
    apiMock.messages.mockResolvedValue([
      record("r-1", "a reply", "m-1"),
      record("m-1", "first message"),
    ]);
    render(() => <ChannelView />);
    await screen.findByText("first message");
    expect(screen.queryByText("a reply")).toBeNull();

    fireEvent.click(screen.getByText("1 reply"));

    await screen.findByText("Thread");
    expect(await screen.findByText("a reply")).toBeTruthy();
  });

  it("marks the newest message read after loading", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    try {
      apiMock.messages.mockResolvedValue([record("m-2", "newest"), record("m-1", "older")]);
      render(() => <ChannelView />);
      await screen.findByText("newest");
      await vi.advanceTimersByTimeAsync(1100);
      expect(apiMock.markRead).toHaveBeenCalledWith("ch-1", "m-2");
      expect(apiMock.markRead).toHaveBeenCalledTimes(1);
    } finally {
      vi.useRealTimers();
    }
  });

  it("lets admins create the webhook key and shows it once", async () => {
    auth.admin = true;
    render(() => <ChannelView />);
    fireEvent.click(await screen.findByText("Create webhook key"));
    const shown = await screen.findByTestId("webhook-key");
    expect(shown.textContent).toBe("k".repeat(64));
    expect(apiMock.regenerateWebhookKey).toHaveBeenCalledWith("ch-1");

    fireEvent.click(screen.getByText("Done"));
    await waitFor(() => expect(screen.queryByTestId("webhook-key")).toBeNull());
  });

  it("offers no webhook key to non-admins", async () => {
    render(() => <ChannelView />);
    await screen.findByText("# general");
    expect(screen.queryByText("Create webhook key")).toBeNull();
  });
});

describe("ChannelList unread counts", () => {
  function channel(id: string, name: string, unread: number): Record<string, unknown> {
    return { id, name, type: "project", unread_count: unread, has_webhook_key: false };
  }

  function readEvent(userId: string): WSMessage {
    return {
      type: "channel.read",
      payload: { channel_id: "ch-1", user_id: userId, message_id: "m-1" },
    };
  }

  it("shows the server counts and follows new messages and own reads", async () => {
    apiMock.list.mockResolvedValue([channel("ch-1", "general", 2), channel("ch-2", "random", 0)]);
    render(() => <ChannelList />);
    expect((await screen.findByLabelText("2 unread")).textContent).toBe("2");

    ws.emit(channelMessage({ ...record("m-5", "hi", "", "ch-2"), sender_id: "u-bob" }));
    await screen.findByLabelText("1 unread");

    apiMock.list.mockResolvedValue([channel("ch-1", "general", 0), channel("ch-2", "random", 1)]);
    ws.emit(readEvent("u-me"));
    await waitFor(() => expect(screen.queryByLabelText("2 unread")).toBeNull());
    expect(screen.getByLabelText("1 unread")).toBeTruthy();
  });

  // S6-H review 7: a read position can be older than the newest message, so
  // the counts come from the server after a read instead of dropping to 0.
  it("takes the counts from the server after an own read, not after someone else's", async () => {
    apiMock.list.mockResolvedValue([channel("ch-1", "general", 3)]);
    render(() => <ChannelList />);
    await screen.findByLabelText("3 unread");

    ws.emit(readEvent("u-bob"));
    await new Promise((resolve) => setTimeout(resolve, 400));
    expect(apiMock.list).toHaveBeenCalledTimes(1);

    apiMock.list.mockResolvedValue([channel("ch-1", "general", 1)]);
    ws.emit(readEvent("u-me"));
    ws.emit(readEvent("u-me"));
    await screen.findByLabelText("1 unread");
    expect(apiMock.list).toHaveBeenCalledTimes(2);
  });

  it("refetches the counts after a read the server does not track", async () => {
    apiMock.list.mockResolvedValue([channel("ch-1", "general", 2)]);
    render(() => <ChannelList />);
    await screen.findByLabelText("2 unread");

    apiMock.list.mockResolvedValue([channel("ch-1", "general", 0)]);
    noteUntrackedRead();
    await waitFor(() => expect(screen.queryByLabelText("2 unread")).toBeNull());
  });

  it("counts messages of a channel that is open but not followed", async () => {
    apiMock.list.mockResolvedValue([channel("ch-1", "general", 0)]);
    render(() => <ChannelList />);
    await screen.findByText("general");

    setFollowedChannel("ch-1");
    ws.emit(channelMessage({ ...record("m-6", "seen", "", "ch-1"), sender_id: "u-bob" }));
    expect(screen.queryByLabelText("1 unread")).toBeNull();

    setFollowedChannel(undefined);
    ws.emit(channelMessage({ ...record("m-7", "below", "", "ch-1"), sender_id: "u-bob" }));
    await screen.findByLabelText("1 unread");
  });
});

// S6-H review 7: messages that arrive while the reader is scrolled up stay
// unread until the reader reaches the bottom.
describe("ChannelView following", () => {
  it("marks messages read only once the reader reaches the bottom", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    try {
      apiMock.messages.mockResolvedValue([record("m-1", "first message")]);
      render(() => <ChannelView />);
      await screen.findByText("first message");
      await vi.advanceTimersByTimeAsync(1100);
      expect(apiMock.markRead).toHaveBeenLastCalledWith("ch-1", "m-1");
      expect(followedChannel()).toBe("ch-1");

      scrollMessages(0);
      expect(followedChannel()).toBeUndefined();
      ws.emit(channelMessage(record("m-2", "while scrolled up")));
      await screen.findByText("while scrolled up");
      await vi.advanceTimersByTimeAsync(1100);
      expect(apiMock.markRead).toHaveBeenCalledTimes(1);

      scrollMessages(1500);
      expect(followedChannel()).toBe("ch-1");
      await vi.advanceTimersByTimeAsync(1100);
      expect(apiMock.markRead).toHaveBeenLastCalledWith("ch-1", "m-2");
    } finally {
      vi.useRealTimers();
    }
  });

  it("stops following when it closes", async () => {
    const { unmount } = render(() => <ChannelView />);
    await screen.findByText("first message");
    expect(followedChannel()).toBe("ch-1");
    unmount();
    expect(followedChannel()).toBeUndefined();
  });
});

// S6-H review 8: a refused post or reply is shown, not an unhandled
// rejection; viewers get no input.
describe("posting and replying", () => {
  const parent = {
    id: "m-1",
    sender_type: "user",
    sender_name: "alice",
    content: "parent",
    created_at: "2026-09-30T10:00:00Z",
  };

  it("shows a refused thread reply and keeps its text", async () => {
    auth.editor = true;
    apiMock.sendThreadReply.mockRejectedValue(new Error("insufficient permissions"));
    render(() => (
      <ThreadPanel channelId="ch-1" parentMessage={parent} visible onClose={() => undefined} />
    ));
    const input = await screen.findByPlaceholderText("Reply...");
    fireEvent.input(input, { target: { value: "my reply" } });
    fireEvent.click(screen.getByText("Send"));

    expect((await screen.findByRole("alert")).textContent).toContain("insufficient permissions");
    expect((input as HTMLInputElement).value).toBe("my reply");
  });

  it("offers viewers no reply input", async () => {
    render(() => (
      <ThreadPanel channelId="ch-1" parentMessage={parent} visible onClose={() => undefined} />
    ));
    await screen.findByText("Only editors and admins can reply.");
    expect(screen.queryByPlaceholderText("Reply...")).toBeNull();
  });

  it("shows a refused channel message", async () => {
    auth.editor = true;
    apiMock.send.mockRejectedValue(new Error("insufficient permissions"));
    render(() => <ChannelView />);
    const input = await screen.findByLabelText("Channel message");
    fireEvent.input(input, { target: { value: "hello" } });
    fireEvent.click(screen.getByLabelText("Send message"));

    expect((await screen.findByRole("alert")).textContent).toContain("insufficient permissions");
  });

  it("offers viewers no message input", async () => {
    render(() => <ChannelView />);
    await screen.findByText("Only editors and admins can post in channels.");
    expect(screen.queryByLabelText("Channel message")).toBeNull();
  });
});
