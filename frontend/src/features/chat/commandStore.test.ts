import { describe, expect, it } from "vitest";

import { useCommandStore } from "./commandStore";

describe("commandStore", () => {
  // FIX-067: Verify commandStore exports and basic API shape.

  it("should export useCommandStore function", () => {
    expect(typeof useCommandStore).toBe("function");
  });

  it("should return an object with commands accessor", () => {
    // useCommandStore uses SolidJS createResource internally,
    // but the return shape should have a commands property.
    const store = useCommandStore();
    expect(store).toHaveProperty("commands");
    expect(typeof store.commands).toBe("function");
  });

  it("should return fallback commands when no backend available", () => {
    const store = useCommandStore();
    const commands = store.commands();
    // Fallback commands include compact, clear, help, mode, model, rewind
    expect(Array.isArray(commands)).toBe(true);
    expect(commands.length).toBeGreaterThan(0);
    const ids = commands.map((c) => c.id);
    expect(ids).toContain("compact");
    expect(ids).toContain("help");
  });
});
