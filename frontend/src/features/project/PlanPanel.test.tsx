import { fireEvent, render, screen, waitFor } from "@solidjs/testing-library";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { ExecutionPlan, PlanStatus, PlanStepStatus } from "~/api/types";
import type { WSMessage } from "~/api/websocket";

const ws = vi.hoisted(() => {
  const handlers = new Set<(msg: WSMessage) => void>();
  return {
    handlers,
    emit(type: string, payload: Record<string, unknown>): void {
      for (const h of [...handlers]) h({ type, payload });
    },
  };
});

const apiMock = vi.hoisted(() => ({
  list: vi.fn<(projectId: string) => Promise<ExecutionPlan[]>>(),
  get: vi.fn<(id: string) => Promise<ExecutionPlan>>(),
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
  api: { plans: { list: apiMock.list, get: apiMock.get } },
}));

vi.mock("~/components/WebSocketProvider", () => ({
  useWebSocket: () => ({
    onMessage: (handler: (msg: WSMessage) => void) => {
      ws.handlers.add(handler);
      return () => ws.handlers.delete(handler);
    },
  }),
}));

vi.mock("~/ui/composites/ModelCombobox", () => ({ ModelCombobox: () => null }));
vi.mock("./AgentFlowGraph", () => ({ default: () => null }));
vi.mock("./StepDetailPanel", () => ({ default: () => null }));

import { I18nProvider } from "~/i18n";

import PlanPanel from "./PlanPanel";

function plan(status: PlanStatus, stepStatus: PlanStepStatus): ExecutionPlan {
  return {
    id: "pl-1",
    project_id: "p-1",
    name: "Refactor",
    description: "",
    protocol: "sequential",
    status,
    max_parallel: 1,
    steps: [
      {
        id: "s-1",
        plan_id: "pl-1",
        task_id: "t-1",
        agent_id: "a-1",
        policy_profile: "",
        deliver_mode: "",
        depends_on: [],
        status: stepStatus,
        run_id: "",
        round: 0,
        error: "",
        created_at: "",
        updated_at: "",
      },
    ],
    version: 1,
    created_at: "",
    updated_at: "",
  };
}

beforeEach(() => {
  ws.handlers.clear();
  apiMock.list.mockReset().mockResolvedValue([plan("pending", "pending")]);
  apiMock.get.mockReset().mockResolvedValue(plan("pending", "pending"));
});

function renderPanel(): void {
  render(() => (
    <I18nProvider>
      <PlanPanel projectId="p-1" tasks={[]} agents={[]} onError={() => undefined} />
    </I18nProvider>
  ));
}

// KI-39: the plan list and the selected plan were fetched once.
describe("PlanPanel live updates", () => {
  it("refetches the plans and the selected plan on its project's plan events", async () => {
    renderPanel();
    fireEvent.click(await screen.findByRole("button", { name: /Plan: Refactor/ }));
    await waitFor(() => expect(apiMock.get).toHaveBeenCalledTimes(1));

    apiMock.list.mockResolvedValue([plan("running", "running")]);
    apiMock.get.mockResolvedValue(plan("running", "running"));
    ws.emit("plan.step.status", {
      plan_id: "pl-1",
      step_id: "s-1",
      project_id: "p-1",
      status: "running",
    });

    await waitFor(() => expect(apiMock.list).toHaveBeenCalledTimes(2));
    expect(apiMock.get).toHaveBeenCalledTimes(2);
  });

  it("ignores plan events of other projects", async () => {
    renderPanel();
    await screen.findByRole("button", { name: /Plan: Refactor/ });

    ws.emit("plan.status", { plan_id: "pl-9", project_id: "p-2", status: "running" });
    await new Promise((resolve) => setTimeout(resolve, 20));
    expect(apiMock.list).toHaveBeenCalledTimes(1);
  });

  // KI-74: every plan.step.status refetched the plans and the selected plan.
  it("refetches once for a burst of step events", async () => {
    renderPanel();
    fireEvent.click(await screen.findByRole("button", { name: /Plan: Refactor/ }));
    await waitFor(() => expect(apiMock.get).toHaveBeenCalledTimes(1));

    for (const status of ["running", "completed", "running", "completed", "running"]) {
      ws.emit("plan.step.status", { plan_id: "pl-1", step_id: "s-1", project_id: "p-1", status });
      await new Promise((resolve) => setTimeout(resolve, 20));
    }

    await waitFor(() => expect(apiMock.list).toHaveBeenCalledTimes(2));
    expect(apiMock.get).toHaveBeenCalledTimes(2);
    await new Promise((resolve) => setTimeout(resolve, 600));
    expect(apiMock.list).toHaveBeenCalledTimes(2);
    expect(apiMock.get).toHaveBeenCalledTimes(2);
  });
});
