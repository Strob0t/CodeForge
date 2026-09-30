import { fireEvent, render, screen, waitFor } from "@solidjs/testing-library";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { MCPServer } from "~/api/types";

const apiMock = vi.hoisted(() => ({
  listServers: vi.fn<() => Promise<MCPServer[]>>(),
  listProjectServers: vi.fn<(projectId: string) => Promise<MCPServer[]>>(),
  assignToProject: vi.fn<(projectId: string, serverId: string) => Promise<void>>(),
  unassignFromProject: vi.fn<(projectId: string, serverId: string) => Promise<void>>(),
  updateProject: vi.fn<(id: string, data: unknown) => Promise<void>>(),
}));

// jsdom has no matchMedia; the UI modules read it when they are loaded.
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

vi.mock("~/api/client", () => ({
  api: {
    mcp: {
      listServers: apiMock.listServers,
      listProjectServers: apiMock.listProjectServers,
      assignToProject: apiMock.assignToProject,
      unassignFromProject: apiMock.unassignFromProject,
    },
    projects: { update: apiMock.updateProject },
  },
}));

vi.mock("../costs/CostDashboardPage", () => ({
  ProjectCostSection: () => <div>cost section</div>,
}));

import { ToastProvider } from "~/components/Toast";
import { I18nProvider } from "~/i18n";

import CompactSettingsPopover from "./CompactSettingsPopover";

function server(id: string, name: string): MCPServer {
  return {
    id,
    name,
    description: "",
    transport: "stdio",
    command: "",
    args: [],
    url: "",
    env: {},
    headers: {},
    enabled: true,
    status: "registered",
  };
}

function renderPopover(): void {
  render(() => (
    <I18nProvider>
      <ToastProvider>
        <CompactSettingsPopover projectId="p-1" open onClose={() => undefined} />
      </ToastProvider>
    </I18nProvider>
  ));
}

describe("CompactSettingsPopover", () => {
  beforeEach(() => {
    apiMock.listServers
      .mockReset()
      .mockResolvedValue([server("s-1", "docs"), server("s-2", "git")]);
    apiMock.listProjectServers.mockReset().mockResolvedValue([server("s-1", "docs")]);
    apiMock.assignToProject.mockReset().mockResolvedValue(undefined);
    apiMock.unassignFromProject.mockReset().mockResolvedValue(undefined);
    apiMock.updateProject.mockReset().mockResolvedValue(undefined);
  });

  // KI-41: autonomy_level is read by no backend code (autonomy comes from the
  // mode), and saving it replaced the project config.
  it("has no autonomy control and never writes the project config", async () => {
    renderPopover();
    await screen.findByText("docs");

    expect(document.getElementById("popover_autonomy")).toBeNull();
    expect(screen.queryByRole("button", { name: /save/i })).toBeNull();
    expect(screen.getByText(/autonomy comes from the selected mode/i)).toBeTruthy();
    expect(apiMock.updateProject).not.toHaveBeenCalled();
  });

  it("assigns and unassigns MCP servers with the project routes", async () => {
    renderPopover();
    const docs = await screen.findByRole("checkbox", { name: /docs/ });
    const git = screen.getByRole("checkbox", { name: /git/ });
    await waitFor(() => expect((docs as HTMLInputElement).checked).toBe(true));
    expect((git as HTMLInputElement).checked).toBe(false);

    fireEvent.click(git);
    await waitFor(() => expect(apiMock.assignToProject).toHaveBeenCalledWith("p-1", "s-2"));

    fireEvent.click(docs);
    await waitFor(() => expect(apiMock.unassignFromProject).toHaveBeenCalledWith("p-1", "s-1"));
    expect(apiMock.updateProject).not.toHaveBeenCalled();
  });
});
