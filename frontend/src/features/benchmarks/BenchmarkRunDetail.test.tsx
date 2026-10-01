import { render, screen } from "@solidjs/testing-library";
import { describe, expect, it, vi } from "vitest";

import type { BenchmarkResult } from "~/api/types";

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

import { I18nProvider } from "~/i18n";

import { BenchmarkRunDetail } from "./BenchmarkRunDetail";

const result: BenchmarkResult = {
  id: "r-1",
  run_id: "run-1",
  task_id: "t-1",
  task_name: "lru-cache",
  scores: { correctness: 0.8 },
  evaluation_errors: { llm_judge_error: "proxy down" },
  actual_output: "",
  expected_output: "",
  tool_calls: [],
  cost_usd: 0,
  tokens_in: 0,
  tokens_out: 0,
  duration_ms: 0,
  // api/types.ts declares a second BenchmarkResult (prompt benchmarks) that TypeScript merges in.
  content: "",
  model: "",
  latency_ms: 0,
};

// S6-G review, item 3: an evaluator error is shown as an error, not a score.
describe("BenchmarkRunDetail", () => {
  it("shows evaluation errors apart from the scores", () => {
    render(() => (
      <I18nProvider>
        <BenchmarkRunDetail results={[result]} loading={false} formatDuration={() => "0s"} />
      </I18nProvider>
    ));

    expect(screen.getByText("correctness: 0.800")).toBeTruthy();
    const error = screen.getByText("llm_judge_error: evaluation error");
    expect(error.getAttribute("title")).toBe("proxy down");
    expect(screen.queryByText(/llm_judge_error: 0/)).toBeNull();
  });
});
