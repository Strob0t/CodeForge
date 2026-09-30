import { createRoot } from "solid-js";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { CommandInfo } from "~/api/types";

const apiMock = vi.hoisted(() => ({
  list: vi.fn<() => Promise<CommandInfo[]>>(),
}));

vi.mock("~/api/client", () => ({
  api: { commands: { list: apiMock.list } },
}));

import { useCommandStore } from "./commandStore";
import type { Item } from "./fuzzySearch";

/** Resolves once the store's commands differ from the initial fallback, or after a tick. */
async function loadedCommands(): Promise<Item[]> {
  let dispose: () => void = () => undefined;
  const store = createRoot((d) => {
    dispose = d;
    return useCommandStore();
  });
  await new Promise((resolve) => setTimeout(resolve, 10));
  const commands = store.commands();
  dispose();
  return commands;
}

const fetchSpy = vi.fn<typeof fetch>();

beforeEach(() => {
  apiMock.list.mockReset();
  fetchSpy.mockReset();
  vi.stubGlobal("fetch", fetchSpy);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("commandStore", () => {
  // WT-7: the commands come through the authenticated API client, not a bare fetch.
  it("loads the commands through the API client", async () => {
    apiMock.list.mockResolvedValue([
      { id: "cost", label: "cost", category: "command", description: "Show the cost" },
    ]);

    const commands = await loadedCommands();

    expect(apiMock.list).toHaveBeenCalledTimes(1);
    expect(fetchSpy).not.toHaveBeenCalled();
    expect(commands).toEqual([{ id: "cost", label: "cost", category: "command" }]);
  });

  it("falls back to the built-in commands when the request fails", async () => {
    apiMock.list.mockRejectedValue(new Error("401"));

    const ids = (await loadedCommands()).map((c) => c.id);

    expect(ids).toContain("compact");
    expect(ids).toContain("help");
  });

  it("falls back to the built-in commands when the backend has none", async () => {
    apiMock.list.mockResolvedValue([]);

    const ids = (await loadedCommands()).map((c) => c.id);

    expect(ids).toContain("rewind");
  });
});
