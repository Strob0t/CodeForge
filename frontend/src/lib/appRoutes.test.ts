import { describe, expect, it } from "vitest";

import indexSource from "../index.tsx?raw";
import { isShellPath } from "./appRoutes";

/** Pages that render without the app shell and route guard. */
const PUBLIC_PATHS = [
  "/login",
  "/change-password",
  "/setup",
  "/forgot-password",
  "/reset-password",
  "/privacy",
];

/** The paths of the router's routes (src/index.tsx), with sample values for their parameters. */
function routedPaths(): string[] {
  const paths = [...indexSource.matchAll(/<Route\s+path="([^"]+)"/g)].map((m) => m[1]);
  return paths
    .filter((p) => !p.startsWith("*"))
    .map((p) => p.replace(/:[A-Za-z]+/g, (param) => `sample-${param.slice(1)}`));
}

// KI-121: /channels/:id and /design-system were routed but not known to the
// app shell, so they rendered without the sidebar and without the route guard.
describe("app routes", () => {
  it("finds the router's routes", () => {
    expect(routedPaths()).toContain("/channels/sample-id");
    expect(routedPaths()).toEqual(expect.arrayContaining(PUBLIC_PATHS));
  });

  it("renders every routed page that is not public inside the app shell", () => {
    const outside = routedPaths().filter((p) => !PUBLIC_PATHS.includes(p) && !isShellPath(p));
    expect(outside).toEqual([]);
  });

  it.each([
    "/channels/c-1",
    "/design-system",
    "/projects/p-1",
    "/approvals/r/c",
    "/settings",
    "/search",
    // S7-G review: the router matches static segments case-insensitively and
    // ignores empty segments, so these render their page: they must not
    // escape the route guard. Unknown paths are guarded too (fail closed).
    "/SETTINGS",
    "/settings/",
    "/settings//",
    "//settings",
    "/Projects/abc",
    "/CHANNELS/x",
    "/Design-System",
    "/nope",
    "/channels",
    "",
    "/",
  ])("renders %j inside the app shell, behind the route guard", (path) => {
    expect(isShellPath(path)).toBe(true);
  });

  it.each([...PUBLIC_PATHS, "/LOGIN", "/login/", "//Privacy//", "/Reset-Password"])(
    "renders the public page %j on its own",
    (path) => {
      expect(isShellPath(path)).toBe(false);
    },
  );
});
