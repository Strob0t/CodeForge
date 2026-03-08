import { describe, it, expect } from "vitest";
import { cx } from "./cx";

describe("cx", () => {
  it("joins truthy strings", () => {
    expect(cx("a", "b", "c")).toBe("a b c");
  });
  it("filters false/undefined/empty", () => {
    expect(cx("a", false, undefined, "", "b")).toBe("a b");
  });
  it("filters null", () => {
    expect(cx("a", null, "b")).toBe("a b");
  });
  it("returns empty for no args", () => {
    expect(cx()).toBe("");
  });
});
