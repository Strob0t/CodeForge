import { fireEvent, render, screen, waitFor } from "@solidjs/testing-library";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { Agent, Run, Task } from "~/api/types";
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
  start: vi.fn<(req: unknown) => Promise<Run>>(),
  listByTask: vi.fn<(taskId: string) => Promise<Run[]>>(),
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
    runs: { start: apiMock.start, listByTask: apiMock.listByTask },
    policies: { list: () => Promise.resolve({ profiles: [], presets: [] }) },
  },
}));

vi.mock("~/components/WebSocketProvider", () => ({
  useWebSocket: () => ({
    onMessage: (handler: (msg: WSMessage) => void) => {
      ws.handlers.add(handler);
      return () => ws.handlers.delete(handler);
    },
  }),
}));

vi.mock("./TrajectoryPanel", () => ({ default: () => null }));

import { ToastProvider } from "~/components/Toast";
import { I18nProvider } from "~/i18n";

import RunPanel from "./RunPanel";

const task: Task = {
  id: "t-1",
  project_id: "p-1",
  title: "Fix the bug",
  prompt: "",
  status: "pending",
  cost_usd: 0,
  created_at: "",
  updated_at: "",
};

const agent: Agent = {
  id: "a-1",
  project_id: "p-1",
  name: "Coder",
  backend: "aider",
  status: "idle",
  config: {},
  created_at: "",
  updated_at: "",
  total_runs: 0,
  total_cost: 0,
  success_rate: 0,
};

const started: Run = {
  id: "r-1",
  task_id: "t-1",
  agent_id: "a-1",
  project_id: "p-1",
  policy_profile: "",
  exec_mode: "mount",
  deliver_mode: "",
  status: "running",
  step_count: 0,
  cost_usd: 0,
  tokens_in: 0,
  tokens_out: 0,
  model: "",
  version: 1,
  started_at: "",
  created_at: "",
  updated_at: "",
};

beforeEach(() => {
  ws.handlers.clear();
  apiMock.start.mockReset().mockResolvedValue(started);
  apiMock.listByTask.mockReset().mockResolvedValue([]);
});

function renderPanel(): void {
  render(() => (
    <I18nProvider>
      <ToastProvider>
        <RunPanel projectId="p-1" tasks={[task]} agents={[agent]} onError={() => undefined} />
      </ToastProvider>
    </I18nProvider>
  ));
}

function clickStart(): void {
  fireEvent.change(screen.getByLabelText("Select task for run"), { target: { value: "t-1" } });
  fireEvent.change(screen.getByLabelText("Select agent for run"), { target: { value: "a-1" } });
  fireEvent.click(screen.getByRole("button", { name: "Start Run" }));
}

async function startRun(): Promise<void> {
  renderPanel();
  clickStart();
  await screen.findByText("running");
}

// KI-39: the active run was set once at start and never updated.
describe("RunPanel live updates", () => {
  it("updates the active run and its tool calls in place", async () => {
    await startRun();

    ws.emit("run.toolcall", {
      run_id: "r-1",
      call_id: "c-1",
      tool: "read_file",
      phase: "approved",
    });
    ws.emit("run.toolcall", {
      run_id: "r-9",
      call_id: "c-9",
      tool: "other_run_tool",
      phase: "approved",
    });
    await screen.findByText("read_file");
    expect(screen.queryByText("other_run_tool")).toBeNull();

    ws.emit("run.status", {
      run_id: "r-1",
      task_id: "t-1",
      project_id: "p-1",
      status: "completed",
      step_count: 4,
      cost_usd: 0.12,
      model: "gpt-x",
    });
    await screen.findByText("completed");
    expect(screen.getByText(/gpt-x/)).toBeTruthy();
  });

  it("refetches the selected task's run history on its run events", async () => {
    await startRun();
    const before = apiMock.listByTask.mock.calls.length;

    ws.emit("run.status", {
      run_id: "r-2",
      task_id: "t-1",
      project_id: "p-1",
      status: "running",
      step_count: 0,
    });
    await waitFor(() => expect(apiMock.listByTask.mock.calls.length).toBeGreaterThan(before));

    const after = apiMock.listByTask.mock.calls.length;
    ws.emit("run.status", {
      run_id: "r-3",
      task_id: "t-2",
      project_id: "p-1",
      status: "running",
      step_count: 0,
    });
    await new Promise((resolve) => setTimeout(resolve, 20));
    expect(apiMock.listByTask.mock.calls.length).toBe(after);
  });

  // KI-74: a fast run's first events arrived before api.runs.start resolved
  // and were dropped.
  it("applies the events that arrive while the run is being started", async () => {
    let resolveStart: (run: Run) => void = () => undefined;
    apiMock.start.mockReturnValue(
      new Promise<Run>((resolve) => {
        resolveStart = resolve;
      }),
    );
    renderPanel();
    clickStart();
    await waitFor(() => expect(apiMock.start).toHaveBeenCalledTimes(1));

    ws.emit("run.toolcall", {
      run_id: "r-1",
      call_id: "c-1",
      tool: "early_tool",
      phase: "approved",
    });
    ws.emit("run.toolcall", {
      run_id: "r-9",
      call_id: "c-9",
      tool: "other_tool",
      phase: "approved",
    });
    ws.emit("run.status", {
      run_id: "r-1",
      task_id: "t-1",
      project_id: "p-1",
      status: "completed",
      step_count: 2,
      model: "fast-model",
    });
    resolveStart(started);

    await screen.findByText("early_tool");
    expect(screen.getByText("completed")).toBeTruthy();
    expect(screen.getByText(/fast-model/)).toBeTruthy();
    expect(screen.queryByText("other_tool")).toBeNull();
  });

  it("drops the buffered events when the start fails", async () => {
    let rejectStart: (err: Error) => void = () => undefined;
    apiMock.start.mockReturnValue(
      new Promise<Run>((_, reject) => {
        rejectStart = reject;
      }),
    );
    renderPanel();
    clickStart();
    await waitFor(() => expect(apiMock.start).toHaveBeenCalledTimes(1));

    ws.emit("run.toolcall", {
      run_id: "r-1",
      call_id: "c-1",
      tool: "early_tool",
      phase: "approved",
    });
    rejectStart(new Error("boom"));
    await waitFor(() => expect(apiMock.start.mock.results[0]?.type).toBe("return"));
    await new Promise((resolve) => setTimeout(resolve, 20));

    // The next run starts with an empty buffer.
    apiMock.start.mockResolvedValue(started);
    clickStart();
    await screen.findByText("running");
    expect(screen.queryByText("early_tool")).toBeNull();
  });
});
