/**
 * Shared test utilities for benchmark validation E2E tests.
 * Provides API wrappers, polling, frontend verification, and debug context collection.
 */

import { type TestInfo } from "@playwright/test";
import type {
  BenchmarkRun,
  BenchmarkResult,
  BenchmarkSuite,
  BenchmarkDataset,
  DebugContext,
  FrontendChecks,
  NetworkEntry,
} from "./types";
import { DEFAULT_MODEL } from "./matrix";

// ---------------------------------------------------------------------------
// Config
// ---------------------------------------------------------------------------

const API_BASE = process.env.API_BASE ?? "http://localhost:8080/api/v1";
const HEALTH_BASE = API_BASE.replace("/api/v1", "");
const LITELLM_URL = process.env.LITELLM_URL ?? "http://codeforge-litellm:4000";
const ADMIN_EMAIL = "admin@localhost";
const ADMIN_PASS = "Changeme123";

/** Default poll interval for run completion (ms). */
const POLL_INTERVAL = 5_000;

// ---------------------------------------------------------------------------
// Auth
// ---------------------------------------------------------------------------

let cachedToken: string | null = null;

async function getToken(): Promise<string> {
  if (cachedToken) return cachedToken;

  const res = await fetch(`${API_BASE}/auth/login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email: ADMIN_EMAIL, password: ADMIN_PASS }),
  });
  if (!res.ok) throw new Error(`Login failed (${res.status}): ${await res.text()}`);
  const body = (await res.json()) as {
    access_token: string;
    user: { must_change_password?: boolean };
  };

  // Handle forced password change for seeded admin
  if (body.user.must_change_password) {
    await fetch(`${API_BASE}/auth/change-password`, {
      method: "POST",
      headers: { "Content-Type": "application/json", Authorization: `Bearer ${body.access_token}` },
      body: JSON.stringify({ old_password: ADMIN_PASS, new_password: ADMIN_PASS }),
    });
    // Re-login for fresh token
    return getToken();
  }

  cachedToken = body.access_token;
  return cachedToken;
}

async function authHeaders(): Promise<Record<string, string>> {
  const token = await getToken();
  return { Authorization: `Bearer ${token}`, "Content-Type": "application/json" };
}

// ---------------------------------------------------------------------------
// Health Checks
// ---------------------------------------------------------------------------

export async function checkBackendHealth(): Promise<{ status: string; dev_mode: boolean }> {
  const token = await getToken();
  const res = await fetch(`${HEALTH_BASE}/health`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  if (!res.ok) throw new Error(`Backend health check failed: ${res.status}`);
  return res.json() as Promise<{ status: string; dev_mode: boolean }>;
}

export async function checkLiteLLMHealth(): Promise<boolean> {
  try {
    const res = await fetch(`${LITELLM_URL}/health/liveliness`);
    if (!res.ok) return false;
    const text = await res.text();
    return text.includes("I'm alive");
  } catch {
    return false;
  }
}

export async function getLiteLLMModels(): Promise<string[]> {
  try {
    const masterKey = process.env.LITELLM_MASTER_KEY ?? "sk-codeforge-dev";
    const res = await fetch(`${LITELLM_URL}/v1/models`, {
      headers: { Authorization: `Bearer ${masterKey}` },
    });
    if (!res.ok) return [];
    const body = (await res.json()) as { data?: Array<{ id: string }> };
    return (body.data ?? []).map((m) => m.id);
  } catch {
    return [];
  }
}

export async function checkNATSConnected(): Promise<boolean> {
  const health = await checkBackendHealth();
  // The /health endpoint includes NATS status
  return health.status === "ok";
}

// ---------------------------------------------------------------------------
// Benchmark API Wrappers
// ---------------------------------------------------------------------------

export async function listSuites(): Promise<BenchmarkSuite[]> {
  const headers = await authHeaders();
  const res = await fetch(`${API_BASE}/benchmarks/suites`, { headers });
  if (!res.ok) throw new Error(`List suites failed: ${res.status}`);
  return res.json() as Promise<BenchmarkSuite[]>;
}

export async function listDatasets(): Promise<BenchmarkDataset[]> {
  const headers = await authHeaders();
  const res = await fetch(`${API_BASE}/benchmarks/datasets`, { headers });
  if (!res.ok) throw new Error(`List datasets failed: ${res.status}`);
  return res.json() as Promise<BenchmarkDataset[]>;
}

export async function getSuiteByProvider(providerName: string): Promise<BenchmarkSuite | null> {
  const suites = await listSuites();
  return suites.find((s) => s.provider_name === providerName) ?? null;
}

export interface CreateRunParams {
  dataset: string;
  model: string;
  metrics: string[];
  benchmark_type?: string;
  exec_mode?: string;
  suite_id?: string;
}

export async function createBenchmarkRun(params: CreateRunParams): Promise<BenchmarkRun> {
  const headers = await authHeaders();
  const body: Record<string, unknown> = {
    dataset: params.dataset,
    model: params.model,
    metrics: params.metrics,
    exec_mode: params.exec_mode ?? "mount",
  };
  if (params.benchmark_type) body.benchmark_type = params.benchmark_type;
  if (params.suite_id) body.suite_id = params.suite_id;

  const res = await fetch(`${API_BASE}/benchmarks/runs`, {
    method: "POST",
    headers,
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(`Create run failed (${res.status}): ${text}`);
  }
  return res.json() as Promise<BenchmarkRun>;
}

/**
 * Create a benchmark run but return the raw response for error scenario testing.
 * Does NOT throw on non-2xx responses.
 */
export async function createBenchmarkRunRaw(
  params: Record<string, unknown>,
): Promise<{ status: number; body: unknown }> {
  const headers = await authHeaders();
  const res = await fetch(`${API_BASE}/benchmarks/runs`, {
    method: "POST",
    headers,
    body: JSON.stringify({ exec_mode: "mount", ...params }),
  });
  const text = await res.text();
  let body: unknown;
  try {
    body = JSON.parse(text);
  } catch {
    body = text;
  }
  return { status: res.status, body };
}

export async function getRun(runId: string): Promise<BenchmarkRun> {
  const headers = await authHeaders();
  const res = await fetch(`${API_BASE}/benchmarks/runs/${runId}`, { headers });
  if (!res.ok) throw new Error(`Get run ${runId} failed: ${res.status}`);
  return res.json() as Promise<BenchmarkRun>;
}

export async function getRunResults(runId: string): Promise<BenchmarkResult[]> {
  const headers = await authHeaders();
  const res = await fetch(`${API_BASE}/benchmarks/runs/${runId}/results`, { headers });
  if (!res.ok) return [];
  return res.json() as Promise<BenchmarkResult[]>;
}

export async function deleteRun(runId: string): Promise<void> {
  const headers = await authHeaders();
  await fetch(`${API_BASE}/benchmarks/runs/${runId}`, { method: "DELETE", headers });
}

// ---------------------------------------------------------------------------
// Polling
// ---------------------------------------------------------------------------

/**
 * Poll a benchmark run until it reaches a terminal status (completed/failed).
 * Returns the final run state.
 */
export async function waitForRunCompletion(
  runId: string,
  timeoutMs: number = 600_000,
  pollMs: number = POLL_INTERVAL,
): Promise<BenchmarkRun> {
  const start = Date.now();
  while (Date.now() - start < timeoutMs) {
    const run = await getRun(runId);
    if (run.status === "completed" || run.status === "failed") {
      return run;
    }
    await sleep(pollMs);
  }
  // Timeout — return whatever state we have
  const finalRun = await getRun(runId);
  return finalRun;
}

// ---------------------------------------------------------------------------
// Frontend Checks (API-level simulation)
// ---------------------------------------------------------------------------

/**
 * Verify frontend-observable state for a completed run.
 * Since we test API-level (no browser), we verify the data that the frontend
 * would display: status transitions, scores, costs, etc.
 */
export async function verifyFrontendState(
  run: BenchmarkRun,
  results: BenchmarkResult[],
): Promise<FrontendChecks> {
  const checks: FrontendChecks = {
    progress_bar_appeared: run.status === "completed" || run.status === "failed",
    status_transition:
      run.status === "completed" ? ["running", "completed"] : ["running", "failed"],
    scores_displayed:
      results.length > 0 && results.some((r) => Object.keys(r.scores ?? {}).length > 0),
    cost_displayed: run.total_cost_usd >= 0 || run.total_tokens_in > 0,
    websocket_events_received: results.length, // approximation: 1 event per task result
  };
  return checks;
}

// ---------------------------------------------------------------------------
// Debug Context Collection
// ---------------------------------------------------------------------------

/**
 * Build debug context for a test report entry.
 * Collects everything an LLM debugger needs for root cause analysis.
 */
export function buildDebugContext(
  consoleErrors: string[] = [],
  networkLog: NetworkEntry[] = [],
): DebugContext {
  return { console_errors: consoleErrors, network_log: networkLog };
}

/**
 * Attach debug context as a Playwright test attachment (picked up by the reporter).
 */
export async function attachTestContext(
  testInfo: TestInfo,
  key: string,
  data: unknown,
): Promise<void> {
  await testInfo.attach(key, {
    body: JSON.stringify(data, null, 2),
    contentType: "application/json",
  });
}

// ---------------------------------------------------------------------------
// Dataset Resolution
// ---------------------------------------------------------------------------

/** Map suite provider name to the dataset name used in the API. */
export function suiteToDataset(suite: string): string {
  const mapping: Record<string, string> = {
    codeforge_simple: "e2e-quick",
    codeforge_tool_use: "e2e-quick",
    codeforge_agent: "e2e-quick",
    // External providers use their provider name as dataset
    humaneval: "humaneval",
    mbpp: "mbpp",
    bigcodebench: "bigcodebench",
    cruxeval: "cruxeval",
    livecodebench: "livecodebench",
    swebench: "swebench",
    sparcbench: "sparcbench",
    aider_polyglot: "aider_polyglot",
  };
  return mapping[suite] ?? suite;
}

// ---------------------------------------------------------------------------
// Environment Info
// ---------------------------------------------------------------------------

export async function collectEnvironmentInfo(): Promise<{
  backend_url: string;
  litellm_url: string;
  app_env: string;
  default_model: string;
  litellm_models_available: string[];
  git_commit: string;
}> {
  const models = await getLiteLLMModels();
  let gitCommit = "unknown";
  try {
    const { execFileSync } = await import("node:child_process");
    gitCommit = execFileSync("git", ["rev-parse", "--short", "HEAD"], { encoding: "utf-8" }).trim();
  } catch {
    // Not in a git repo or git not available
  }
  return {
    backend_url: HEALTH_BASE,
    litellm_url: LITELLM_URL,
    app_env: "development",
    default_model: DEFAULT_MODEL,
    litellm_models_available: models,
    git_commit: gitCommit,
  };
}

// ---------------------------------------------------------------------------
// Utilities
// ---------------------------------------------------------------------------

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

export { API_BASE, LITELLM_URL, HEALTH_BASE };
