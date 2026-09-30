import { describe, expect, it } from "vitest";

import type { ChannelMessageRecord } from "~/api/types";
import type { WSMessage } from "~/api/websocket";

import { addMessage, parseChannelMessageEvent } from "./channelEvents";

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
