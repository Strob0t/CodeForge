import { fireEvent, render, screen, waitFor } from "@solidjs/testing-library";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { Agent } from "~/api/types";

const apiMock = vi.hoisted(() => ({
  list: vi.fn<(projectId: string) => Promise<Agent[]>>(),
  create: vi.fn<(projectId: string, data: unknown) => Promise<Agent>>(),
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
    agents: { list: apiMock.list, create: apiMock.create },
    providers: { agent: () => Promise.resolve({ backends: [] }) },
  },
}));

import { ConfirmProvider } from "~/components/ConfirmProvider";
import { ToastProvider } from "~/components/Toast";
import { I18nProvider } from "~/i18n";

import AgentPanel from "./AgentPanel";

function agent(id: string, name: string): Agent {
  return {
    id,
    project_id: "p-1",
    name,
    backend: "aider",
    status: "idle",
    config: {},
    created_at: "",
    updated_at: "",
    total_runs: 0,
    total_cost: 0,
    success_rate: 0,
  };
}

beforeEach(() => {
  apiMock.list.mockReset().mockResolvedValue([]);
  apiMock.create.mockReset().mockResolvedValue(agent("a-2", "Reviewer"));
});

// KI-74: AgentPanel fetched the agent list itself, next to the project page,
// so every agent change cost two GET /agents.
describe("AgentPanel", () => {
  it("shows the page's agents and asks the page to refetch after a change", async () => {
    const onAgentsChanged = vi.fn();
    render(() => (
      <I18nProvider>
        <ToastProvider>
          <ConfirmProvider>
            <AgentPanel
              projectId="p-1"
              agents={[agent("a-1", "Coder")]}
              tasks={[]}
              onAgentsChanged={onAgentsChanged}
              onError={() => undefined}
            />
          </ConfirmProvider>
        </ToastProvider>
      </I18nProvider>
    ));

    expect(await screen.findByText("Coder")).toBeTruthy();
    expect(apiMock.list).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "Add Agent" }));
    fireEvent.input(screen.getByLabelText(/Name/), { target: { value: "Reviewer" } });
    fireEvent.input(screen.getByLabelText(/Backend/), { target: { value: "aider" } });
    fireEvent.submit(
      screen.getByRole("button", { name: "Create" }).closest("form") as HTMLFormElement,
    );

    await waitFor(() => expect(onAgentsChanged).toHaveBeenCalledTimes(1));
    expect(apiMock.create).toHaveBeenCalledWith("p-1", { name: "Reviewer", backend: "aider" });
    expect(apiMock.list).not.toHaveBeenCalled();
  });
});
