import { fireEvent, render, screen, waitFor } from "@solidjs/testing-library";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { MCPServer } from "~/api/types";

// KI-71 review: the Go Core never starts a stdio MCP server, so stdio servers
// get no connection test before they are saved (the test would always fail).
// Reads show env and header values as "***"; editing a saved server tests it
// with its id, so the Go Core tests with the stored values.

vi.hoisted(() => {
  Object.defineProperty(window, "matchMedia", {
    writable: true,
    value: (query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: () => undefined,
      removeListener: () => undefined,
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
      dispatchEvent: () => false,
    }),
  });
});

vi.mock("@solidjs/router", () => ({
  A: (props: Record<string, unknown>) => props,
  useNavigate: () => () => undefined,
  useLocation: () => ({ pathname: "/" }),
}));

const mcp = vi.hoisted(() => ({
  servers: [] as MCPServer[],
  listServers: vi.fn(),
  createServer: vi.fn(),
  updateServer: vi.fn(),
  deleteServer: vi.fn(),
  testConnection: vi.fn(),
  testServer: vi.fn(),
  listTools: vi.fn(),
}));

vi.mock("~/api/client", () => ({ api: { mcp } }));

import { ConfirmProvider } from "~/components/ConfirmProvider";
import { ToastProvider } from "~/components/Toast";
import { I18nProvider } from "~/i18n";

import MCPServersPage from "./MCPServersPage";

const remote: MCPServer = {
  id: "s-remote",
  name: "remote",
  description: "",
  transport: "sse",
  command: "",
  args: [],
  url: "http://mcp.example/sse",
  env: { API_TOKEN: "***" },
  headers: { Authorization: "***" },
  enabled: true,
  status: "registered",
};

const local: MCPServer = {
  ...remote,
  id: "s-local",
  name: "local",
  transport: "stdio",
  command: "mcp-local",
  url: "",
};

function renderPage(): void {
  render(() => (
    <I18nProvider>
      <ToastProvider>
        <ConfirmProvider>
          <MCPServersPage />
        </ConfirmProvider>
      </ToastProvider>
    </I18nProvider>
  ));
}

describe("MCP server connection tests", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mcp.servers = [remote, local];
    mcp.listServers.mockImplementation(() => Promise.resolve(mcp.servers));
    mcp.createServer.mockResolvedValue(remote);
    mcp.updateServer.mockResolvedValue(remote);
    mcp.testConnection.mockResolvedValue({ success: true, tools: [] });
    mcp.testServer.mockResolvedValue({ success: true, tools: [] });
  });

  it("tests an edited saved server with its id, so the stored values are used", async () => {
    renderPage();
    fireEvent.click(await screen.findByLabelText("Edit server remote"));
    fireEvent.click(await screen.findByText("Update Server"));

    await waitFor(() => expect(mcp.updateServer).toHaveBeenCalled());
    expect(mcp.testConnection).toHaveBeenCalledWith(
      expect.objectContaining({ id: "s-remote", env: { API_TOKEN: "***" } }),
    );
    // Sent back as read: the Go Core keeps the stored values (the form has no
    // header editor, so the headers would otherwise be dropped).
    expect(mcp.updateServer).toHaveBeenCalledWith(
      "s-remote",
      expect.objectContaining({ env: { API_TOKEN: "***" }, headers: { Authorization: "***" } }),
    );
  });

  it("drops the stored headers when the endpoint changes", async () => {
    renderPage();
    fireEvent.click(await screen.findByLabelText("Edit server remote"));
    fireEvent.input(await screen.findByLabelText(/^Server URL/), {
      target: { value: "http://other.example/sse" },
    });
    fireEvent.click(screen.getByText("Update Server"));

    await waitFor(() => expect(mcp.updateServer).toHaveBeenCalled());
    expect(mcp.updateServer.mock.calls[0][1]).not.toHaveProperty("headers", expect.anything());
  });

  it("tests a new server without an id", async () => {
    renderPage();
    fireEvent.click(await screen.findByText("Add Server"));
    fireEvent.input(screen.getByLabelText(/^Name/), { target: { value: "new" } });
    fireEvent.change(screen.getByLabelText("Transport"), { target: { value: "sse" } });
    fireEvent.input(await screen.findByLabelText(/^Server URL/), {
      target: { value: "http://new.example/sse" },
    });
    fireEvent.click(screen.getByText("Create Server"));

    await waitFor(() => expect(mcp.createServer).toHaveBeenCalled());
    expect(mcp.testConnection.mock.calls[0][0]).not.toHaveProperty("id");
  });

  it("saves a stdio server without a connection test and explains why", async () => {
    renderPage();
    fireEvent.click(await screen.findByText("Add Server"));
    fireEvent.input(screen.getByLabelText(/^Name/), { target: { value: "files" } });
    fireEvent.input(screen.getByLabelText(/^Command/), { target: { value: "mcp-files" } });

    expect(screen.getByText(/stdio servers run in the worker/)).toBeDefined();
    expect(screen.queryByRole("button", { name: "Test" })).toBeNull();
    fireEvent.click(screen.getByText("Create Server"));

    await waitFor(() => expect(mcp.createServer).toHaveBeenCalled());
    expect(mcp.testConnection).not.toHaveBeenCalled();
    expect(screen.queryByText("Connection Test Failed")).toBeNull();
  });

  it("updates a saved stdio server without a connection test", async () => {
    renderPage();
    fireEvent.click(await screen.findByLabelText("Edit server local"));
    fireEvent.click(await screen.findByText("Update Server"));

    await waitFor(() => expect(mcp.updateServer).toHaveBeenCalled());
    expect(mcp.testConnection).not.toHaveBeenCalled();
  });

  it("offers the saved-server test only for sse and streamable_http servers", async () => {
    renderPage();
    await screen.findByLabelText("Edit server local");

    expect(screen.queryByLabelText("Test connection for remote")).not.toBeNull();
    expect(screen.queryByLabelText("Test connection for local")).toBeNull();
  });
});

// KI-97: reads show the url's password and credential argument values as
// "***"; the form sends them back as read, so the Go Core keeps the stored
// values. Secrets belong in env variables, whose values no read shows.
describe("MCP server arguments", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mcp.servers = [
      { ...local, args: ["-y", "mcp-github", "--token=***", "--api-key", "***"] },
      { ...remote, url: "https://***@mcp.example/sse" },
    ];
    mcp.listServers.mockImplementation(() => Promise.resolve(mcp.servers));
    mcp.updateServer.mockResolvedValue(remote);
    mcp.testConnection.mockResolvedValue({ success: true, tools: [] });
  });

  it("tells admins to keep secrets out of the arguments", async () => {
    renderPage();
    fireEvent.click(await screen.findByText("Add Server"));

    expect(
      screen.getByText(
        "Use env variables for secrets; arguments are visible to all users of the tenant.",
      ),
    ).toBeDefined();
  });

  it("sends redacted arguments back as read", async () => {
    renderPage();
    fireEvent.click(await screen.findByLabelText("Edit server local"));
    fireEvent.click(await screen.findByText("Update Server"));

    await waitFor(() => expect(mcp.updateServer).toHaveBeenCalled());
    expect(mcp.updateServer).toHaveBeenCalledWith(
      "s-local",
      expect.objectContaining({ args: ["-y", "mcp-github", "--token=***", "--api-key", "***"] }),
    );
  });

  it("sends a redacted url password back as read", async () => {
    renderPage();
    fireEvent.click(await screen.findByLabelText("Edit server remote"));
    fireEvent.click(await screen.findByText("Update Server"));

    await waitFor(() => expect(mcp.updateServer).toHaveBeenCalled());
    expect(mcp.updateServer).toHaveBeenCalledWith(
      "s-remote",
      expect.objectContaining({ url: "https://***@mcp.example/sse" }),
    );
  });
});
