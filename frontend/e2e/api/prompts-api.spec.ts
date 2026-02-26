import { test, expect } from "@playwright/test";
import { apiLogin, API_BASE } from "../helpers/api-helpers";

test.describe("Prompt Sections API", () => {
  let token: string;
  const createdNames: string[] = [];

  test.beforeAll(async () => {
    const auth = await apiLogin("admin@localhost", "Changeme123");
    token = auth.accessToken;
  });

  test.afterAll(async () => {
    // Clean up by listing sections and deleting by ID
    try {
      const res = await fetch(`${API_BASE}/prompt-sections?scope=global`, {
        headers: { Authorization: `Bearer ${token}` },
      });
      if (res.ok) {
        const sections = (await res.json()) as Array<{ id: string; name: string }>;
        for (const name of createdNames) {
          const section = sections.find((s) => s.name === name);
          if (section) {
            await fetch(`${API_BASE}/prompt-sections/${section.id}`, {
              method: "DELETE",
              headers: { Authorization: `Bearer ${token}` },
            });
          }
        }
      }
    } catch {
      // best-effort cleanup
    }
  });

  const headers = () => ({ Authorization: `Bearer ${token}` });
  const jsonHeaders = () => ({ ...headers(), "Content-Type": "application/json" });

  test("GET /prompt-sections returns 200 with array", async () => {
    const res = await fetch(`${API_BASE}/prompt-sections?scope=global`, {
      headers: headers(),
    });
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(Array.isArray(body)).toBe(true);
  });

  test("PUT /prompt-sections upserts a section", async () => {
    const name = `e2e-section-${Date.now()}`;
    const res = await fetch(`${API_BASE}/prompt-sections`, {
      method: "PUT",
      headers: jsonHeaders(),
      body: JSON.stringify({
        name,
        scope: "global",
        content: "This is a test prompt section.",
        priority: 10,
        merge: "replace",
      }),
    });
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body.name).toBe(name);
    createdNames.push(name);
  });

  test("DELETE /prompt-sections/{id} removes the section", async () => {
    // Create a section first
    const name = `e2e-del-${Date.now()}`;
    const createRes = await fetch(`${API_BASE}/prompt-sections`, {
      method: "PUT",
      headers: jsonHeaders(),
      body: JSON.stringify({
        name,
        scope: "global",
        content: "To be deleted.",
        priority: 5,
        merge: "replace",
      }),
    });
    expect(createRes.status).toBe(200);

    // List sections to find the ID (upsert does not return the generated ID)
    const listRes = await fetch(`${API_BASE}/prompt-sections?scope=global`, {
      headers: headers(),
    });
    expect(listRes.status).toBe(200);
    const sections = (await listRes.json()) as Array<{ id: string; name: string }>;
    const created = sections.find((s) => s.name === name);
    expect(created).toBeTruthy();
    expect(created!.id).toBeTruthy();

    // Delete it
    const delRes = await fetch(`${API_BASE}/prompt-sections/${created!.id}`, {
      method: "DELETE",
      headers: headers(),
    });
    expect(delRes.status).toBe(204);
  });

  test("POST /prompt-sections/preview returns preview text", async () => {
    const res = await fetch(`${API_BASE}/prompt-sections/preview`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({
        sections: [{ name: "intro", text: "Hello world", priority: 10, tokens: 0 }],
        budget: 1000,
      }),
    });
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body).toHaveProperty("text");
    expect(body).toHaveProperty("sections");
    expect(body).toHaveProperty("total_tokens");
  });

  test("DELETE /prompt-sections/{non-existent} returns 204 (idempotent)", async () => {
    const res = await fetch(`${API_BASE}/prompt-sections/00000000-0000-0000-0000-000000000000`, {
      method: "DELETE",
      headers: headers(),
    });
    // DELETE is idempotent: the store does not check row count, so
    // deleting a non-existent (but valid UUID) section returns 204.
    expect(res.status).toBe(204);
  });
});
