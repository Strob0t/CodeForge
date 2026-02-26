import { test, expect } from "@playwright/test";
import { apiLogin, API_BASE } from "../helpers/api-helpers";

test.describe("Settings API", () => {
  let token: string;

  test.beforeAll(async () => {
    const auth = await apiLogin("admin@localhost", "Changeme123");
    token = auth.accessToken;
  });

  const headers = () => ({ Authorization: `Bearer ${token}` });

  test("GET /settings returns 200 with object", async () => {
    const res = await fetch(`${API_BASE}/settings`, { headers: headers() });
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(typeof body).toBe("object");
    expect(body).not.toBeNull();
  });

  test("PUT /settings updates a setting and returns 200", async () => {
    const res = await fetch(`${API_BASE}/settings`, {
      method: "PUT",
      headers: { ...headers(), "Content-Type": "application/json" },
      body: JSON.stringify({
        settings: { "test.e2e_key": JSON.parse('"e2e_value"') },
      }),
    });
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body.status).toBe("ok");
  });

  test("round-trip: PUT then GET returns updated value", async () => {
    const value = `round-trip-${Date.now()}`;
    // PUT
    const putRes = await fetch(`${API_BASE}/settings`, {
      method: "PUT",
      headers: { ...headers(), "Content-Type": "application/json" },
      body: JSON.stringify({
        settings: { "test.roundtrip": JSON.parse(`"${value}"`) },
      }),
    });
    expect(putRes.status).toBe(200);

    // GET
    const getRes = await fetch(`${API_BASE}/settings`, { headers: headers() });
    expect(getRes.status).toBe(200);
    const body = await getRes.json();
    expect(body["test.roundtrip"]).toBe(value);
  });

  test("PUT /settings with empty settings map returns 400", async () => {
    const res = await fetch(`${API_BASE}/settings`, {
      method: "PUT",
      headers: { ...headers(), "Content-Type": "application/json" },
      body: JSON.stringify({ settings: {} }),
    });
    expect(res.status).toBe(400);
  });
});
