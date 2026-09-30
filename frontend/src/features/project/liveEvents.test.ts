import { describe, expect, it } from "vitest";

import type { Run } from "~/api/types";
import type { WSMessage } from "~/api/websocket";

import {
  type AgentWork,
  agentWorkFromRuns,
  applyRunStatus,
  createProjectTaskIndex,
  IDLE_WORK,
  isProjectEvent,
  parseTaskOutput,
  parseToolCall,
  planEventEffect,
  reduceAgentWork,
} from "./liveEvents";

function ws(type: string, payload: Record<string, unknown>): WSMessage {
  return { type, payload };
}

function run(overrides: Partial<Run> = {}): Run {
  return {
    id: "r-1",
    task_id: "t-1",
    agent_id: "a-1",
    project_id: "p-1",
    policy_profile: "headless-safe-sandbox",
    exec_mode: "mount",
    deliver_mode: "",
    status: "running",
    step_count: 0,
    cost_usd: 0,
    tokens_in: 0,
    tokens_out: 0,
    model: "",
    version: 1,
    started_at: "2026-09-30T10:00:00Z",
    created_at: "2026-09-30T10:00:00Z",
    updated_at: "2026-09-30T10:00:00Z",
    ...overrides,
  };
}

describe("isProjectEvent", () => {
  it.each([
    ["same project", { project_id: "p-1" }, "p-1", true],
    ["other project", { project_id: "p-2" }, "p-1", false],
    ["no project id", {}, "p-1", false],
    ["non-string project id", { project_id: 1 }, "p-1", false],
    ["empty project filter", { project_id: "" }, "", false],
  ])("%s", (_name, payload, projectId, want) => {
    expect(isProjectEvent(ws("run.status", payload), projectId)).toBe(want);
  });
});

describe("parseTaskOutput", () => {
  it("parses a task.output line", () => {
    expect(
      parseTaskOutput(ws("task.output", { task_id: "t-1", line: "ok", stream: "stderr" })),
    ).toEqual({ taskId: "t-1", line: "ok", stream: "stderr" });
  });

  it("defaults to stdout and keeps empty lines", () => {
    expect(parseTaskOutput(ws("task.output", { task_id: "t-1", line: "" }))).toEqual({
      taskId: "t-1",
      line: "",
      stream: "stdout",
    });
  });

  it.each([
    ["another event", ws("agent.output", { task_id: "t-1", line: "x" })],
    ["no task id", ws("task.output", { line: "x" })],
    ["no line", ws("task.output", { task_id: "t-1" })],
  ])("ignores %s", (_name, msg) => {
    expect(parseTaskOutput(msg)).toBeNull();
  });
});

// task.output carries only a task ID (its NATS sources have no project ID);
// the page attributes it through the project's task IDs.
describe("createProjectTaskIndex", () => {
  it("owns the project's listed tasks and remembers their agents", () => {
    const index = createProjectTaskIndex("p-1");
    index.addTasks([
      { id: "t-1", project_id: "p-1", agent_id: "a-1" },
      { id: "t-2", project_id: "p-1" },
      { id: "t-3", project_id: "p-2" },
    ]);
    expect(index.owns("t-1")).toBe(true);
    expect(index.owns("t-2")).toBe(true);
    expect(index.owns("t-3")).toBe(false);
    expect(index.owns("t-9")).toBe(false);
    expect(index.agentOf("t-1")).toBe("a-1");
    expect(index.agentOf("t-2")).toBeUndefined();
  });

  it("learns tasks from project events before the task list is refetched", () => {
    const index = createProjectTaskIndex("p-1");
    index.observe(
      ws("run.status", { run_id: "r-1", task_id: "t-3", project_id: "p-1", agent_id: "a-2" }),
    );
    index.observe(ws("task.status", { task_id: "t-4", project_id: "p-1", agent_id: "a-3" }));
    expect(index.owns("t-3")).toBe(true);
    expect(index.agentOf("t-3")).toBe("a-2");
    expect(index.agentOf("t-4")).toBe("a-3");
  });

  it("ignores tasks of other projects and events without a task", () => {
    const index = createProjectTaskIndex("p-1");
    index.observe(ws("run.status", { task_id: "t-5", project_id: "p-2" }));
    index.observe(ws("agent.status", { agent_id: "a-1", project_id: "p-1" }));
    expect(index.owns("t-5")).toBe(false);
  });

  it("keeps a known agent when a later event names none", () => {
    const index = createProjectTaskIndex("p-1");
    index.addTasks([{ id: "t-1", project_id: "p-1", agent_id: "a-1" }]);
    index.observe(ws("task.status", { task_id: "t-1", project_id: "p-1", status: "completed" }));
    expect(index.agentOf("t-1")).toBe("a-1");
  });
});

describe("applyRunStatus", () => {
  it("updates the run the event names", () => {
    const updated = applyRunStatus(
      run(),
      ws("run.status", {
        run_id: "r-1",
        status: "completed",
        step_count: 7,
        cost_usd: 0.5,
        tokens_in: 10,
        tokens_out: 20,
        model: "m",
      }),
    );
    expect(updated).toMatchObject({
      id: "r-1",
      status: "completed",
      step_count: 7,
      cost_usd: 0.5,
      tokens_in: 10,
      tokens_out: 20,
      model: "m",
    });
  });

  it("keeps metrics Go omits when zero", () => {
    const updated = applyRunStatus(
      run({ cost_usd: 0.2, model: "m" }),
      ws("run.status", { run_id: "r-1", status: "quality_gate", step_count: 3 }),
    );
    expect(updated).toMatchObject({
      status: "quality_gate",
      step_count: 3,
      cost_usd: 0.2,
      model: "m",
    });
  });

  it.each([
    ["another run", ws("run.status", { run_id: "r-2", status: "completed", step_count: 1 })],
    ["another event", ws("run.toolcall", { run_id: "r-1", status: "completed" })],
    ["an unknown status", ws("run.status", { run_id: "r-1", status: "exploded", step_count: 1 })],
  ])("ignores %s", (_name, msg) => {
    expect(applyRunStatus(run(), msg)).toBeNull();
  });
});

describe("parseToolCall", () => {
  it("parses a run.toolcall event", () => {
    expect(
      parseToolCall(
        ws("run.toolcall", {
          run_id: "r-1",
          call_id: "c-1",
          tool: "bash",
          phase: "approved",
          decision: "allow",
        }),
      ),
    ).toEqual({
      run_id: "r-1",
      call_id: "c-1",
      tool: "bash",
      phase: "approved",
      decision: "allow",
    });
  });

  it.each([
    [
      "another event",
      ws("run.status", { run_id: "r-1", call_id: "c-1", tool: "bash", phase: "x" }),
    ],
    ["no run id", ws("run.toolcall", { call_id: "c-1", tool: "bash", phase: "x" })],
    ["no phase", ws("run.toolcall", { run_id: "r-1", call_id: "c-1", tool: "bash" })],
  ])("ignores %s", (_name, msg) => {
    expect(parseToolCall(msg)).toBeNull();
  });
});

describe("reduceAgentWork", () => {
  it("binds the agent to the run and task of its run.status", () => {
    const work = reduceAgentWork(
      IDLE_WORK,
      ws("run.status", {
        run_id: "r-1",
        task_id: "t-1",
        project_id: "p-1",
        agent_id: "a-1",
        step_count: 2,
        cost_usd: 0.1,
      }),
      "a-1",
    );
    expect(work).toEqual({ runId: "r-1", taskId: "t-1", steps: 2, costUsd: 0.1 });
  });

  it("keeps the cost of the same run when Go omits a zero cost", () => {
    const start: AgentWork = { runId: "r-1", taskId: "t-1", steps: 2, costUsd: 0.1 };
    const work = reduceAgentWork(
      start,
      ws("run.status", { run_id: "r-1", task_id: "t-1", agent_id: "a-1", step_count: 4 }),
      "a-1",
    );
    expect(work).toEqual({ runId: "r-1", taskId: "t-1", steps: 4, costUsd: 0.1 });
  });

  it("starts over for a new run", () => {
    const start: AgentWork = { runId: "r-1", taskId: "t-1", steps: 9, costUsd: 1 };
    const work = reduceAgentWork(
      start,
      ws("run.status", { run_id: "r-2", task_id: "t-2", agent_id: "a-1", step_count: 0 }),
      "a-1",
    );
    expect(work).toEqual({ runId: "r-2", taskId: "t-2", steps: 0, costUsd: 0 });
  });

  it("binds to a dispatched task", () => {
    const work = reduceAgentWork(
      IDLE_WORK,
      ws("task.status", { task_id: "t-7", agent_id: "a-1", status: "queued" }),
      "a-1",
    );
    expect(work).toEqual({ taskId: "t-7", steps: 0, costUsd: 0 });
  });

  it.each([
    [
      "another agent's run",
      ws("run.status", { run_id: "r-9", task_id: "t-9", agent_id: "a-2", step_count: 1 }),
    ],
    [
      "a run.status without agent",
      ws("run.status", { run_id: "r-9", task_id: "t-9", step_count: 1 }),
    ],
    ["unrelated events", ws("task.output", { task_id: "t-9", line: "x", agent_id: "a-1" })],
  ])("ignores %s", (_name, msg) => {
    expect(reduceAgentWork(IDLE_WORK, msg, "a-1")).toBe(IDLE_WORK);
  });
});

describe("agentWorkFromRuns", () => {
  it("takes the agent's newest active run", () => {
    const runs = [
      run({ id: "r-3", agent_id: "a-2", status: "running" }),
      run({
        id: "r-2",
        task_id: "t-2",
        agent_id: "a-1",
        status: "running",
        step_count: 4,
        cost_usd: 0.3,
      }),
      run({ id: "r-1", agent_id: "a-1", status: "running" }),
    ];
    expect(agentWorkFromRuns(runs, "a-1")).toEqual({
      runId: "r-2",
      taskId: "t-2",
      steps: 4,
      costUsd: 0.3,
    });
  });

  it("ignores finished runs", () => {
    expect(
      agentWorkFromRuns([run({ status: "completed" }), run({ status: "failed" })], "a-1"),
    ).toBeUndefined();
  });
});

describe("planEventEffect", () => {
  it("refetches the list, and the selected plan when it changed", () => {
    expect(
      planEventEffect(
        ws("plan.status", { plan_id: "pl-1", project_id: "p-1", status: "running" }),
        "p-1",
        "pl-1",
      ),
    ).toEqual({
      refetchPlans: true,
      refetchSelected: true,
    });
    expect(
      planEventEffect(
        ws("plan.status", { plan_id: "pl-2", project_id: "p-1", status: "running" }),
        "p-1",
        "pl-1",
      ),
    ).toEqual({
      refetchPlans: true,
      refetchSelected: false,
    });
  });

  it("carries the review decision of a step status", () => {
    const effect = planEventEffect(
      ws("plan.step.status", {
        plan_id: "pl-1",
        step_id: "s-1",
        project_id: "p-1",
        status: "running",
        review_decision: { needs_review: true, confidence: 0.4, reason: "risky", routed: true },
      }),
      "p-1",
      null,
    );
    expect(effect).toEqual({
      refetchPlans: true,
      refetchSelected: false,
      reviewDecision: {
        stepId: "s-1",
        decision: { needs_review: true, confidence: 0.4, reason: "risky", routed: true },
      },
    });
  });

  it("maps review router decisions and debates to their step", () => {
    expect(
      planEventEffect(
        ws("review_router.decision", {
          plan_id: "pl-1",
          step_id: "s-2",
          project_id: "p-1",
          needs_review: false,
          confidence: 0.9,
          reason: "fine",
          routed: false,
        }),
        "p-1",
        "pl-1",
      ),
    ).toEqual({
      refetchPlans: false,
      refetchSelected: false,
      reviewDecision: {
        stepId: "s-2",
        decision: { needs_review: false, confidence: 0.9, reason: "fine", routed: false },
      },
    });

    const debate = {
      plan_id: "pl-1",
      step_id: "s-3",
      project_id: "p-1",
      debate_plan_id: "d-1",
      status: "completed",
      synthesis: "agreed",
    };
    expect(planEventEffect(ws("debate.status", debate), "p-1", "pl-1")).toEqual({
      refetchPlans: false,
      refetchSelected: false,
      debate,
    });
  });

  it.each([
    [
      "another project",
      ws("plan.status", { plan_id: "pl-1", project_id: "p-2", status: "running" }),
    ],
    [
      "an unrelated event",
      ws("run.status", { run_id: "r-1", project_id: "p-1", status: "running" }),
    ],
    [
      "a debate with an unknown status",
      ws("debate.status", {
        plan_id: "pl-1",
        step_id: "s-3",
        project_id: "p-1",
        debate_plan_id: "d",
        status: "odd",
      }),
    ],
  ])("ignores %s", (_name, msg) => {
    expect(planEventEffect(msg, "p-1", "pl-1")).toBeNull();
  });
});
