import { describe, expect, it } from "vitest";

import type { AGUIPermissionRequest } from "~/api/websocket";

import { addPermissionRequest, mergePermissionRequests, mergeStreamedText } from "./chatRunRestore";

function request(callId: string, runId = "conv-1"): AGUIPermissionRequest {
  return { run_id: runId, call_id: callId, tool: "bash" };
}

// KI-148: after a reload the chat shows the running turn the Core reports,
// merged with what arrived over the WebSocket in the meantime.
describe("mergePermissionRequests", () => {
  it("adds the restored approvals once", () => {
    const shown = [request("c1")];
    const merged = mergePermissionRequests(shown, [request("c1"), request("c2")]);
    expect(merged.map((pr) => pr.call_id)).toEqual(["c1", "c2"]);
  });

  it("tells approvals apart by run and call", () => {
    const merged = mergePermissionRequests([request("c1")], [request("c1", "conv-2")]);
    expect(merged.map((pr) => `${pr.run_id}/${pr.call_id}`)).toEqual(["conv-1/c1", "conv-2/c1"]);
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

// KI-148 review: the live permission_request event can arrive after the
// restore already showed its card.
describe("addPermissionRequest", () => {
  it("adds a new approval", () => {
    expect(addPermissionRequest([request("c1")], request("c2")).map((pr) => pr.call_id)).toEqual([
      "c1",
      "c2",
    ]);
  });

  it("does not add an approval that is shown already", () => {
    const shown = [request("c1")];
    expect(addPermissionRequest(shown, request("c1"))).toBe(shown);
  });

  it("adds the same call ID of another run", () => {
    expect(addPermissionRequest([request("c1")], request("c1", "conv-2"))).toHaveLength(2);
  });
});
