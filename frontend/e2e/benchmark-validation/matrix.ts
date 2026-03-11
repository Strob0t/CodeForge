/**
 * Declarative test combination matrix for benchmark validation.
 * Derived from the spec: docs/superpowers/specs/2026-03-11-benchmark-validation-design.md
 *
 * Each entry maps to one benchmark run. Spec files filter by block number.
 */

import type { TestCase, ErrorTestCase } from "./types";

export const DEFAULT_MODEL = "lm_studio/qwen3-30b-a3b";

/** All valid suite x benchmark-type x metric combinations. */
export const VALIDATION_MATRIX: TestCase[] = [
  // --- Block 1: Simple Benchmarks ---
  // Uses e2e-quick dataset (2 tasks) for fast validation.
  // External providers (humaneval, mbpp, etc.) have 100+ tasks each — too slow for E2E.
  { id: "1.1", block: 1, suite: "codeforge_simple", type: "simple", metrics: ["llm_judge"] },
  {
    id: "1.2",
    block: 1,
    suite: "codeforge_simple",
    type: "simple",
    metrics: ["functional_test"],
    expectation: "Graceful degradation: score=0, no crash",
  },
  {
    id: "1.3",
    block: 1,
    suite: "codeforge_simple",
    type: "simple",
    metrics: ["llm_judge", "functional_test"],
    expectation: "Both evaluators produce scores (functional_test may be 0)",
  },

  // --- Block 2: Tool-Use Benchmarks ---
  { id: "2.1", block: 2, suite: "codeforge_tool_use", type: "tool_use", metrics: ["llm_judge"] },
  {
    id: "2.2",
    block: 2,
    suite: "codeforge_tool_use",
    type: "tool_use",
    metrics: ["functional_test"],
  },
  {
    id: "2.3",
    block: 2,
    suite: "codeforge_tool_use",
    type: "tool_use",
    metrics: ["llm_judge", "functional_test"],
  },

  // --- Block 3: Agent Benchmarks ---
  {
    id: "3.1",
    block: 3,
    suite: "codeforge_agent",
    type: "agent",
    metrics: ["llm_judge", "trajectory_verifier"],
  },
  { id: "3.2", block: 3, suite: "codeforge_agent", type: "agent", metrics: ["llm_judge", "sparc"] },
  {
    id: "3.3",
    block: 3,
    suite: "codeforge_agent",
    type: "agent",
    metrics: ["llm_judge", "functional_test", "sparc", "trajectory_verifier"],
    expectation: "All evaluator scores present (functional_test may be 0)",
  },
  // External agent suites (swebench, sparcbench, aider_polyglot) have 100+ tasks each.
  // At ~4 min/task with local models, these would take hours — too slow for E2E.
  // Registration is validated in Block 0 (difficulty audit).

  // --- Block 4: Routing ---
  {
    id: "4.1",
    block: 4,
    suite: "codeforge_simple",
    type: "simple",
    metrics: ["llm_judge"],
    model: "auto",
  },
];

/** Error scenarios for Block 5. */
export const ERROR_SCENARIOS: ErrorTestCase[] = [
  {
    id: "5.1",
    name: "Invalid dataset",
    params: {
      dataset: "nonexistent-dataset-xyz",
      model: DEFAULT_MODEL,
      metrics: ["llm_judge"],
      benchmark_type: "simple",
    },
    expectation: "Run status = failed, error message present",
  },
  {
    id: "5.2",
    name: "Invalid model",
    params: {
      dataset: "basic-coding",
      model: "nonexistent/model-xyz",
      metrics: ["llm_judge"],
      benchmark_type: "simple",
    },
    expectation: "Run status = failed, clear error",
  },
  {
    id: "5.3",
    name: "Empty dataset (0 tasks)",
    params: {
      dataset: "empty-test",
      model: DEFAULT_MODEL,
      metrics: ["llm_judge"],
      benchmark_type: "simple",
    },
    expectation: "Graceful handling, no infinite loop",
  },
  {
    id: "5.4",
    name: "Unknown evaluator",
    params: {
      dataset: "e2e-quick",
      model: DEFAULT_MODEL,
      metrics: ["nonexistent_evaluator"],
      benchmark_type: "simple",
    },
    expectation: "Graceful degradation: falls back to llm_judge",
  },
  {
    id: "5.5",
    name: "Duplicate run (same params)",
    params: {
      dataset: "e2e-quick",
      model: DEFAULT_MODEL,
      metrics: ["llm_judge"],
      benchmark_type: "simple",
    },
    expectation: "Idempotent, no double processing",
  },
];

/** External suites that need difficulty auditing. */
export const EXTERNAL_SUITES_FOR_AUDIT = [
  "humaneval",
  "mbpp",
  "bigcodebench",
  "cruxeval",
  "livecodebench",
  "swebench",
  "sparcbench",
  "aider_polyglot",
];

/** Get test cases for a specific block. */
export function getBlockCases(block: number): TestCase[] {
  return VALIDATION_MATRIX.filter((tc) => tc.block === block);
}
