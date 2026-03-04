import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e",
  fullyParallel: false,
  workers: 1,
  retries: 0,
  reporter: [["list"]],
  outputDir: "e2e-results",
  use: {
    baseURL: "http://localhost:3000",
    storageState: "./e2e/.auth/admin.json",
    trace: "off",
    ignoreHTTPSErrors: true,
  },
  projects: [
    {
      name: "chromium",
      use: {
        browserName: "chromium",
        launchOptions: { args: ["--no-sandbox", "--disable-setuid-sandbox"] },
      },
    },
  ],
});
