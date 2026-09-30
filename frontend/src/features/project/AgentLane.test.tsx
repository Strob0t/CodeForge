import { render, screen, waitFor } from "@solidjs/testing-library";
import { createSignal } from "solid-js";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { Agent, Run } from "~/api/types";
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
  active: vi.fn<(projectId: string) => Promise<Agent[]>>(),
  recentRuns: vi.fn<(projectId: string, limit?: number) => Promise<Run[]>>(),
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
    agents: { active: apiMock.active },
    costs: { recentRuns: apiMock.recentRuns },
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

vi.mock("./MessageFlow", () => ({ default: () => null }));
vi.mock("./SharedContextPanel", () => ({ default: () => null }));

import { I18nProvider } from "~/i18n";

import AgentLane from "./AgentLane";
import { type AgentWork, IDLE_WORK } from "./liveEvents";
import WarRoom from "./WarRoom";

function agent(id: string, name: string): Agent {
  return {
    id,
    project_id: "p-1",
    name,
    backend: "aider",
    status: "running",
    config: {},
    created_at: "",
    updated_at: "",
    total_runs: 0,
    total_cost: 0,
    success_rate: 0,
  };
}

function activeRun(id: string, taskId: string, agentId: string): Run {
  return {
    id,
    task_id: taskId,
    agent_id: agentId,
    project_id: "p-1",
    policy_profile: "",
    exec_mode: "mount",
    deliver_mode: "",
    status: "running",
    step_count: 3,
    cost_usd: 0.25,
    tokens_in: 0,
    tokens_out: 0,
    model: "",
    version: 1,
    started_at: "",
    created_at: "",
    updated_at: "",
  };
}

beforeEach(() => {
  ws.handlers.clear();
  apiMock.active.mockReset().mockResolvedValue([]);
  apiMock.recentRuns.mockReset().mockResolvedValue([]);
});

// KI-39: every lane used to append every task's output.
describe("AgentLane", () => {
  it("shows only the output and tool calls of its own task and run", async () => {
    const [work, setWork] = createSignal<AgentWork>({
      runId: "r-1",
      taskId: "t-1",
      steps: 2,
      costUsd: 0.5,
    });
    render(() => <AgentLane agent={agent("a-1", "Coder")} work={work()} />);

    ws.emit("task.output", { task_id: "t-1", line: "own line", stream: "stdout" });
    ws.emit("task.output", { task_id: "t-2", line: "foreign line", stream: "stdout" });
    ws.emit("run.toolcall", { run_id: "r-1", call_id: "c-1", tool: "own_tool", phase: "approved" });
    ws.emit("run.toolcall", {
      run_id: "r-2",
      call_id: "c-2",
      tool: "foreign_tool",
      phase: "approved",
    });

    await screen.findByText("own line");
    expect(screen.getByText("own_tool")).toBeTruthy();
    expect(screen.queryByText("foreign line")).toBeNull();
    expect(screen.queryByText("foreign_tool")).toBeNull();
    expect(screen.getByText("Steps: 2")).toBeTruthy();
    expect(screen.getByText("$0.5000")).toBeTruthy();

    setWork({ runId: "r-2", taskId: "t-2", steps: 0, costUsd: 0 });
    ws.emit("task.output", { task_id: "t-2", line: "next task line", stream: "stdout" });
    await screen.findByText("next task line");
  });

  it("shows nothing while the agent has no task", async () => {
    render(() => <AgentLane agent={agent("a-1", "Coder")} work={IDLE_WORK} />);
    ws.emit("task.output", { task_id: "t-1", line: "stray line", stream: "stdout" });
    await waitFor(() => expect(screen.queryByText("stray line")).toBeNull());
  });
});

describe("WarRoom", () => {
  function renderWarRoom(): void {
    render(() => (
      <I18nProvider>
        <WarRoom projectId="p-1" />
      </I18nProvider>
    ));
  }

  it("routes each task's output to the lane of the agent running it", async () => {
    apiMock.active.mockResolvedValue([agent("a-1", "Coder"), agent("a-2", "Reviewer")]);
    renderWarRoom();
    await screen.findByText("Coder");

    ws.emit("run.status", {
      run_id: "r-1",
      task_id: "t-1",
      project_id: "p-1",
      agent_id: "a-1",
      status: "running",
      step_count: 0,
    });
    ws.emit("run.status", {
      run_id: "r-2",
      task_id: "t-2",
      project_id: "p-1",
      agent_id: "a-2",
      status: "running",
      step_count: 0,
    });
    ws.emit("task.output", { task_id: "t-1", line: "coder output", stream: "stdout" });
    ws.emit("task.output", { task_id: "t-2", line: "reviewer output", stream: "stdout" });

    const coderLane = document.querySelector('[data-agent-id="a-1"]');
    const reviewerLane = document.querySelector('[data-agent-id="a-2"]');
    await waitFor(() => expect(coderLane?.textContent).toContain("coder output"));
    expect(coderLane?.textContent).not.toContain("reviewer output");
    expect(reviewerLane?.textContent).toContain("reviewer output");
    expect(reviewerLane?.textContent).not.toContain("coder output");
  });

  it("attaches a lane opened mid-run to the agent's active run", async () => {
    apiMock.active.mockResolvedValue([agent("a-1", "Coder")]);
    apiMock.recentRuns.mockResolvedValue([activeRun("r-1", "t-1", "a-1")]);
    renderWarRoom();
    await screen.findByText("Coder");
    await screen.findByText("Steps: 3");

    ws.emit("task.output", { task_id: "t-1", line: "mid-run output", stream: "stdout" });
    await screen.findByText("mid-run output");
  });

  it("ignores run events of other projects", async () => {
    apiMock.active.mockResolvedValue([agent("a-1", "Coder")]);
    renderWarRoom();
    await screen.findByText("Coder");

    ws.emit("run.status", {
      run_id: "r-9",
      task_id: "t-9",
      project_id: "p-2",
      agent_id: "a-1",
      status: "running",
      step_count: 0,
    });
    ws.emit("task.output", { task_id: "t-9", line: "other project output", stream: "stdout" });
    await waitFor(() => expect(screen.queryByText("other project output")).toBeNull());
  });
});
