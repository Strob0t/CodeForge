/**
 * TypeScript types for the benchmark validation LLM-debug report.
 * Matches the schema defined in the spec:
 * docs/superpowers/specs/2026-03-11-benchmark-validation-design.md
 */

// --- Test Matrix Types ---

export type BenchmarkType = "simple" | "tool_use" | "agent";
export type MetricName = "llm_judge" | "functional_test" | "sparc" | "trajectory_verifier";
export type BlockStatus = "passed" | "failed" | "partial" | "skipped";
export type TestStatus = "passed" | "failed" | "skipped";

export interface TestCase {
  id: string;
  block: number;
  suite: string;
  type: BenchmarkType;
  metrics: MetricName[];
  model?: string; // defaults to local model
  expectation?: string;
}

export interface ErrorTestCase {
  id: string;
  name: string;
  params: Record<string, unknown>;
  expectation: string;
}

// --- Report Types ---

export interface BlockSummary {
  total: number;
  passed: number;
  failed: number;
  skipped: number;
}

export interface BlockReport {
  block: {
    name: string;
    status: BlockStatus;
    started_at: string;
    finished_at: string;
    duration_ms: number;
    summary: BlockSummary;
  };
  environment: EnvironmentInfo;
  tests: TestReport[];
}

export interface EnvironmentInfo {
  backend_url: string;
  litellm_url: string;
  app_env: string;
  default_model: string;
  litellm_models_available: string[];
  git_commit: string;
}

export interface TestReport {
  id: string;
  name: string;
  status: TestStatus;
  suite: string;
  benchmark_type: string;
  metrics: string[];
  model: string;
  duration_ms: number;
  request: {
    method: string;
    url: string;
    body: Record<string, unknown>;
  };
  response: {
    status_code: number;
    body: Record<string, unknown>;
  };
  run_result?: {
    status: string;
    total_cost: number;
    total_tokens: number;
    results: TaskResult[];
  };
  frontend_checks: FrontendChecks;
  failure?: {
    assertion: string;
    message: string;
    screenshot?: string;
  };
  debug_context: DebugContext;
}

export interface TaskResult {
  task_id: string;
  scores: Record<string, number>;
  actual_output?: string;
  expected_output?: string;
  functional_test_output?: string;
  duration_ms: number;
}

export interface FrontendChecks {
  progress_bar_appeared: boolean;
  status_transition: string[];
  scores_displayed: boolean;
  cost_displayed: boolean;
  websocket_events_received: number;
}

export interface DebugContext {
  console_errors: string[];
  network_log: NetworkEntry[];
}

export interface NetworkEntry {
  method: string;
  url: string;
  status?: number;
  duration_ms?: number;
  event?: string;
  data?: string;
}

// --- Full Report Types ---

export interface FullReport {
  generated_at: string;
  total_duration_ms: number;
  git_commit: string;
  summary: {
    blocks_total: number;
    blocks_passed: number;
    blocks_failed: number;
    tests_total: number;
    tests_passed: number;
    tests_failed: number;
    tests_skipped: number;
    total_llm_cost_usd: number;
    total_llm_tokens: number;
  };
  matrix: MatrixEntry[];
  difficulty_audit: DifficultyAudit[];
  failures: FailureSummary[];
  recommendations: string[];
}

export interface MatrixEntry {
  suite: string;
  results: Record<string, TestStatus>;
}

export interface DifficultyAudit {
  suite: string;
  has_difficulty: boolean;
  estimation_method: string;
  values_found: string[];
  distribution: Record<string, number>;
}

export interface FailureSummary {
  test_id: string;
  one_line: string;
  root_hint: string;
}

// --- Run API Types ---

export interface BenchmarkRun {
  id: string;
  suite_id: string;
  dataset: string;
  model: string;
  status: string;
  benchmark_type: string;
  exec_mode: string;
  metrics: string[];
  total_cost_usd: number;
  total_tokens_in: number;
  total_tokens_out: number;
  created_at: string;
  updated_at: string;
  completed_at?: string;
  error_message?: string;
  selected_model?: string;
  routing_reason?: string;
}

export interface BenchmarkResult {
  id: string;
  run_id: string;
  task_id: string;
  task_name: string;
  scores: Record<string, number>;
  cost_usd: number;
  tokens_in: number;
  tokens_out: number;
  duration_ms: number;
  actual_output?: string;
  expected_output?: string;
  error_message?: string;
}

export interface BenchmarkSuite {
  id: string;
  name: string;
  type: string;
  provider_name: string;
  description?: string;
  config?: Record<string, unknown>;
}

export interface BenchmarkDataset {
  name: string;
  path: string;
  task_count: number;
}
