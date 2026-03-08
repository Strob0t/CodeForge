import { describe, expect, it } from "vitest";

import { getErrorMessage } from "./getErrorMessage";

describe("getErrorMessage", () => {
  it("extracts Error.message", () => {
    expect(getErrorMessage(new Error("boom"), "fallback")).toBe("boom");
  });
  it("passes through strings", () => {
    expect(getErrorMessage("oops", "fallback")).toBe("oops");
  });
  it("returns fallback for unknown", () => {
    expect(getErrorMessage(42, "fallback")).toBe("fallback");
  });
  it("returns fallback for null", () => {
    expect(getErrorMessage(null, "fallback")).toBe("fallback");
  });
});
