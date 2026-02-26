import { test, expect } from "@playwright/test";
import { apiLogin, API_BASE } from "../helpers/api-helpers";

test.describe("LLM API", () => {
  let token: string;

  test.beforeAll(async () => {
    const auth = await apiLogin("admin@localhost", "Changeme123");
    token = auth.accessToken;
  });

  const headers = () => ({ Authorization: `Bearer ${token}` });

  test("list models returns array", async ({ request }) => {
    const res = await request.get(`${API_BASE}/llm/models`, { headers: headers() });
    // 200 if LiteLLM is up, 502 if not — both are valid
    expect([200, 502]).toContain(res.status());
    if (res.status() === 200) {
      const body = await res.json();
      expect(Array.isArray(body)).toBe(true);
    }
  });

  test("add model requires model_name", async ({ request }) => {
    const res = await request.post(`${API_BASE}/llm/models`, {
      headers: headers(),
      data: {},
    });
    expect(res.status()).toBe(400);
  });

  test("add model with valid data", async ({ request }) => {
    const res = await request.post(`${API_BASE}/llm/models`, {
      headers: headers(),
      data: {
        model_name: `e2e-test-model-${Date.now()}`,
        litellm_params: { model: "openai/gpt-4", api_key: "test-key" },
      },
    });
    // 201 if LiteLLM is up, 502 if not
    expect([201, 502]).toContain(res.status());
  });

  test("delete model requires id", async ({ request }) => {
    const res = await request.post(`${API_BASE}/llm/models/delete`, {
      headers: headers(),
      data: {},
    });
    expect(res.status()).toBe(400);
  });

  test("delete model with id", async ({ request }) => {
    const res = await request.post(`${API_BASE}/llm/models/delete`, {
      headers: headers(),
      data: { id: "non-existent-model-id" },
    });
    // 200 if LiteLLM is up, 502 if not
    expect([200, 502]).toContain(res.status());
  });

  test("LLM health returns status field", async ({ request }) => {
    const res = await request.get(`${API_BASE}/llm/health`, { headers: headers() });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(body.status).toBeTruthy();
    expect(["healthy", "unhealthy"]).toContain(body.status);
  });

  test("discover models returns response", async ({ request }) => {
    const res = await request.get(`${API_BASE}/llm/discover`, { headers: headers() });
    // 200 if LiteLLM is up, 502 if not
    expect([200, 502]).toContain(res.status());
    if (res.status() === 200) {
      const body = await res.json();
      expect(body.models).toBeDefined();
      expect(typeof body.count).toBe("number");
    }
  });

  test("list models returns array structure", async ({ request }) => {
    const res = await request.get(`${API_BASE}/llm/models`, { headers: headers() });
    if (res.status() === 200) {
      const body = await res.json();
      expect(Array.isArray(body)).toBe(true);
    } else {
      expect(res.status()).toBe(502);
    }
  });
});
