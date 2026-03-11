/**
 * Custom Playwright Reporter for benchmark validation.
 * Generates LLM-optimized JSON debug reports per block and a full aggregated report.
 *
 * Reports written to: frontend/e2e/benchmark-validation/reports/
 */

import * as fs from "node:fs";
import * as path from "node:path";
import { fileURLToPath } from "node:url";
import type { Reporter, FullConfig, Suite, TestCase, TestResult } from "@playwright/test/reporter";
import type {
  BlockReport,
  BlockSummary,
  BlockStatus,
  EnvironmentInfo,
  FullReport,
  FailureSummary,
  MatrixEntry,
  TestReport,
  TestStatus,
} from "./types";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const REPORTS_DIR = path.resolve(__dirname, "reports");

interface TestData {
  blockName: string;
  testId: string;
  report: Partial<TestReport>;
  status: TestStatus;
  durationMs: number;
}

class BenchmarkValidationReporter implements Reporter {
  private tests: TestData[] = [];
  private startTime = 0;
  private environment: EnvironmentInfo | null = null;

  onBegin(_config: FullConfig, _suite: Suite): void {
    this.startTime = Date.now();
    fs.mkdirSync(REPORTS_DIR, { recursive: true });
    console.log("\n=== Benchmark Validation Reporter Started ===\n");
  }

  onTestEnd(test: TestCase, result: TestResult): void {
    const titlePath = test.titlePath();
    // Block name is the describe() title, e.g. "Block 1: Simple Benchmarks"
    const blockName = titlePath.find((t) => t.startsWith("Block ")) ?? "unknown";
    // Extract test ID from title, e.g. "[1.1]" or use full title
    const idMatch = test.title.match(/\[(\d+\.\d+)\]/);
    const testId = idMatch?.[1] ?? test.title;

    const status: TestStatus =
      result.status === "passed" ? "passed" : result.status === "skipped" ? "skipped" : "failed";

    // Extract attached debug context from test attachments
    const attachments: Record<string, unknown> = {};
    for (const attachment of result.attachments) {
      if (attachment.contentType === "application/json" && attachment.body) {
        try {
          attachments[attachment.name] = JSON.parse(attachment.body.toString("utf-8"));
        } catch {
          // Ignore parse errors
        }
      }
    }

    const report: Partial<TestReport> = {
      id: testId,
      name: test.title,
      status,
      duration_ms: result.duration,
      ...(attachments["request"]
        ? { request: attachments["request"] as TestReport["request"] }
        : {}),
      ...(attachments["response"]
        ? { response: attachments["response"] as TestReport["response"] }
        : {}),
      ...(attachments["run_result"]
        ? { run_result: attachments["run_result"] as TestReport["run_result"] }
        : {}),
      ...(attachments["frontend_checks"]
        ? { frontend_checks: attachments["frontend_checks"] as TestReport["frontend_checks"] }
        : {}),
      ...(attachments["debug_context"]
        ? { debug_context: attachments["debug_context"] as TestReport["debug_context"] }
        : {}),
    };

    // Capture failure info
    if (status === "failed" && result.error) {
      report.failure = {
        assertion: result.error.message?.split("\n")[0] ?? "unknown",
        message: result.error.message ?? "unknown error",
        screenshot: result.attachments.find((a) => a.contentType?.startsWith("image/"))?.path,
      };
    }

    // Capture environment info from first test
    if (!this.environment && attachments["environment"]) {
      this.environment = attachments["environment"] as EnvironmentInfo;
    }

    this.tests.push({ blockName, testId, report, status, durationMs: result.duration });
  }

  async onEnd(): Promise<void> {
    const totalDuration = Date.now() - this.startTime;

    // Group tests by block
    const blockMap = new Map<string, TestData[]>();
    for (const t of this.tests) {
      const existing = blockMap.get(t.blockName) ?? [];
      existing.push(t);
      blockMap.set(t.blockName, existing);
    }

    const blockReports: BlockReport[] = [];
    const allFailures: FailureSummary[] = [];

    // Write per-block reports
    for (const [blockName, tests] of blockMap) {
      const summary = computeSummary(tests);
      const blockStatus = computeBlockStatus(summary);
      const slug = blockName
        .toLowerCase()
        .replace(/[^a-z0-9]+/g, "-")
        .replace(/-+$/, "");

      const blockReport: BlockReport = {
        block: {
          name: slug,
          status: blockStatus,
          started_at: new Date(this.startTime).toISOString(),
          finished_at: new Date().toISOString(),
          duration_ms: tests.reduce((sum, t) => sum + t.durationMs, 0),
          summary,
        },
        environment: this.environment ?? emptyEnvironment(),
        tests: tests.map((t) => fillTestReport(t)),
      };

      blockReports.push(blockReport);
      const filePath = path.join(REPORTS_DIR, `${slug}.json`);
      fs.writeFileSync(filePath, JSON.stringify(blockReport, null, 2));
      console.log(`Report written: ${filePath}`);

      // Collect failures
      for (const t of tests) {
        if (t.status === "failed") {
          allFailures.push({
            test_id: t.testId,
            one_line: `${t.report.name}: ${t.report.failure?.assertion ?? "unknown failure"}`,
            root_hint: t.report.failure?.message?.slice(0, 200) ?? "no details",
          });
        }
      }
    }

    // Write full report
    const fullReport: FullReport = {
      generated_at: new Date().toISOString(),
      total_duration_ms: totalDuration,
      git_commit: this.environment?.git_commit ?? "unknown",
      summary: {
        blocks_total: blockReports.length,
        blocks_passed: blockReports.filter((b) => b.block.status === "passed").length,
        blocks_failed: blockReports.filter((b) => b.block.status === "failed").length,
        tests_total: this.tests.length,
        tests_passed: this.tests.filter((t) => t.status === "passed").length,
        tests_failed: this.tests.filter((t) => t.status === "failed").length,
        tests_skipped: this.tests.filter((t) => t.status === "skipped").length,
        total_llm_cost_usd: 0, // Summed from run results if available
        total_llm_tokens: 0,
      },
      matrix: buildMatrix(this.tests),
      difficulty_audit: [], // Filled by block-0 test
      failures: allFailures,
      recommendations: generateRecommendations(allFailures, blockReports),
    };

    // Sum costs from run results
    for (const t of this.tests) {
      const runResult = t.report.run_result;
      if (runResult) {
        fullReport.summary.total_llm_cost_usd += runResult.total_cost ?? 0;
        fullReport.summary.total_llm_tokens += runResult.total_tokens ?? 0;
      }
    }

    const fullPath = path.join(REPORTS_DIR, "full-report.json");
    fs.writeFileSync(fullPath, JSON.stringify(fullReport, null, 2));
    console.log(`\nFull report written: ${fullPath}`);
    console.log(
      `\nSummary: ${fullReport.summary.tests_passed}/${fullReport.summary.tests_total} passed, ${fullReport.summary.tests_failed} failed, ${fullReport.summary.tests_skipped} skipped`,
    );
    console.log(`Duration: ${(totalDuration / 1000).toFixed(1)}s`);
    if (allFailures.length > 0) {
      console.log(`\nFailures:`);
      for (const f of allFailures) {
        console.log(`  - [${f.test_id}] ${f.one_line}`);
      }
    }
    console.log("\n=== Benchmark Validation Reporter Finished ===\n");
  }
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function computeSummary(tests: TestData[]): BlockSummary {
  return {
    total: tests.length,
    passed: tests.filter((t) => t.status === "passed").length,
    failed: tests.filter((t) => t.status === "failed").length,
    skipped: tests.filter((t) => t.status === "skipped").length,
  };
}

function computeBlockStatus(summary: BlockSummary): BlockStatus {
  if (summary.failed === 0 && summary.passed > 0) return "passed";
  if (summary.passed === 0 && summary.failed > 0) return "failed";
  if (summary.passed > 0 && summary.failed > 0) return "partial";
  return "skipped";
}

function fillTestReport(t: TestData): TestReport {
  return {
    id: t.testId,
    name: t.report.name ?? "unknown",
    status: t.status,
    suite: "",
    benchmark_type: "",
    metrics: [],
    model: "",
    duration_ms: t.durationMs,
    request: t.report.request ?? { method: "", url: "", body: {} },
    response: t.report.response ?? { status_code: 0, body: {} },
    run_result: t.report.run_result,
    frontend_checks: t.report.frontend_checks ?? {
      progress_bar_appeared: false,
      status_transition: [],
      scores_displayed: false,
      cost_displayed: false,
      websocket_events_received: 0,
    },
    failure: t.report.failure,
    debug_context: t.report.debug_context ?? { console_errors: [], network_log: [] },
  };
}

function emptyEnvironment(): EnvironmentInfo {
  return {
    backend_url: "http://localhost:8080",
    litellm_url: "http://localhost:4000",
    app_env: "development",
    default_model: "unknown",
    litellm_models_available: [],
    git_commit: "unknown",
  };
}

function buildMatrix(tests: TestData[]): MatrixEntry[] {
  const byProvider = new Map<string, Record<string, TestStatus>>();
  for (const t of tests) {
    // Extract suite and type from attachments or test name
    const req = t.report.request?.body as Record<string, unknown> | undefined;
    if (!req?.dataset) continue;
    const suite = String(req.dataset);
    const type = String(req.benchmark_type ?? "simple");
    const metrics = (req.metrics as string[] | undefined)?.join("+") ?? "unknown";
    const key = `${type}_${metrics}`;

    const existing = byProvider.get(suite) ?? {};
    existing[key] = t.status;
    byProvider.set(suite, existing);
  }
  return Array.from(byProvider.entries()).map(([suite, results]) => ({ suite, results }));
}

function generateRecommendations(failures: FailureSummary[], blocks: BlockReport[]): string[] {
  const recs: string[] = [];
  if (failures.length === 0) {
    recs.push("All tests passed - benchmark system is fully operational");
    return recs;
  }
  for (const f of failures) {
    if (f.one_line.includes("timeout") || f.one_line.includes("Timeout")) {
      recs.push(`${f.test_id}: Consider increasing timeout for local models`);
    }
    if (f.one_line.includes("functional_test") && f.one_line.includes("simple")) {
      recs.push(
        `${f.test_id}: functional_test on simple runs returns score=0 by design -- verify UI handles this`,
      );
    }
  }
  for (const b of blocks) {
    if (b.block.status === "failed" && b.block.name.includes("agent")) {
      recs.push("Agent benchmarks failed -- may need higher max_iterations for local models");
    }
  }
  return recs;
}

export default BenchmarkValidationReporter;
