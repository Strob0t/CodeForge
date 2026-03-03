import { defineConfig } from "@playwright/test";

/**
 * Playwright config for LLM E2E tests.
 * These tests use direct HTTP/fetch calls (no browser UI needed),
 * so we skip global-setup (which requires a working browser for cookie auth).
 */
export default defineConfig({
  testDir: "./e2e/llm",
  fullyParallel: false,
  workers: 1,
  retries: 1,
  reporter: [["list"], ["html", { outputFolder: "e2e-report-llm", open: "never" }]],
  outputDir: "e2e-results-llm",
  timeout: 120_000,
  use: {
    baseURL: "http://localhost:3000",
    trace: "on-first-retry",
    ignoreHTTPSErrors: true,
  },
  projects: [
    {
      name: "llm-api",
      testMatch: /.*\.spec\.ts/,
    },
  ],
});
