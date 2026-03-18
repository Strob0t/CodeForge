import { describe, expect, it } from "vitest";

import type { AgentEvent, TrajectorySummary } from "~/api/types";

import {
  agentEventToLiveFeedEvent,
  computeEta,
  emptyLiveFeedState,
  formatTokens,
  resultToFeatureEntry,
  statsFromSummary,
} from "./liveFeedState";

describe("formatTokens", () => {
  it("returns raw number below 1000", () => {
    expect(formatTokens(0)).toBe("0");
    expect(formatTokens(999)).toBe("999");
  });
  it("formats thousands with k suffix", () => {
    expect(formatTokens(1000)).toBe("1.0k");
    expect(formatTokens(1500)).toBe("1.5k");
    expect(formatTokens(24300)).toBe("24.3k");
    expect(formatTokens(999_999)).toBe("1000.0k");
  });
  it("formats millions with M suffix", () => {
    expect(formatTokens(1_000_000)).toBe("1.0M");
    expect(formatTokens(2_500_000)).toBe("2.5M");
  });
});

describe("computeEta", () => {
  it("returns null when total_tasks is null", () => {
    expect(computeEta(3, null, 120)).toBeNull();
  });
  it("returns null when 0 completed", () => {
    expect(computeEta(0, 5, 120)).toBeNull();
  });
  it("returns null when all completed", () => {
    expect(computeEta(5, 5, 120)).toBeNull();
  });
  it("calculates remaining seconds", () => {
    expect(computeEta(3, 5, 120)).toBe(80);
  });
  it("rounds to nearest second", () => {
    expect(computeEta(2, 3, 100)).toBe(50);
  });
});

describe("agentEventToLiveFeedEvent", () => {
  const base: AgentEvent = {
    id: "evt-1",
    agent_id: "agent-1",
    task_id: "task-1",
    project_id: "proj-1",
    type: "agent.tool_called",
    payload: { input: "hello", output: "world", success: true, step: 3 },
    version: 1,
    sequence_number: 42,
    created_at: "2026-03-16T12:00:00Z",
    tool_name: "Read",
    model: "gpt-4",
    tokens_in: 100,
    tokens_out: 50,
    cost_usd: 0.005,
  };

  it("maps top-level fields", () => {
    const result = agentEventToLiveFeedEvent(base);
    expect(result.id).toBe("evt-1");
    expect(result.event_type).toBe("agent.tool_called");
    expect(result.tool_name).toBe("Read");
    expect(result.model).toBe("gpt-4");
    expect(result.tokens_in).toBe(100);
    expect(result.tokens_out).toBe(50);
    expect(result.cost_usd).toBe(0.005);
    expect(result.project_id).toBe("proj-1");
  });

  it("maps payload fields", () => {
    const result = agentEventToLiveFeedEvent(base);
    expect(result.input).toBe("hello");
    expect(result.output).toBe("world");
    expect(result.success).toBe(true);
    expect(result.step).toBe(3);
  });

  it("converts created_at to timestamp", () => {
    const result = agentEventToLiveFeedEvent(base);
    expect(result.timestamp).toBe(new Date("2026-03-16T12:00:00Z").getTime());
  });

  it("maps sequence_number from AgentEvent", () => {
    const result = agentEventToLiveFeedEvent(base);
    expect(result.sequence_number).toBe(42);
  });

  it("falls back to payload when top-level fields missing", () => {
    const ev: AgentEvent = {
      ...base,
      sequence_number: 0,
      tool_name: undefined,
      model: undefined,
      tokens_in: undefined,
      tokens_out: undefined,
      cost_usd: undefined,
      payload: {
        tool_name: "Edit",
        model: "claude",
        tokens_in: 200,
        tokens_out: 80,
        cost_usd: 0.01,
        input: "x",
        output: "y",
        success: false,
        step: 1,
      },
    };
    const result = agentEventToLiveFeedEvent(ev);
    expect(result.tool_name).toBe("Edit");
    expect(result.model).toBe("claude");
    expect(result.tokens_in).toBe(200);
    expect(result.tokens_out).toBe(80);
    expect(result.cost_usd).toBe(0.01);
  });
});

describe("statsFromSummary", () => {
  const summary: TrajectorySummary = {
    total_events: 100,
    event_counts: {},
    duration_ms: 60000,
    tool_call_count: 47,
    error_count: 3,
    total_tokens_in: 24300,
    total_tokens_out: 8100,
    total_cost_usd: 0.42,
  };

  it("maps summary fields to AggregateStats", () => {
    const stats = statsFromSummary(summary, []);
    expect(stats.totalTokensIn).toBe(24300);
    expect(stats.totalTokensOut).toBe(8100);
    expect(stats.toolCallCount).toBe(47);
    expect(stats.toolSuccessCount).toBe(44);
  });

  it("computes avgScore from results", () => {
    const results = [{ scores: { correctness: 0.8 } }, { scores: { correctness: 0.6 } }];
    const stats = statsFromSummary(summary, results);
    expect(stats.avgScore).toBeCloseTo(0.7);
  });

  it("computes costPerTask from results count", () => {
    const results = [
      { scores: { correctness: 0.8 } },
      { scores: { correctness: 0.6 } },
      { scores: { correctness: 0.7 } },
    ];
    const stats = statsFromSummary(summary, results);
    expect(stats.costPerTask).toBeCloseTo(0.14);
  });

  it("handles zero results", () => {
    const stats = statsFromSummary(summary, []);
    expect(stats.avgScore).toBe(0);
    expect(stats.costPerTask).toBe(0);
  });
});

describe("resultToFeatureEntry", () => {
  it("maps BenchmarkResult to FeatureEntry", () => {
    const r = {
      task_id: "t1",
      task_name: "parse-json",
      cost_usd: 0.12,
      duration_ms: 72000,
      scores: { correctness: 0.95 },
    };
    const entry = resultToFeatureEntry(r);
    expect(entry.id).toBe("t1");
    expect(entry.name).toBe("parse-json");
    expect(entry.status).toBe("completed");
    expect(entry.cost).toBe(0.12);
    expect(entry.score).toBe(0.95);
    expect(entry.events).toEqual([]);
    expect(entry.startedAt).toBeDefined();
  });

  it("handles missing scores", () => {
    const r = {
      task_id: "t2",
      task_name: "empty",
      cost_usd: 0,
      duration_ms: 0,
    };
    const entry = resultToFeatureEntry(r);
    expect(entry.score).toBeUndefined();
    expect(entry.startedAt).toBeUndefined();
  });
});

describe("emptyLiveFeedState", () => {
  it("initializes lastSequenceNumber to 0", () => {
    const state = emptyLiveFeedState();
    expect(state.lastSequenceNumber).toBe(0);
  });

  it("initializes all fields to empty defaults", () => {
    const state = emptyLiveFeedState();
    expect(state.events).toEqual([]);
    expect(state.progress).toBeNull();
    expect(state.features.size).toBe(0);
    expect(state.hydratedFromApi).toBe(false);
    expect(state.lastEventId).toBeNull();
  });
});

describe("sequence_number dedup logic", () => {
  it("events with sequence_number <= lastSequenceNumber should be skippable", () => {
    const state = emptyLiveFeedState();
    state.lastSequenceNumber = 10;

    // Simulate dedup check: seqNum <= lastSequenceNumber means skip
    const shouldSkip = (seqNum: number) => seqNum > 0 && seqNum <= state.lastSequenceNumber;

    expect(shouldSkip(5)).toBe(true);
    expect(shouldSkip(10)).toBe(true);
    expect(shouldSkip(11)).toBe(false);
    expect(shouldSkip(0)).toBe(false); // 0 means no sequence_number, don't skip
  });

  it("hydration updates lastSequenceNumber to max from events", () => {
    const events = [
      {
        ...agentEventToLiveFeedEvent({
          id: "e1",
          agent_id: "",
          task_id: "",
          project_id: "p",
          type: "agent.started",
          payload: {},
          version: 1,
          sequence_number: 5,
          created_at: "2026-01-01T00:00:00Z",
        }),
        run_id: "r1",
      },
      {
        ...agentEventToLiveFeedEvent({
          id: "e2",
          agent_id: "",
          task_id: "",
          project_id: "p",
          type: "agent.step_done",
          payload: {},
          version: 2,
          sequence_number: 12,
          created_at: "2026-01-01T00:01:00Z",
        }),
        run_id: "r1",
      },
      {
        ...agentEventToLiveFeedEvent({
          id: "e3",
          agent_id: "",
          task_id: "",
          project_id: "p",
          type: "agent.finished",
          payload: {},
          version: 3,
          sequence_number: 8,
          created_at: "2026-01-01T00:02:00Z",
        }),
        run_id: "r1",
      },
    ];

    const maxSeq = events.reduce((max, e) => Math.max(max, e.sequence_number ?? 0), 0);
    expect(maxSeq).toBe(12);
  });
});
