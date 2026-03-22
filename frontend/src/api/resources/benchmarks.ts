import type { CoreClient } from "../core";
import { url } from "../factory";
import type {
  BenchmarkCompareResult,
  BenchmarkDatasetInfo,
  BenchmarkResult,
  BenchmarkRun,
  BenchmarkSuite,
  CostAnalysis,
  CreateBenchmarkRunRequest,
  LeaderboardEntry,
  MultiCompareEntry,
  PromptAnalysisReport,
} from "../types";

export function createBenchmarksResource(c: CoreClient) {
  return {
    listRuns: () => c.get<BenchmarkRun[]>("/benchmarks/runs"),

    getRun: (id: string) => c.get<BenchmarkRun>(url`/benchmarks/runs/${id}`),

    createRun: (data: CreateBenchmarkRunRequest) => c.post<BenchmarkRun>("/benchmarks/runs", data),

    deleteRun: (id: string) => c.del<undefined>(url`/benchmarks/runs/${id}`),

    listResults: (runId: string) =>
      c.get<BenchmarkResult[]>(url`/benchmarks/runs/${runId}/results`),

    compare: (runIdA: string, runIdB: string) =>
      c.post<BenchmarkCompareResult>("/benchmarks/compare", {
        run_id_a: runIdA,
        run_id_b: runIdB,
      }),

    listDatasets: () => c.get<BenchmarkDatasetInfo[]>("/benchmarks/datasets"),

    listSuites: () => c.get<BenchmarkSuite[]>("/benchmarks/suites"),

    createSuite: (data: {
      name: string;
      description?: string;
      type: string;
      provider_name: string;
    }) => c.post<BenchmarkSuite>("/benchmarks/suites", data),

    getSuite: (id: string) => c.get<BenchmarkSuite>(url`/benchmarks/suites/${id}`),

    deleteSuite: (id: string) => c.del<undefined>(url`/benchmarks/suites/${id}`),

    updateSuite: (
      id: string,
      data: { name?: string; description?: string; type?: string; provider_name?: string },
    ) => c.put<BenchmarkSuite>(url`/benchmarks/suites/${id}`, data),

    cancelRun: (id: string) => c.patch<BenchmarkRun>(url`/benchmarks/runs/${id}`),

    exportResultsUrl: (runId: string, format: "json" | "csv" = "json") =>
      `${c.BASE}/benchmarks/runs/${encodeURIComponent(runId)}/export/results?format=${format}`,

    compareMulti: (runIds: string[]) =>
      c.post<MultiCompareEntry[]>("/benchmarks/compare-multi", { run_ids: runIds }),

    costAnalysis: (runId: string) =>
      c.get<CostAnalysis>(url`/benchmarks/runs/${runId}/cost-analysis`),

    leaderboard: (suiteId?: string) =>
      c.get<LeaderboardEntry[]>(
        `/benchmarks/leaderboard${suiteId ? `?suite_id=${encodeURIComponent(suiteId)}` : ""}`,
      ),

    exportTrainingUrl: (runId: string, format: "json" | "jsonl" = "json") =>
      `${c.BASE}/benchmarks/runs/${encodeURIComponent(runId)}/export/training?format=${format}`,

    analyzeRun: (runId: string) =>
      c.post<PromptAnalysisReport>(url`/benchmarks/runs/${runId}/analyze`),
  };
}
