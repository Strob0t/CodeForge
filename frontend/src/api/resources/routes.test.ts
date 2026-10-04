import { describe, expect, it } from "vitest";

import type { CoreClient } from "../core";
import { createLLMResource } from "./llm";
import { createMCPResource } from "./misc";

interface Call {
  method: "GET" | "POST" | "PUT" | "PATCH" | "DELETE";
  path: string;
  body?: unknown;
}

/** A CoreClient that records every request instead of sending it. */
function recordingClient(): { client: CoreClient; calls: Call[] } {
  const calls: Call[] = [];
  const record =
    (method: Call["method"]) =>
    <T>(path: string, body?: unknown): Promise<T> => {
      calls.push(body === undefined ? { method, path } : { method, path, body });
      return Promise.resolve(undefined as T);
    };
  const client: CoreClient = {
    request: <T>(path: string) => record("GET")<T>(path),
    get: record("GET"),
    post: record("POST"),
    put: record("PUT"),
    patch: record("PATCH"),
    del: record("DELETE"),
    BASE: "/api/v1",
    invalidateCache: () => undefined,
  };
  return { client, calls };
}

// KI-40: every client method must hit a route the Go router registers
// (internal/adapter/http/routes.go).
describe("API routes", () => {
  it.each([
    ["model-123", "/llm/models/model-123"],
    ["openai/gpt-4o", "/llm/models/openai%2Fgpt-4o"],
    ["50%off", "/llm/models/50%25off"],
  ])("deletes model %s with DELETE /llm/models/{id}", async (id, path) => {
    const { client, calls } = recordingClient();
    await createLLMResource(client).deleteModel(id);
    expect(calls).toEqual([{ method: "DELETE", path }]);
  });

  it("lists, assigns and unassigns project MCP servers", async () => {
    const { client, calls } = recordingClient();
    const mcp = createMCPResource(client);

    await mcp.listProjectServers("p 1");
    await mcp.assignToProject("p 1", "srv-1");
    await mcp.unassignFromProject("p 1", "srv/1");

    expect(calls).toEqual([
      { method: "GET", path: "/projects/p%201/mcp-servers" },
      { method: "POST", path: "/projects/p%201/mcp-servers", body: { server_id: "srv-1" } },
      { method: "DELETE", path: "/projects/p%201/mcp-servers/srv%2F1" },
    ]);
  });
});
