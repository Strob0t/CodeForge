import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e",
  fullyParallel: false,
  workers: 1,
  retries: 0,
  globalSetup: "./e2e/global-setup.ts",
  reporter: [["html", { outputFolder: "e2e-report", open: "never" }]],
  outputDir: "e2e-results",
  use: {
    baseURL: "http://localhost:3000",
    storageState: "./e2e/.auth/admin.json",
    trace: "on-first-retry",
    ignoreHTTPSErrors: true,
  },
  projects: [
    {
      name: "chromium",
      use: { browserName: "chromium" },
    },
  ],
});
