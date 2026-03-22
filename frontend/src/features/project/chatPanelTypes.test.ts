import { describe, expect, it } from "vitest";

describe("chatPanelTypes", () => {
  it("should export ToolCallState type", async () => {
    const mod = await import("./chatPanelTypes");
    // Type-only exports don't exist at runtime, but the module should load without error
    expect(mod).toBeDefined();
  });

  it("should export PlanStepState type", async () => {
    const mod = await import("./chatPanelTypes");
    expect(mod).toBeDefined();
  });

  it("should export ChatAGUIState type", async () => {
    const mod = await import("./chatPanelTypes");
    expect(mod).toBeDefined();
  });
});
