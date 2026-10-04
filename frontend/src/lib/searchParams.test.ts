import { describe, expect, it } from "vitest";

import { firstParam } from "./searchParams";

// Links open a settings section (?section=privacy) or a conversation in its
// project's chat (?conversation=<id>, the search page's conversation hits).
describe("firstParam", () => {
  it.each<[string | string[] | undefined, string | undefined]>([
    ["c-1", "c-1"],
    [["c-1", "c-2"], "c-1"],
    [[], undefined],
    ["", undefined],
    [undefined, undefined],
  ])("reads %j as %j", (value, first) => {
    expect(firstParam(value)).toBe(first);
  });
});
