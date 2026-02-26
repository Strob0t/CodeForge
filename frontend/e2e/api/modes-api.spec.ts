import { test, expect } from "@playwright/test";
import { apiLogin, API_BASE } from "../helpers/api-helpers";

test.describe("Modes API", () => {
  let token: string;
  const createdModeIds: string[] = [];

  test.beforeAll(async () => {
    const auth = await apiLogin("admin@localhost", "Changeme123");
    token = auth.accessToken;
  });

  test.afterAll(async () => {
    for (const id of createdModeIds) {
      try {
        await fetch(`${API_BASE}/modes/${id}`, {
          method: "DELETE",
          headers: { Authorization: `Bearer ${token}` },
        });
      } catch {
        // best-effort cleanup
      }
    }
  });

  const headers = () => ({ Authorization: `Bearer ${token}` });

  test("list modes returns array with built-ins", async ({ request }) => {
    const res = await request.get(`${API_BASE}/modes`, { headers: headers() });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(Array.isArray(body)).toBe(true);
    expect(body.length).toBeGreaterThan(0);
  });

  test("list scenarios returns array", async ({ request }) => {
    const res = await request.get(`${API_BASE}/modes/scenarios`, { headers: headers() });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(Array.isArray(body)).toBe(true);
  });

  test("get mode by ID", async ({ request }) => {
    // First get the list to find a built-in mode ID
    const listRes = await request.get(`${API_BASE}/modes`, { headers: headers() });
    const modes = await listRes.json();
    expect(modes.length).toBeGreaterThan(0);

    const modeId = modes[0].id;
    const res = await request.get(`${API_BASE}/modes/${modeId}`, { headers: headers() });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(body.id).toBe(modeId);
  });

  test("create custom mode returns 201", async ({ request }) => {
    const id = `e2e-mode-${Date.now()}`;
    const res = await request.post(`${API_BASE}/modes`, {
      headers: headers(),
      data: {
        id,
        name: `E2E Test Mode ${Date.now()}`,
        description: "Created by E2E test",
        tools: ["read", "write"],
        llm_scenario: "default",
        autonomy: 2,
      },
    });
    expect(res.status()).toBe(201);
    const body = await res.json();
    expect(body.id).toBe(id);
    createdModeIds.push(id);
  });

  test("update custom mode", async ({ request }) => {
    const id = `e2e-upd-mode-${Date.now()}`;
    await request.post(`${API_BASE}/modes`, {
      headers: headers(),
      data: {
        id,
        name: "Update Test Mode",
        description: "Will be updated",
        tools: ["read"],
        llm_scenario: "default",
        autonomy: 1,
      },
    });
    createdModeIds.push(id);

    const res = await request.put(`${API_BASE}/modes/${id}`, {
      headers: headers(),
      data: {
        id,
        name: "Updated Mode Name",
        description: "Updated description",
        tools: ["read", "write", "bash"],
        llm_scenario: "default",
        autonomy: 3,
      },
    });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(body.name).toBe("Updated Mode Name");
  });

  test("built-in mode has expected fields", async ({ request }) => {
    const listRes = await request.get(`${API_BASE}/modes`, { headers: headers() });
    const modes = await listRes.json();
    const builtIn = modes.find((m: { builtin: boolean }) => m.builtin === true);

    if (builtIn) {
      expect(builtIn.id).toBeTruthy();
      expect(builtIn.name).toBeTruthy();
      expect(builtIn.builtin).toBe(true);
      expect(typeof builtIn.autonomy).toBe("number");
    } else {
      // All modes should have standard fields
      expect(modes[0].id).toBeTruthy();
      expect(modes[0].name).toBeTruthy();
    }
  });

  test("create mode validation requires id and name", async ({ request }) => {
    const res = await request.post(`${API_BASE}/modes`, {
      headers: headers(),
      data: { description: "no id or name" },
    });
    expect(res.status()).toBe(400);
  });
});
