import { describe, expect, it } from "vitest";

import indexSource from "../index.tsx?raw";
import paletteSource from "./CommandPalette.tsx?raw";

/** The literal paths of the router's routes (src/index.tsx). */
function routedPaths(): string[] {
  return [...indexSource.matchAll(/<Route\s+path="([^"]+)"/g)].map((m) => m[1]);
}

// The palette's "models" command navigated to /models, which is not a route
// (the models live on the AI page, /ai).
describe("command palette", () => {
  it("navigates only to routed pages", () => {
    const targets = [...paletteSource.matchAll(/navigate\("([^"?]+)/g)].map((m) => m[1]);
    expect(targets.length).toBeGreaterThan(0);
    expect(targets.filter((p) => !routedPaths().includes(p))).toEqual([]);
  });
});
