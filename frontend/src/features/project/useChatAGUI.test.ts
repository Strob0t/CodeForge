import { describe, expect, it } from "vitest";

describe("useChatAGUI", () => {
  it("should export useChatAGUI function", async () => {
    const mod = await import("./useChatAGUI");
    expect(typeof mod.useChatAGUI).toBe("function");
  });

  it("should be the only named export", async () => {
    const mod = await import("./useChatAGUI");
    const exports = Object.keys(mod);
    expect(exports).toContain("useChatAGUI");
    expect(exports).toHaveLength(1);
  });
});
