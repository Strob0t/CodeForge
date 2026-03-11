import { defineConfig } from "@playwright/test";

/**
 * Playwright config for benchmark system validation.
 *
 * Differences from normal E2E tests:
 * - Sequential execution (workers: 1) because benchmark runs share backend state
 * - High timeouts (10 min per test) because local LM Studio models are slow
 * - Custom LLM-debug reporter that writes JSON reports per block
 * - No browser needed (API-level tests)
 */
export default defineConfig({
  testDir: ".",
  fullyParallel: false,
  workers: 1,
  retries: 0, // No retries — we want accurate failure data for the report
  reporter: [["list"], ["./reporter.ts"]],
  outputDir: "./test-results",
  timeout: 600_000, // 10 minutes per test (local models are slow)
  expect: {
    timeout: 300_000, // 5 minutes for polling assertions
  },
  use: {
    baseURL: "http://localhost:8080",
    trace: "on-first-retry",
  },
  projects: [
    {
      name: "benchmark-validation",
      testMatch: /block-\d+.*\.spec\.ts/,
    },
  ],
});
