import { test, expect } from "@playwright/test";
import { apiLogin, apiGet, API_BASE } from "../helpers/api-helpers";
import { checkLLMHealth } from "./llm-helpers";

/**
 * Final cleanup suite for the LLM E2E tests.
 * Removes all e2e-llm test projects and verifies the system is healthy after the run.
 */
test.describe("LLM E2E — Cleanup", () => {
  let token: string;

  test.beforeAll(async () => {
    const auth = await apiLogin("admin@localhost", "Changeme123");
    token = auth.accessToken;
  });

  test("delete all e2e-llm test projects", async () => {
    const projects = await apiGet<Array<{ id: string; name: string }>>("/projects", token);
    const e2eProjects = projects.filter((p) => p.name.startsWith("e2e-llm-"));

    for (const proj of e2eProjects) {
      const res = await fetch(`${API_BASE}/projects/${proj.id}`, {
        method: "DELETE",
        headers: { Authorization: `Bearer ${token}` },
      });
      // Accept any success or already-deleted status
      expect([200, 204, 404]).toContain(res.status);
    }
  });

  test("LiteLLM is still healthy after test suite", async () => {
    const health = await checkLLMHealth();
    expect(health.status).toBe("healthy");
  });

  test("no orphaned e2e-llm projects remain", async () => {
    const projects = await apiGet<Array<{ id: string; name: string }>>("/projects", token);
    const orphaned = projects.filter((p) => p.name.startsWith("e2e-llm-"));
    expect(orphaned).toHaveLength(0);
  });
});
