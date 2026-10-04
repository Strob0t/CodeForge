import { describe, expect, it } from "vitest";

import { carriesRedacted, keepsStoredSecrets, type MCPEndpoint } from "./mcpSecrets";

const read: MCPEndpoint = {
  transport: "stdio",
  url: "",
  command: "mcp-github",
  args: ["--token", "***", "-v"],
};

// KI-98: the same rule as the Go Core's mcp.ServerDef.KeepRedacted.
describe("keepsStoredSecrets", () => {
  it.each<[string, MCPEndpoint, boolean]>([
    ["the endpoint as read", { ...read, args: [...read.args] }, true],
    ["another transport", { ...read, transport: "sse" }, false],
    ["another url", { ...read, url: "https://x.example/sse" }, false],
    ["another command", { ...read, command: "mcp-gitlab" }, false],
    ["an added argument", { ...read, args: [...read.args, "-q"] }, false],
    ["a removed argument", { ...read, args: ["--token", "***"] }, false],
    ["reordered arguments", { ...read, args: ["-v", "--token", "***"] }, false],
    ["no arguments", { ...read, args: [] }, false],
  ])("for %s: %s", (_name, current, keeps) => {
    expect(keepsStoredSecrets(read, current)).toBe(keeps);
  });

  it("keeps nothing for a new server, which has no stored values", () => {
    expect(keepsStoredSecrets(null, read)).toBe(false);
  });
});

describe("carriesRedacted", () => {
  const plain: MCPEndpoint = {
    transport: "sse",
    url: "https://x.example/sse",
    command: "",
    args: [],
  };

  it.each<[string, MCPEndpoint, string[], boolean]>([
    ["nothing", plain, ["a", ""], false],
    ["an env or header value", plain, ["a", "***"], true],
    ["a value that only contains ***", plain, ["x***y"], false],
    ["the url's credentials", { ...plain, url: "https://***@x.example/sse" }, [], true],
    ["a credential argument", { ...plain, args: ["--token=***"] }, [], true],
    ["no values at all", plain, [], false],
  ])("finds %s", (_name, endpoint, values, carries) => {
    expect(carriesRedacted(endpoint, values)).toBe(carries);
  });
});
