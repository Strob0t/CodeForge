import { test, expect } from "@playwright/test";
import { apiLogin, API_BASE } from "../helpers/api-helpers";
import { checkLLMHealth, discoverAvailableModels, extractProviders } from "./llm-helpers";

/**
 * Suite prerequisites: verify the full stack is healthy and LLM providers are available.
 * If these tests fail, subsequent LLM tests should be skipped.
 */
test.describe("LLM E2E — Prerequisites", () => {
  let token: string;

  test.beforeAll(async () => {
    const auth = await apiLogin("admin@localhost", "Changeme123");
    token = auth.accessToken;
  });

  test("backend is healthy and in development mode", async () => {
    const res = await fetch(`${API_BASE.replace("/api/v1", "")}/health`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    expect(res.status).toBe(200);
    const body = (await res.json()) as { status: string; dev_mode?: boolean };
    expect(body.status).toBe("ok");
    expect(body.dev_mode).toBe(true);
  });

  test("LiteLLM proxy is healthy", async () => {
    const health = await checkLLMHealth();
    expect(health.status).toBe("healthy");
  });

  test("at least one LLM provider is configured", async () => {
    const discovery = await discoverAvailableModels();
    expect(discovery.count).toBeGreaterThanOrEqual(1);
    expect(discovery.models.length).toBeGreaterThanOrEqual(1);
  });

  test("at least one discovered model is reachable", async () => {
    const discovery = await discoverAvailableModels();
    const reachable = discovery.models.filter((m) => m.status === "reachable");
    expect(reachable.length).toBeGreaterThanOrEqual(1);
  });

  test("log available providers for this test run", async () => {
    const discovery = await discoverAvailableModels();
    const providers = extractProviders(discovery.models);
    const reachable = discovery.models.filter((m) => m.status === "reachable");

    console.log("=== LLM E2E Provider Discovery ===");
    console.log(`Total models discovered: ${discovery.count}`);
    console.log(`Reachable models: ${reachable.length}`);
    console.log(`Providers: ${providers.join(", ")}`);
    for (const model of reachable) {
      console.log(`  - ${model.model_name} (${model.provider})`);
    }
    console.log("==================================");

    // This test always passes — its purpose is logging
    expect(providers.length).toBeGreaterThanOrEqual(0);
  });

  test("model refresh endpoint works", async () => {
    const res = await fetch(`${API_BASE}/llm/refresh`, {
      method: "POST",
      headers: { Authorization: `Bearer ${token}` },
    });
    expect([200, 204]).toContain(res.status);
  });
});
