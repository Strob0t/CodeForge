/**
 * Extended E2E fixture for benchmark tests.
 * Provides API helpers for creating/deleting benchmark runs and suites,
 * with automatic cleanup on teardown.
 */
import { test as base } from "./fixtures";
import { apiFetch, apiPost, apiDelete, apiGet } from "./helpers/api-helpers";

// --- Benchmark Types ---

export interface BenchmarkRun {
  id: string;
  dataset: string;
  model: string;
  status: string;
  metrics: string[];
  benchmark_type?: string;
  exec_mode?: string;
  total_cost: number;
  total_duration_ms: number;
  summary_scores: Record<string, number>;
}

export interface BenchmarkSuite {
  id: string;
  name: string;
  description?: string;
  type: string;
  provider_name: string;
  task_count: number;
}

export interface BenchmarkResult {
  task_id: string;
  task_name: string;
  scores: Record<string, number>;
  cost_usd: number;
  tokens_in: number;
  tokens_out: number;
  duration_ms: number;
  actual_output?: string;
  tool_calls?: Array<Record<string, string>>;
}

export interface BenchmarkDatasetInfo {
  name: string;
  path: string;
  task_count: number;
  description?: string;
}

// --- Bench API Helper ---

export interface BenchApiHelper {
  createRun(data: {
    dataset: string;
    model: string;
    metrics: string[];
    benchmark_type?: string;
    exec_mode?: string;
    suite_id?: string;
  }): Promise<BenchmarkRun>;
  deleteRun(id: string): Promise<void>;
  listRuns(): Promise<BenchmarkRun[]>;
  getRun(id: string): Promise<BenchmarkRun>;
  cancelRun(id: string): Promise<BenchmarkRun>;
  listResults(runId: string): Promise<BenchmarkResult[]>;

  createSuite(data: {
    name: string;
    description?: string;
    type: string;
    provider_name: string;
  }): Promise<BenchmarkSuite>;
  updateSuite(
    id: string,
    data: { name?: string; description?: string; type?: string; provider_name?: string },
  ): Promise<BenchmarkSuite>;
  deleteSuite(id: string): Promise<void>;
  listSuites(): Promise<BenchmarkSuite[]>;

  listDatasets(): Promise<BenchmarkDatasetInfo[]>;
  compareMulti(runIds: string[]): Promise<unknown[]>;
  costAnalysis(runId: string): Promise<unknown>;
  leaderboard(suiteId?: string): Promise<unknown[]>;

  exportResultsUrl(runId: string, format: "json" | "csv"): string;
  exportTrainingUrl(runId: string, format: "json" | "jsonl"): string;

  cleanup(): Promise<void>;
}

export const test = base.extend<{ benchApi: BenchApiHelper }>({
  benchApi: async ({}, use) => {
    const createdRuns: string[] = [];
    const createdSuites: string[] = [];

    const helper: BenchApiHelper = {
      async createRun(data) {
        const run = await apiPost<BenchmarkRun>("/benchmarks/runs", data);
        createdRuns.push(run.id);
        return run;
      },

      async deleteRun(id: string) {
        await apiDelete(`/benchmarks/runs/${encodeURIComponent(id)}`);
        const idx = createdRuns.indexOf(id);
        if (idx >= 0) createdRuns.splice(idx, 1);
      },

      async listRuns() {
        return apiGet<BenchmarkRun[]>("/benchmarks/runs");
      },

      async getRun(id: string) {
        return apiGet<BenchmarkRun>(`/benchmarks/runs/${encodeURIComponent(id)}`);
      },

      async cancelRun(id: string) {
        const res = await apiFetch(`/benchmarks/runs/${encodeURIComponent(id)}`, {
          method: "PATCH",
        });
        if (!res.ok) throw new Error(`Cancel run failed (${res.status}): ${await res.text()}`);
        return (await res.json()) as BenchmarkRun;
      },

      async listResults(runId: string) {
        return apiGet<BenchmarkResult[]>(`/benchmarks/runs/${encodeURIComponent(runId)}/results`);
      },

      async createSuite(data) {
        const suite = await apiPost<BenchmarkSuite>("/benchmarks/suites", data);
        createdSuites.push(suite.id);
        return suite;
      },

      async updateSuite(id, data) {
        const res = await apiFetch(`/benchmarks/suites/${encodeURIComponent(id)}`, {
          method: "PUT",
          body: JSON.stringify(data),
        });
        if (!res.ok) throw new Error(`Update suite failed (${res.status}): ${await res.text()}`);
        return (await res.json()) as BenchmarkSuite;
      },

      async deleteSuite(id: string) {
        await apiDelete(`/benchmarks/suites/${encodeURIComponent(id)}`);
        const idx = createdSuites.indexOf(id);
        if (idx >= 0) createdSuites.splice(idx, 1);
      },

      async listSuites() {
        return apiGet<BenchmarkSuite[]>("/benchmarks/suites");
      },

      async listDatasets() {
        return apiGet<BenchmarkDatasetInfo[]>("/benchmarks/datasets");
      },

      async compareMulti(runIds: string[]) {
        return apiPost<unknown[]>("/benchmarks/compare-multi", { run_ids: runIds });
      },

      async costAnalysis(runId: string) {
        return apiGet<unknown>(`/benchmarks/runs/${encodeURIComponent(runId)}/cost-analysis`);
      },

      async leaderboard(suiteId?: string) {
        const qs = suiteId ? `?suite_id=${encodeURIComponent(suiteId)}` : "";
        return apiGet<unknown[]>(`/benchmarks/leaderboard${qs}`);
      },

      exportResultsUrl(runId: string, format: "json" | "csv") {
        return `/benchmarks/runs/${encodeURIComponent(runId)}/export/results?format=${format}`;
      },

      exportTrainingUrl(runId: string, format: "json" | "jsonl") {
        return `/benchmarks/runs/${encodeURIComponent(runId)}/export/training?format=${format}`;
      },

      async cleanup() {
        for (const id of [...createdRuns]) {
          try {
            await apiDelete(`/benchmarks/runs/${encodeURIComponent(id)}`);
          } catch {
            // best-effort
          }
        }
        for (const id of [...createdSuites]) {
          try {
            await apiDelete(`/benchmarks/suites/${encodeURIComponent(id)}`);
          } catch {
            // best-effort
          }
        }
        createdRuns.length = 0;
        createdSuites.length = 0;
      },
    };

    await use(helper);

    // Automatic teardown
    await helper.cleanup();
  },
});

export { expect } from "@playwright/test";

// --- Page helpers ---

/** Navigate to the benchmarks page and verify it loaded (dev mode required). */
export async function ensureBenchmarkPage(page: import("@playwright/test").Page): Promise<boolean> {
  await page.goto("/benchmarks");
  try {
    await page.locator("main h1").waitFor({ state: "visible", timeout: 5_000 });
    const text = await page.locator("main h1").textContent();
    return text?.includes("Benchmark") ?? false;
  } catch {
    return false;
  }
}

/** Click a tab on the benchmark page. */
export async function clickTab(page: import("@playwright/test").Page, tabName: string) {
  await page.getByRole("tab", { name: tabName }).click();
}
