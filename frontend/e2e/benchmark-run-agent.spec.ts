import { expect, test, ensureBenchmarkPage } from "./benchmark-fixtures";

test.describe("Benchmark Runs - Agent Mode", () => {
  test.beforeEach(async ({ page }) => {
    const loaded = await ensureBenchmarkPage(page);
    if (!loaded) test.skip();
  });

  test("selecting agent type shows exec_mode dropdown", async ({ page }) => {
    await page.getByRole("button", { name: "New Run" }).click();
    await page.locator("#benchmark-type").selectOption("agent");
    await expect(page.locator("#benchmark-exec-mode")).toBeVisible();
  });

  test("exec_mode dropdown has mount/sandbox/hybrid options", async ({ page }) => {
    await page.getByRole("button", { name: "New Run" }).click();
    await page.locator("#benchmark-type").selectOption("agent");
    const select = page.locator("#benchmark-exec-mode");
    await expect(select.locator("option[value='mount']")).toBeAttached();
    await expect(select.locator("option[value='sandbox']")).toBeAttached();
    await expect(select.locator("option[value='hybrid']")).toBeAttached();
  });

  test("exec_mode defaults to mount", async ({ page }) => {
    await page.getByRole("button", { name: "New Run" }).click();
    await page.locator("#benchmark-type").selectOption("agent");
    await expect(page.locator("#benchmark-exec-mode")).toHaveValue("mount");
  });

  test("switching from agent back to simple hides exec_mode", async ({ page }) => {
    await page.getByRole("button", { name: "New Run" }).click();
    await page.locator("#benchmark-type").selectOption("agent");
    await expect(page.locator("#benchmark-exec-mode")).toBeVisible();
    await page.locator("#benchmark-type").selectOption("simple");
    await expect(page.locator("#benchmark-exec-mode")).not.toBeVisible();
  });

  test("create agent run with mount exec_mode via API", async ({ benchApi }) => {
    const run = await benchApi.createRun({
      dataset: "basic-coding",
      model: "test/agent-mount",
      metrics: ["correctness"],
      benchmark_type: "agent",
      exec_mode: "mount",
    });
    expect(run.id).toBeTruthy();
    expect(run.status).toBe("running");
  });

  test("create agent run with sandbox exec_mode via API", async ({ benchApi }) => {
    const run = await benchApi.createRun({
      dataset: "basic-coding",
      model: "test/agent-sandbox",
      metrics: ["correctness"],
      benchmark_type: "agent",
      exec_mode: "sandbox",
    });
    expect(run.id).toBeTruthy();
  });

  test("create agent run with hybrid exec_mode via API", async ({ benchApi }) => {
    const run = await benchApi.createRun({
      dataset: "basic-coding",
      model: "test/agent-hybrid",
      metrics: ["correctness"],
      benchmark_type: "agent",
      exec_mode: "hybrid",
    });
    expect(run.id).toBeTruthy();
  });

  test("agent run shows type badge in list", async ({ page, benchApi }) => {
    await benchApi.createRun({
      dataset: "basic-coding",
      model: "test/agent-badge",
      metrics: ["correctness"],
      benchmark_type: "agent",
      exec_mode: "mount",
    });
    await page.reload();
    await page.locator("main h1").waitFor({ state: "visible", timeout: 5_000 });
    await expect(page.getByText("agent").first()).toBeVisible({ timeout: 10_000 });
  });

  test("agent run shows exec_mode badge in list", async ({ page, benchApi }) => {
    await benchApi.createRun({
      dataset: "basic-coding",
      model: "test/agent-exec-badge",
      metrics: ["correctness"],
      benchmark_type: "agent",
      exec_mode: "sandbox",
    });
    await page.reload();
    await page.locator("main h1").waitFor({ state: "visible", timeout: 5_000 });
    await expect(page.getByText("sandbox").first()).toBeVisible({ timeout: 10_000 });
  });

  test("agent run badge has info variant", async ({ page, benchApi }) => {
    await benchApi.createRun({
      dataset: "basic-coding",
      model: "test/agent-info",
      metrics: ["correctness"],
      benchmark_type: "agent",
      exec_mode: "mount",
    });
    await page.reload();
    await page.locator("main h1").waitFor({ state: "visible", timeout: 5_000 });
    const agentBadge = page.getByText("agent").first();
    await expect(agentBadge).toBeVisible({ timeout: 10_000 });
  });

  test("multiple agent runs with different exec_modes", async ({ benchApi }) => {
    const mount = await benchApi.createRun({
      dataset: "basic-coding",
      model: "test/multi-mount",
      metrics: ["correctness"],
      benchmark_type: "agent",
      exec_mode: "mount",
    });
    const sandbox = await benchApi.createRun({
      dataset: "basic-coding",
      model: "test/multi-sandbox",
      metrics: ["correctness"],
      benchmark_type: "agent",
      exec_mode: "sandbox",
    });
    expect(mount.id).not.toBe(sandbox.id);
  });

  test("agent run can be deleted via API", async ({ benchApi }) => {
    const run = await benchApi.createRun({
      dataset: "basic-coding",
      model: "test/agent-delete",
      metrics: ["correctness"],
      benchmark_type: "agent",
      exec_mode: "mount",
    });
    await benchApi.deleteRun(run.id);
    const runs = await benchApi.listRuns();
    expect(runs.find((r) => r.id === run.id)).toBeUndefined();
  });
});
