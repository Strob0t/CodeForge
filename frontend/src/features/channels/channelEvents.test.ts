import { describe, expect, it } from "vitest";

import type { ChannelMessageRecord } from "~/api/types";
import type { WSMessage } from "~/api/websocket";

import {
  addMessage,
  applyUnreadEvent,
  parseChannelMessageEvent,
  parseChannelReadEvent,
  replyCounts,
} from "./channelEvents";

const stored: ChannelMessageRecord = {
  id: "m-1",
  channel_id: "ch-1",
  sender_type: "user",
  sender_name: "alice",
  content: "hi",
  parent_id: "",
  created_at: "2026-09-30T10:00:00Z",
};

// The payload of Go event.ChannelMessageEvent; Go omits an empty parent_id.
function channelEvent(message: unknown, channelId = "ch-1"): WSMessage {
  return { type: "channel.message", payload: { channel_id: channelId, message } };
}

describe("parseChannelMessageEvent", () => {
  it("returns the message of a channel.message event", () => {
    const wire: Record<string, unknown> = { ...stored };
    delete wire.parent_id;
    expect(parseChannelMessageEvent(channelEvent(wire))).toEqual(stored);
  });

  it("keeps the parent of a thread reply", () => {
    const reply = { ...stored, id: "m-2", parent_id: "m-1" };
    expect(parseChannelMessageEvent(channelEvent(reply))?.parent_id).toBe("m-1");
  });

  it.each<[string, WSMessage]>([
    ["another event type", { type: "channel.typing", payload: { channel_id: "ch-1" } }],
    ["no message", { type: "channel.message", payload: { channel_id: "ch-1" } }],
    ["message is not an object", channelEvent("hi")],
    ["no channel id", { type: "channel.message", payload: { message: stored } }],
    ["message without id", channelEvent({ ...stored, id: undefined })],
    ["message without content", channelEvent({ ...stored, content: 42 })],
    ["message of another channel than the event", channelEvent(stored, "ch-2")],
  ])("ignores %s", (_name, msg) => {
    expect(parseChannelMessageEvent(msg)).toBeNull();
  });
});

describe("addMessage", () => {
  const a = { id: "a" };
  const b = { id: "b" };

  it("adds to the start of a newest-first list", () => {
    expect(addMessage([a], b, "start")).toEqual([b, a]);
  });

  it("adds to the end of a chronological list", () => {
    expect(addMessage([a], b, "end")).toEqual([a, b]);
  });

  it("adds to an empty or missing list", () => {
    expect(addMessage([], a, "end")).toEqual([a]);
    expect(addMessage(undefined, a, "start")).toEqual([a]);
  });

  it("does not add a message twice (the sender also receives its own event)", () => {
    const list = [a, b];
    expect(addMessage(list, { id: "a" }, "start")).toBe(list);
  });
});

// KI-73: read positions and unread counts.
describe("parseChannelReadEvent", () => {
  it("returns the read position of a channel.read event", () => {
    const msg: WSMessage = {
      type: "channel.read",
      payload: { channel_id: "ch-1", user_id: "u-1", message_id: "m-9", last_read_at: "t" },
    };
    expect(parseChannelReadEvent(msg)).toEqual({
      channel_id: "ch-1",
      user_id: "u-1",
      last_read_message_id: "m-9",
      last_read_at: "t",
    });
  });

  it.each<[string, WSMessage]>([
    ["another event type", channelEvent(stored)],
    ["no user", { type: "channel.read", payload: { channel_id: "ch-1", message_id: "m" } }],
    ["no message", { type: "channel.read", payload: { channel_id: "ch-1", user_id: "u" } }],
  ])("ignores %s", (_name, msg) => {
    expect(parseChannelReadEvent(msg)).toBeNull();
  });
});

describe("replyCounts", () => {
  it("counts the replies per parent", () => {
    const counts = replyCounts([
      { parent_id: "" },
      { parent_id: "m-1" },
      { parent_id: "m-1" },
      { parent_id: "m-2" },
    ]);
    expect(counts.get("m-1")).toBe(2);
    expect(counts.get("m-2")).toBe(1);
    expect(counts.has("")).toBe(false);
  });
});

describe("applyUnreadEvent", () => {
  const fromBob = { ...stored, sender_id: "u-bob" };
  const read = (userId: string): WSMessage => ({
    type: "channel.read",
    payload: { channel_id: "ch-1", user_id: userId, message_id: "m-1" },
  });

  it("counts a top-level message of someone else in a channel that is not open", () => {
    expect(applyUnreadEvent({ "ch-1": 2 }, channelEvent(fromBob), "u-me", "ch-9")).toEqual({
      "ch-1": 3,
    });
    const webhook = channelEvent({ ...fromBob, sender_id: undefined });
    expect(applyUnreadEvent({}, webhook, "u-me", undefined)).toEqual({ "ch-1": 1 });
  });

  it.each<[string, WSMessage, string | undefined]>([
    ["own message", channelEvent({ ...stored, sender_id: "u-me" }), undefined],
    ["thread reply", channelEvent({ ...fromBob, parent_id: "m-0" }), undefined],
    ["message in the open channel", channelEvent(fromBob), "ch-1"],
    ["event of another type", { type: "task.output", payload: { line: "x" } }, undefined],
    ["read position of someone else", read("u-bob"), undefined],
  ])("does not count an %s and keeps the same counts object", (_name, msg, open) => {
    const counts = { "ch-1": 2 };
    expect(applyUnreadEvent(counts, msg, "u-me", open)).toBe(counts);
  });

  it("clears the count when the user read the channel, not when someone else did", () => {
    expect(applyUnreadEvent({ "ch-1": 4 }, read("u-me"), "u-me", undefined)).toEqual({
      "ch-1": 0,
    });
    expect(applyUnreadEvent({ "ch-1": 4 }, read("u-bob"), "u-me", undefined)).toEqual({
      "ch-1": 4,
    });
  });
});
