import { describe, expect, it } from "vitest";

import type { AGUIPermissionRequest } from "~/api/websocket";

import { mergePermissionRequests, mergeStreamedText } from "./chatRunRestore";

function request(callId: string): AGUIPermissionRequest {
  return { run_id: "conv-1", call_id: callId, tool: "bash" };
}

// KI-148: after a reload the chat shows the running turn the Core reports,
// merged with what arrived over the WebSocket in the meantime.
describe("mergePermissionRequests", () => {
  it("adds the restored approvals once", () => {
    const shown = [request("c1")];
    const merged = mergePermissionRequests(shown, [request("c1"), request("c2")]);
    expect(merged.map((pr) => pr.call_id)).toEqual(["c1", "c2"]);
  });

  it("keeps the cards already shown", () => {
    expect(mergePermissionRequests([request("c1")], [])).toEqual([request("c1")]);
    expect(mergePermissionRequests([], [])).toEqual([]);
  });
});

describe("mergeStreamedText", () => {
  it("takes the Core's text when nothing arrived since the request", () => {
    expect(mergeStreamedText("Hello world", "")).toBe("Hello world");
  });

  it("appends text that arrived after the Core answered", () => {
    expect(mergeStreamedText("Hello ", "world")).toBe("Hello world");
  });

  it("does not repeat text the Core already included", () => {
    expect(mergeStreamedText("Hello wor", "world!")).toBe("Hello world!");
    expect(mergeStreamedText("Hello world", "world")).toBe("Hello world");
  });

  it("keeps the live text when the Core streamed nothing", () => {
    expect(mergeStreamedText("", "live")).toBe("live");
  });
});
