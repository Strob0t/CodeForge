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

  let modelAdded = false;

  test("POST /llm/models adds model with real provider params", async ({ request }) => {
    const res = await request.post(`${API_BASE}/llm/models`, {
      headers: headers(),
      data: {
        model_name: testModelName,
        litellm_params: { model: "openai/gpt-4o-mini", api_key: "test-key" },
      },
    });
    // 201 if LiteLLM accepted, 502 if proxy down or rejected
    expect([201, 502]).toContain(res.status());
    modelAdded = res.status() === 201;
  });

  test("added model appears in model list", async ({ request }) => {
    test.skip(!modelAdded, "model add was not accepted by LiteLLM");
    const res = await request.get(`${API_BASE}/llm/models`, { headers: headers() });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(Array.isArray(body)).toBe(true);
    const found = body.some((m: { model_name?: string }) => m.model_name === testModelName);
    expect(found).toBe(true);
  });

  test("POST /llm/models/delete removes test model", async ({ request }) => {
    test.skip(!modelAdded, "model was not added");
    const res = await request.post(`${API_BASE}/llm/models/delete`, {
      headers: headers(),
      data: { id: testModelName },
    });
    expect([200, 502]).toContain(res.status());
  });

  test("deleted model no longer in list", async ({ request }) => {
    test.skip(!modelAdded, "model was not added");
    const res = await request.get(`${API_BASE}/llm/models`, { headers: headers() });
    expect([200, 502]).toContain(res.status());
    if (res.status() === 200) {
      const body = await res.json();
      expect(Array.isArray(body)).toBe(true);
      const found = body.some((m: { model_name?: string }) => m.model_name === testModelName);
      expect(found).toBe(false);
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
