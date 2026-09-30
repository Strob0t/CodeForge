import { renderHook, waitFor } from "@solidjs/testing-library";
import type { JSX } from "solid-js";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { Agent, Task } from "~/api/types";
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
  tasks: vi.fn<(projectId: string) => Promise<Task[]>>(),
  agents: vi.fn<(projectId: string) => Promise<Agent[]>>(),
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
    projects: {
      get: (id: string) => Promise.resolve({ id, name: "P", config: {} }),
      gitStatus: () => Promise.resolve(null),
      branches: () => Promise.resolve([]),
    },
    tasks: { list: apiMock.tasks },
    agents: { list: apiMock.agents },
    goals: { list: () => Promise.resolve([]) },
    roadmap: { get: () => Promise.resolve(null) },
    sessions: { list: () => Promise.resolve([]) },
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

import { ToastProvider } from "~/components/Toast";
import { I18nProvider } from "~/i18n";

import { useProjectDetail } from "./useProjectDetail";

function task(id: string, agentId?: string): Task {
  return {
    id,
    project_id: "p-1",
    agent_id: agentId,
    title: id,
    prompt: "",
    status: "running",
    cost_usd: 0,
    created_at: "",
    updated_at: "",
  };
}

function Providers(props: { children: JSX.Element }): JSX.Element {
  return (
    <I18nProvider>
      <ToastProvider>{props.children}</ToastProvider>
    </I18nProvider>
  );
}

function renderDetail() {
  const { result } = renderHook(() => useProjectDetail(() => "p-1"), { wrapper: Providers });
  return result;
}

beforeEach(() => {
  ws.handlers.clear();
  apiMock.tasks.mockReset().mockResolvedValue([task("t-1", "a-1")]);
  apiMock.agents.mockReset().mockResolvedValue([
    {
      id: "a-1",
      project_id: "p-1",
      name: "Coder",
      backend: "aider",
      status: "running",
      config: {},
      created_at: "",
      updated_at: "",
      total_runs: 0,
      total_cost: 0,
      success_rate: 0,
    },
  ]);
});

// KI-39: the page filtered task.output on a project_id the event never
// carries, so live output stayed empty.
describe("useProjectDetail live output", () => {
  it("shows the output of the project's tasks only", async () => {
    const detail = renderDetail();
    await waitFor(() => expect(detail.tasks()).toHaveLength(1));

    ws.emit("task.output", { task_id: "t-1", line: "project line", stream: "stdout" });
    ws.emit("task.output", { task_id: "t-other", line: "foreign line", stream: "stdout" });
    ws.emit("task.output", { task_id: "conv-run-1", line: "chat chunk", stream: "stdout" });

    expect(detail.liveOutputLines().map((l) => l.line)).toEqual(["project line"]);
    expect(detail.liveOutputTaskId()).toBe("t-1");
    expect(detail.agentTerminals()).toEqual([
      expect.objectContaining({ agentId: "a-1", agentName: "Coder" }),
    ]);
  });

  it("attributes output of a run that started after the task list loaded", async () => {
    const detail = renderDetail();
    await waitFor(() => expect(detail.tasks()).toHaveLength(1));

    ws.emit("run.status", {
      run_id: "r-2",
      task_id: "t-new",
      project_id: "p-1",
      agent_id: "a-1",
      status: "running",
      step_count: 0,
    });
    ws.emit("task.output", { task_id: "t-new", line: "new task line", stream: "stderr" });

    expect(detail.liveOutputLines()).toEqual([
      expect.objectContaining({ line: "new task line", stream: "stderr" }),
    ]);
  });

  it("ignores runs of other projects", async () => {
    const detail = renderDetail();
    await waitFor(() => expect(detail.tasks()).toHaveLength(1));

    ws.emit("run.status", {
      run_id: "r-9",
      task_id: "t-9",
      project_id: "p-2",
      status: "running",
      step_count: 0,
    });
    ws.emit("task.output", { task_id: "t-9", line: "other project", stream: "stdout" });

    expect(detail.liveOutputLines()).toEqual([]);
  });

  it("refetches tasks and agents when a project run changes status", async () => {
    const detail = renderDetail();
    await waitFor(() => expect(detail.tasks()).toHaveLength(1));
    const tasksBefore = apiMock.tasks.mock.calls.length;
    const agentsBefore = apiMock.agents.mock.calls.length;

    ws.emit("run.status", {
      run_id: "r-1",
      task_id: "t-1",
      project_id: "p-1",
      status: "running",
      step_count: 0,
    });

    await waitFor(() => expect(apiMock.tasks.mock.calls.length).toBe(tasksBefore + 1));
    expect(apiMock.agents.mock.calls.length).toBe(agentsBefore + 1);
  });
});
