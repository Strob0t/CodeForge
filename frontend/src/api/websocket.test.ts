import { describe, expect, it } from "vitest";

import { parseWSMessage } from "./websocket";

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
