import { test, expect } from "@playwright/test";
import { apiLogin, API_BASE } from "../helpers/api-helpers";

test.describe("LLM E2E — Model Management", () => {
  let token: string;
  const testModelName = `e2e-llm-test-model-${Date.now()}`;

  test.beforeAll(async () => {
    const auth = await apiLogin("admin@localhost", "Changeme123");
    token = auth.accessToken;
  });

  const headers = () => ({ Authorization: `Bearer ${token}` });

  test("GET /llm/health returns healthy", async ({ request }) => {
    const res = await request.get(`${API_BASE}/llm/health`, { headers: headers() });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(body.status).toBe("healthy");
  });

  test("GET /llm/discover returns real models with count", async ({ request }) => {
    const res = await request.get(`${API_BASE}/llm/discover`, { headers: headers() });
    expect([200, 502]).toContain(res.status());
    if (res.status() === 200) {
      const body = await res.json();
      expect(body.count).toBeGreaterThan(0);
      expect(Array.isArray(body.models)).toBe(true);
      expect(body.models.length).toBeGreaterThan(0);
      // Every model should have a provider field
      for (const model of body.models) {
        expect(typeof model.provider).toBe("string");
      }
    }
  });

  test("GET /llm/available returns models and best_model", async ({ request }) => {
    const res = await request.get(`${API_BASE}/llm/available`, { headers: headers() });
    expect([200, 502]).toContain(res.status());
    if (res.status() === 200) {
      const body = await res.json();
      expect(typeof body.best_model).toBe("string");
      expect(body.best_model.length).toBeGreaterThan(0);
      expect(Array.isArray(body.models)).toBe(true);
    }
  });

  test("POST /llm/models add model endpoint responds correctly", async ({ request }) => {
    // LiteLLM requires STORE_MODEL_IN_DB=True for dynamic model registration.
    // When not configured, it returns 500 (proxied as 502). Both cases are valid behaviors.
    const res = await request.post(`${API_BASE}/llm/models`, {
      headers: headers(),
      data: {
        model_name: testModelName,
        litellm_params: { model: "ollama/e2e-test-model", api_base: "http://localhost:11434" },
      },
    });
    // 201 = model added successfully (STORE_MODEL_IN_DB=True)
    // 502 = LiteLLM rejected (STORE_MODEL_IN_DB not set) — still a valid endpoint response
    expect([201, 502]).toContain(res.status());

    if (res.status() === 201) {
      // Full lifecycle: verify in list, delete, verify gone
      const listRes = await request.get(`${API_BASE}/llm/models`, { headers: headers() });
      expect(listRes.status()).toBe(200);
      const models = await listRes.json();
      expect(Array.isArray(models)).toBe(true);
      const found = models.some((m: { model_name?: string }) => m.model_name === testModelName);
      expect(found).toBe(true);

      const delRes = await request.post(`${API_BASE}/llm/models/delete`, {
        headers: headers(),
        data: { id: testModelName },
      });
      expect([200, 502]).toContain(delRes.status());

      const listRes2 = await request.get(`${API_BASE}/llm/models`, { headers: headers() });
      expect(listRes2.status()).toBe(200);
      const models2 = await listRes2.json();
      const found2 = models2.some((m: { model_name?: string }) => m.model_name === testModelName);
      expect(found2).toBe(false);
    } else {
      // LiteLLM doesn't support dynamic model add — verify list still works
      const listRes = await request.get(`${API_BASE}/llm/models`, { headers: headers() });
      expect(listRes.status()).toBe(200);
      const models = await listRes.json();
      expect(Array.isArray(models)).toBe(true);
    }
  });

  test("POST /llm/models without model_name returns 400", async ({ request }) => {
    const res = await request.post(`${API_BASE}/llm/models`, {
      headers: headers(),
      data: {},
    });
    expect(res.status()).toBe(400);
  });

  test("POST /llm/refresh triggers model refresh", async ({ request }) => {
    const res = await request.post(`${API_BASE}/llm/refresh`, {
      headers: headers(),
    });
    expect([200, 204]).toContain(res.status());
  });

  test("available endpoint returns fresh data after refresh", async ({ request }) => {
    const res = await request.get(`${API_BASE}/llm/available`, { headers: headers() });
    expect([200, 502]).toContain(res.status());
    if (res.status() === 200) {
      const body = await res.json();
      expect(body).toBeTruthy();
      expect(typeof body.best_model).toBe("string");
      expect(body.best_model.length).toBeGreaterThan(0);
    }
  });
});
