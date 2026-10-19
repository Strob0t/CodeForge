import { describe, expect, it } from "vitest";

import {
  carriesRedacted,
  keepsStoredSecrets,
  type MCPEndpoint,
  type MCPSecretsView,
} from "./mcpSecrets";

/** A stdio server as read: credential argument, env token, empty variable, header. */
const read: MCPSecretsView = {
  endpoint: { transport: "stdio", url: "", command: "mcp-github", args: ["--token", "***", "-v"] },
  env: { GITHUB_TOKEN: "***", EMPTY: "" },
  headers: { Authorization: "***" },
};

interface EditableView {
  endpoint: MCPEndpoint;
  env: Record<string, string>;
  headers: Record<string, string>;
}

function edited(change: (view: EditableView) => void): MCPSecretsView {
  const view: EditableView = {
    endpoint: { ...read.endpoint, args: [...read.endpoint.args] },
    env: { ...read.env },
    headers: { ...read.headers },
  };
  change(view);
  return view;
}

// KI-98: the same rule as the Go Core's mcp.ServerDef.KeepRedacted.
describe("keepsStoredSecrets", () => {
  it.each<[string, MCPSecretsView, boolean]>([
    ["the server as read", edited(() => undefined), true],
    ["another transport", edited((v) => (v.endpoint = { ...v.endpoint, transport: "sse" })), false],
    [
      "another url",
      edited((v) => (v.endpoint = { ...v.endpoint, url: "https://x.example" })),
      false,
    ],
    ["another command", edited((v) => (v.endpoint = { ...v.endpoint, command: "mcp-x" })), false],
    [
      "an added argument",
      edited((v) => (v.endpoint = { ...v.endpoint, args: [...v.endpoint.args, "-q"] })),
      false,
    ],
    [
      "reordered arguments",
      edited((v) => (v.endpoint = { ...v.endpoint, args: ["-v", "--token", "***"] })),
      false,
    ],
    // S7-G review: the env and the headers decide where a kept secret goes too.
    [
      "an added env variable",
      edited((v) => (v.env.GITHUB_API_URL = "https://evil.example")),
      false,
    ],
    ["an added proxy", edited((v) => (v.env.HTTPS_PROXY = "http://evil.example:3128")), false],
    ["a changed env value", edited((v) => (v.env.EMPTY = "x")), false],
    ["a removed env variable", edited((v) => delete v.env.EMPTY), false],
    ["a changed header", edited((v) => (v.headers.Authorization = "Bearer new")), false],
    ["an added header", edited((v) => (v.headers["X-Forward-To"] = "evil")), false],
    ["a removed header", edited((v) => delete v.headers.Authorization), false],
    // S7-G review: a "***" stands only for a value read as "***" under the
    // same key; the Go Core has nothing stored for another key.
    ["*** under a key read as empty", edited((v) => (v.env.EMPTY = "***")), false],
    [
      "a renamed header that keeps ***",
      edited((v) => {
        delete v.headers.Authorization;
        v.headers["X-Api-Key"] = "***";
      }),
      false,
    ],
    [
      "a renamed env variable that keeps ***",
      edited((v) => {
        delete v.env.GITHUB_TOKEN;
        v.env.GH_TOKEN = "***";
      }),
      false,
    ],
  ])("for %s", (_name, current, keeps) => {
    expect(keepsStoredSecrets(read, current)).toBe(keeps);
  });

  it("keeps nothing for a new server, which has no stored values", () => {
    expect(keepsStoredSecrets(null, read)).toBe(false);
  });
});

describe("carriesRedacted", () => {
  const plain: MCPSecretsView = {
    endpoint: { transport: "sse", url: "https://x.example/sse", command: "", args: [] },
    env: { A: "a", B: "" },
    headers: {},
  };

  it.each<[string, MCPSecretsView, boolean]>([
    ["nothing", plain, false],
    ["an env value", { ...plain, env: { A: "***" } }, true],
    ["a header value", { ...plain, headers: { Authorization: "***" } }, true],
    ["a value that only contains ***", { ...plain, env: { A: "x***y" } }, false],
    [
      "the url's credentials",
      { ...plain, endpoint: { ...plain.endpoint, url: "https://***@x.example/sse" } },
      true,
    ],
    [
      "a credential argument",
      { ...plain, endpoint: { ...plain.endpoint, args: ["--token=***"] } },
      true,
    ],
  ])("finds %s", (_name, view, carries) => {
    expect(carriesRedacted(view)).toBe(carries);
  });
});
