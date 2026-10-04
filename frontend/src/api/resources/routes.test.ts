import { describe, expect, it } from "vitest";

import type { CoreClient } from "../core";
import { createConversationsResource } from "./conversations";
import { createLLMResource } from "./llm";
import { createMCPResource } from "./misc";
import { createPrivacyResource } from "./privacy";
import { createRoadmapResource } from "./roadmap";

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

// KI-121: the roadmap UI calls the bidirectional sync next to Import from PM.
describe("roadmap sync route", () => {
  it("syncs with POST /projects/{id}/roadmap/sync", async () => {
    const { client, calls } = recordingClient();
    const body = {
      provider: "github-issues",
      project_ref: "owner/repo",
      direction: "bidi" as const,
      dry_run: true,
      create_new: true,
      update_exist: false,
    };
    await createRoadmapResource(client).sync("p 1", body);
    expect(calls).toEqual([{ method: "POST", path: "/projects/p%201/roadmap/sync", body }]);
  });
});

// KI-121: a conversation hit of the search page opens in its project's chat.
describe("conversation routes", () => {
  it("gets a conversation with GET /conversations/{id}", async () => {
    const { client, calls } = recordingClient();
    await createConversationsResource(client).get("c/1");
    expect(calls).toEqual([{ method: "GET", path: "/conversations/c%2F1" }]);
  });
});

// KI-93: the Settings > Privacy screen uses the GDPR self-service and consent
// routes (internal/adapter/http/routes.go, "GDPR self-service").
describe("privacy routes", () => {
  it("exports, deletes and manages consent with the /me routes", async () => {
    const { client, calls } = recordingClient();
    const privacy = createPrivacyResource(client);

    await privacy.exportMyData();
    await privacy.deleteMyData();
    await privacy.consentPurposes();
    await privacy.consentStatus();
    await privacy.setConsent("llm/external processing", false);

    expect(calls).toEqual([
      { method: "GET", path: "/me/export" },
      { method: "DELETE", path: "/me/data" },
      { method: "GET", path: "/me/consent/purposes" },
      { method: "GET", path: "/me/consent" },
      {
        method: "PUT",
        path: "/me/consent/llm%2Fexternal%20processing",
        body: { granted: false },
      },
    ]);
  });

  it("keeps no copy of the export in the response cache", async () => {
    const { client } = recordingClient();
    const invalidated: string[] = [];
    client.invalidateCache = (prefix) => void invalidated.push(prefix);

    await createPrivacyResource(client).exportMyData();

    expect(invalidated).toEqual(["/me/export"]);
  });

  it("keeps no copy of the export when the request fails", async () => {
    const { client } = recordingClient();
    const invalidated: string[] = [];
    client.invalidateCache = (prefix) => void invalidated.push(prefix);
    client.get = <T>(): Promise<T> => Promise.reject(new Error("offline"));

    await expect(createPrivacyResource(client).exportMyData()).rejects.toThrow("offline");
    expect(invalidated).toEqual(["/me/export"]);
  });
});
