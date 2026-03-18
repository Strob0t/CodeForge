import type {
  AgentEvent,
  BenchmarkLiveProgress,
  LiveFeedEvent,
  TrajectorySummary,
} from "~/api/types";

export type { LiveFeedEvent };

export const MAX_EVENTS = 5000;

// ---- Types ----

export interface FeatureEntry {
  id: string;
  name: string;
  status: "pending" | "running" | "completed" | "failed";
  events: LiveFeedEvent[];
  startedAt?: number;
  cost: number;
  step: number;
  score?: number;
}

export interface AggregateStats {
  totalTokensIn: number;
  totalTokensOut: number;
  toolCallCount: number;
  toolSuccessCount: number;
  avgScore: number;
  costPerTask: number;
}

export interface LiveFeedState {
  events: LiveFeedEvent[];
  progress: BenchmarkLiveProgress | null;
  features: Map<string, FeatureEntry>;
  stats: AggregateStats;
  hydratedFromApi: boolean;
  lastEventId: string | null;
  lastSequenceNumber: number;
}

// ---- Helpers ----

export function emptyStats(): AggregateStats {
  return {
    totalTokensIn: 0,
    totalTokensOut: 0,
    toolCallCount: 0,
    toolSuccessCount: 0,
    avgScore: 0,
    costPerTask: 0,
  };
}

export function emptyLiveFeedState(): LiveFeedState {
  return {
    events: [],
    progress: null,
    features: new Map(),
    stats: emptyStats(),
    hydratedFromApi: false,
    lastEventId: null,
    lastSequenceNumber: 0,
  };
}

export function formatTokens(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}k`;
  return String(n);
}

// ---- Derived helpers ----

export function computeEta(
  completed: number,
  total: number | null,
  elapsedSec: number,
): number | null {
  if (total === null || completed === 0 || completed >= total) return null;
  const secPerTask = elapsedSec / completed;
  return Math.round(secPerTask * (total - completed));
}

export function agentEventToLiveFeedEvent(ev: AgentEvent): LiveFeedEvent {
  return {
    id: ev.id,
    timestamp: new Date(ev.created_at).getTime(),
    run_id: "",
    project_id: ev.project_id,
    event_type: ev.type,
    sequence_number: ev.sequence_number,
    tool_name: ev.tool_name ?? (ev.payload.tool_name as string | undefined),
    model: ev.model ?? (ev.payload.model as string | undefined),
    input: ev.payload.input as string | undefined,
    output: ev.payload.output as string | undefined,
    success: ev.payload.success as boolean | undefined,
    step: ev.payload.step as number | undefined,
    cost_usd: ev.cost_usd ?? (ev.payload.cost_usd as number | undefined),
    tokens_in: ev.tokens_in ?? (ev.payload.tokens_in as number | undefined),
    tokens_out: ev.tokens_out ?? (ev.payload.tokens_out as number | undefined),
  };
}

interface ResultWithScores {
  scores?: Record<string, number>;
}

export function statsFromSummary(
  summary: TrajectorySummary,
  results: ResultWithScores[],
): AggregateStats {
  const completedCount = results.length;
  const avgScore =
    completedCount > 0
      ? results.reduce((sum, r) => {
          const firstScore = r.scores ? (Object.values(r.scores)[0] ?? 0) : 0;
          return sum + firstScore;
        }, 0) / completedCount
      : 0;

  return {
    totalTokensIn: summary.total_tokens_in,
    totalTokensOut: summary.total_tokens_out,
    toolCallCount: summary.tool_call_count,
    toolSuccessCount: summary.tool_call_count - summary.error_count,
    avgScore,
    costPerTask: completedCount > 0 ? summary.total_cost_usd / completedCount : 0,
  };
}

interface ResultForFeature {
  task_id: string;
  task_name: string;
  cost_usd: number;
  duration_ms: number;
  scores?: Record<string, number>;
}

export function resultToFeatureEntry(r: ResultForFeature): FeatureEntry {
  return {
    id: r.task_id,
    name: r.task_name,
    status: "completed",
    events: [],
    cost: r.cost_usd,
    step: 0,
    score: r.scores ? (Object.values(r.scores)[0] as number | undefined) : undefined,
    startedAt: r.duration_ms > 0 ? Date.now() - r.duration_ms : undefined,
  };
}
