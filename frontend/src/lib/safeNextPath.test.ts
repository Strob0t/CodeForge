import { describe, expect, it } from "vitest";

import { safeNextPath } from "./safeNextPath";

// After sign-in the app returns to the page the user asked for (e.g. an
// approval link from an email) - only a path of this app, never elsewhere.
describe("safeNextPath", () => {
  it.each([
    ["/approvals/run-1/call-1", "/approvals/run-1/call-1"],
    ["/projects/p1?tab=chat", "/projects/p1?tab=chat"],
  ])("accepts the app path %s", (next, want) => {
    expect(safeNextPath(next)).toBe(want);
  });

  it.each([
    [undefined],
    [""],
    ["https://evil.example/"],
    ["//evil.example/"],
    ["/\\evil.example/"],
    ["javascript:alert(1)"],
    ["approvals/x"],
    ["/login"],
    ["/login?next=/x"],
    ["/a\nb"],
  ])("refuses %s", (next) => {
    expect(safeNextPath(next)).toBeNull();
  });
});
