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

/** The signed-in user's role (the auth context); the Go Core stays the authority. */
const auth = vi.hoisted(() => ({ role: "admin" as "admin" | "editor" | "viewer" }));

vi.mock("~/components/AuthProvider", () => ({
  useAuth: () => ({
    hasRole: (...roles: string[]) => roles.includes(auth.role),
  }),
}));

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
    auth.role = "admin";
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
    // Sent back as read: the Go Core keeps the stored values.
    expect(mcp.updateServer).toHaveBeenCalledWith(
      "s-remote",
      expect.objectContaining({ env: { API_TOKEN: "***" }, headers: { Authorization: "***" } }),
    );
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
    auth.role = "admin";
    mcp.servers = [
      { ...local, args: ["-y", "mcp-github", "--token=***", "--api-key", "***"] },
      { ...remote, url: "https://***@mcp.example/sse" },
    ];
    mcp.listServers.mockImplementation(() => Promise.resolve(mcp.servers));
    mcp.updateServer.mockResolvedValue(remote);
    mcp.testConnection.mockResolvedValue({ success: true, tools: [] });
  });

  it("suggests a public https url, not a loopback one the server refuses", async () => {
    renderPage();
    fireEvent.click(await screen.findByText("Add Server"));
    fireEvent.change(screen.getByLabelText("Transport"), { target: { value: "sse" } });

    expect(await screen.findByPlaceholderText("https://mcp.example.com/sse")).toBeDefined();
    expect(screen.queryByPlaceholderText(/localhost/)).toBeNull();
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

  it("asks for the url's secret again when the url changes", async () => {
    renderPage();
    fireEvent.click(await screen.findByLabelText("Edit server remote"));
    fireEvent.input(await screen.findByLabelText(/^Server URL/), {
      target: { value: "https://***@other.example/sse" },
    });
    fireEvent.click(screen.getByText("Update Server"));

    expect((await screen.findAllByText(STORED_NOT_KEPT)).length).toBeGreaterThan(0);
    expect(mcp.updateServer).not.toHaveBeenCalled();
  });
});

const STORED_UNCHANGED = "Stored, unchanged: *** keeps the saved value.";
const STORED_NOT_KEPT =
  "Enter it again: a stored value is kept only while transport, URL, command and arguments are unchanged.";

// KI-98: the form had no header editor (an edit sent the headers back as read
// and dropped them silently when the endpoint changed) and no hint for "***".
// The Go Core keeps the value a "***" stands for only while transport, url,
// command and args are the ones it was read with (mcp.ServerDef.KeepRedacted).
describe("MCP header editor", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    auth.role = "admin";
    mcp.servers = [remote, local];
    mcp.listServers.mockImplementation(() => Promise.resolve(mcp.servers));
    mcp.createServer.mockResolvedValue(remote);
    mcp.updateServer.mockResolvedValue(remote);
    mcp.testConnection.mockResolvedValue({ success: true, tools: [] });
  });

  async function editRemote(): Promise<void> {
    renderPage();
    fireEvent.click(await screen.findByLabelText("Edit server remote"));
    await screen.findByLabelText("Header name 1");
  }

  it("shows the stored headers with a stored, unchanged hint", async () => {
    await editRemote();

    expect((screen.getByLabelText("Header name 1") as HTMLInputElement).value).toBe(
      "Authorization",
    );
    expect((screen.getByLabelText("Header value 1") as HTMLInputElement).value).toBe("***");
    // One hint for the header, one for the env variable.
    expect(screen.getAllByText(STORED_UNCHANGED)).toHaveLength(2);
  });

  it("asks for the stored values again when the url changes, and saves nothing", async () => {
    await editRemote();
    fireEvent.input(screen.getByLabelText(/^Server URL/), {
      target: { value: "http://other.example/sse" },
    });

    expect(screen.getAllByText(STORED_NOT_KEPT)).toHaveLength(2);
    expect(screen.queryByText(STORED_UNCHANGED)).toBeNull();

    fireEvent.click(screen.getByText("Update Server"));
    await screen.findByText(
      "Stored secrets (***) are kept only while transport, URL, command and arguments are unchanged. Enter them again.",
    );
    expect(mcp.testConnection).not.toHaveBeenCalled();
    expect(mcp.updateServer).not.toHaveBeenCalled();
  });

  it("saves with a changed url once the values are entered again", async () => {
    await editRemote();
    fireEvent.input(screen.getByLabelText(/^Server URL/), {
      target: { value: "http://other.example/sse" },
    });
    fireEvent.input(screen.getByLabelText("Header value 1"), {
      target: { value: "Bearer new" },
    });
    fireEvent.click(screen.getByLabelText("Remove variable 1"));
    fireEvent.click(screen.getByText("Update Server"));

    await waitFor(() => expect(mcp.updateServer).toHaveBeenCalled());
    const req = mcp.updateServer.mock.calls[0][1];
    expect(req).toMatchObject({
      url: "http://other.example/sse",
      headers: { Authorization: "Bearer new" },
    });
    expect(req).not.toHaveProperty("env", expect.anything());
  });

  it("refuses to keep stored values when the transport changes", async () => {
    await editRemote();
    fireEvent.change(screen.getByLabelText("Transport"), {
      target: { value: "streamable_http" },
    });
    fireEvent.click(screen.getByText("Update Server"));

    expect((await screen.findAllByText(STORED_NOT_KEPT)).length).toBeGreaterThan(0);
    expect(mcp.updateServer).not.toHaveBeenCalled();
  });

  it("refuses to keep a stored env value when a stdio server's arguments change", async () => {
    renderPage();
    fireEvent.click(await screen.findByLabelText("Edit server local"));
    fireEvent.input(await screen.findByLabelText(/^Arguments/), {
      target: { value: "--verbose" },
    });

    // The env token and the stored header (shown: the Go Core keeps a stdio
    // server's headers only when they are sent back).
    expect(screen.getAllByText(STORED_NOT_KEPT)).toHaveLength(2);
    fireEvent.click(screen.getByText("Update Server"));
    await screen.findByText(/Stored secrets \(\*\*\*\) are kept only/);
    expect(mcp.updateServer).not.toHaveBeenCalled();
  });

  it("sends a stdio server's stored headers back as read", async () => {
    renderPage();
    fireEvent.click(await screen.findByLabelText("Edit server local"));
    fireEvent.click(await screen.findByText("Update Server"));

    await waitFor(() => expect(mcp.updateServer).toHaveBeenCalled());
    expect(mcp.updateServer.mock.calls[0][1]).toMatchObject({
      env: { API_TOKEN: "***" },
      headers: { Authorization: "***" },
    });
  });

  // S7-G review: a stored token kept next to an added or changed env
  // variable (GITLAB_API_URL, HTTPS_PROXY) could be sent elsewhere; the
  // Go Core refuses it, and the form asks for the token again.
  it("asks for the stored values again when an env variable is added", async () => {
    await editRemote();
    fireEvent.click(screen.getByText("Add Variable"));
    fireEvent.input(screen.getByLabelText("Key 2"), { target: { value: "HTTPS_PROXY" } });
    fireEvent.input(screen.getByLabelText("Value 2"), {
      target: { value: "http://attacker.example:3128" },
    });

    expect(screen.getAllByText(STORED_NOT_KEPT)).toHaveLength(2);
    fireEvent.click(screen.getByText("Update Server"));
    await screen.findByText(/Stored secrets \(\*\*\*\) are kept only/);
    expect(mcp.updateServer).not.toHaveBeenCalled();
  });

  it("asks for the stored token again when a header changes", async () => {
    await editRemote();
    fireEvent.click(screen.getByText("Add Header"));
    fireEvent.input(screen.getByLabelText("Header name 2"), { target: { value: "X-Org" } });
    fireEvent.input(screen.getByLabelText("Header value 2"), { target: { value: "evil" } });

    expect(screen.getAllByText(STORED_NOT_KEPT)).toHaveLength(2);
  });

  it("creates a server with the headers entered", async () => {
    renderPage();
    fireEvent.click(await screen.findByText("Add Server"));
    fireEvent.input(screen.getByLabelText(/^Name/), { target: { value: "new" } });
    fireEvent.change(screen.getByLabelText("Transport"), { target: { value: "sse" } });
    fireEvent.input(await screen.findByLabelText(/^Server URL/), {
      target: { value: "https://new.example/sse" },
    });
    fireEvent.click(screen.getByText("Add Header"));
    fireEvent.input(screen.getByLabelText("Header name 1"), {
      target: { value: "X-Api-Key" },
    });
    fireEvent.input(screen.getByLabelText("Header value 1"), { target: { value: "k-1" } });
    // A row without a name is not sent.
    fireEvent.click(screen.getByText("Add Header"));
    fireEvent.click(screen.getByText("Create Server"));

    await waitFor(() => expect(mcp.createServer).toHaveBeenCalled());
    expect(mcp.createServer.mock.calls[0][0]).toMatchObject({ headers: { "X-Api-Key": "k-1" } });
  });

  // S7-G review: "Stored, unchanged" was shown for a "***" under a key that
  // was not read as "***"; the Go Core has no value for it and refuses.
  it("does not keep *** under a renamed header", async () => {
    await editRemote();
    fireEvent.input(screen.getByLabelText("Header name 1"), { target: { value: "X-Api-Key" } });

    expect(screen.queryByText(STORED_UNCHANGED)).toBeNull();
    fireEvent.click(screen.getByText("Update Server"));
    await screen.findByText(/Stored secrets \(\*\*\*\) are kept only/);
    expect(mcp.updateServer).not.toHaveBeenCalled();
  });

  it("does not keep *** typed under a key that was read empty", async () => {
    mcp.servers = [{ ...remote, env: { API_TOKEN: "***", REGION: "" } }];
    renderPage();
    fireEvent.click(await screen.findByLabelText("Edit server remote"));
    fireEvent.input(await screen.findByLabelText("Value 2"), { target: { value: "***" } });

    expect(screen.queryByText(STORED_UNCHANGED)).toBeNull();
    fireEvent.click(screen.getByText("Update Server"));
    await screen.findByText(/Stored secrets \(\*\*\*\) are kept only/);
    expect(mcp.updateServer).not.toHaveBeenCalled();
  });

  it("refuses *** for a new server, which has no stored value", async () => {
    renderPage();
    fireEvent.click(await screen.findByText("Add Server"));
    fireEvent.input(screen.getByLabelText(/^Name/), { target: { value: "new" } });
    fireEvent.click(screen.getByText("Add Variable"));
    fireEvent.input(screen.getByLabelText("Key 1"), { target: { value: "TOKEN" } });
    fireEvent.input(screen.getByLabelText("Value 1"), { target: { value: "***" } });

    expect(screen.getByText(STORED_NOT_KEPT)).toBeDefined();
    fireEvent.click(screen.getByText("Create Server"));
    await screen.findByText(/Stored secrets \(\*\*\*\) are kept only/);
    expect(mcp.createServer).not.toHaveBeenCalled();
  });

  it("offers headers only for sse and streamable_http servers", async () => {
    renderPage();
    fireEvent.click(await screen.findByText("Add Server"));

    expect(screen.queryByText("Add Header")).toBeNull();
    fireEvent.change(screen.getByLabelText("Transport"), { target: { value: "sse" } });
    expect(await screen.findByText("Add Header")).toBeDefined();
  });
});

// KI-98: viewers and editors saw the create, edit, delete and test actions,
// which the Go Core refuses them (403). They are hidden now; the server
// stays the authority.
describe("MCP admin actions", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mcp.servers = [remote, local];
    mcp.listServers.mockImplementation(() => Promise.resolve(mcp.servers));
  });

  it.each(["viewer", "editor"] as const)("hides the admin actions from a %s", async (role) => {
    auth.role = role;
    renderPage();
    await screen.findByText("remote");

    expect(screen.queryByText("Add Server")).toBeNull();
    expect(screen.queryByLabelText("Edit server remote")).toBeNull();
    expect(screen.queryByLabelText("Delete server remote")).toBeNull();
    expect(screen.queryByLabelText("Test connection for remote")).toBeNull();
    expect(screen.queryByText("Actions")).toBeNull();
    expect(
      screen.getByText(
        "Only admins of your organization add, change, test and assign MCP servers.",
      ),
    ).toBeDefined();
    // Reading the tools stays open to everyone.
    expect(screen.getByLabelText("Show tools for remote")).toBeDefined();
  });

  it("shows the admin actions to an admin", async () => {
    auth.role = "admin";
    renderPage();
    await screen.findByText("remote");

    expect(screen.getByText("Add Server")).toBeDefined();
    expect(screen.getByLabelText("Edit server remote")).toBeDefined();
    expect(screen.getByLabelText("Delete server remote")).toBeDefined();
    expect(screen.getByLabelText("Test connection for remote")).toBeDefined();
    expect(
      screen.queryByText(
        "Only admins of your organization add, change, test and assign MCP servers.",
      ),
    ).toBeNull();
  });
});
