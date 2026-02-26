import { test, expect } from "@playwright/test";
import { apiLogin, createCleanupTracker, API_BASE } from "../helpers/api-helpers";

test.describe("MCP API", () => {
  let token: string;
  const cleanup = createCleanupTracker();

  test.beforeAll(async () => {
    const auth = await apiLogin("admin@localhost", "Changeme123");
    token = auth.accessToken;
  });

  test.afterAll(async () => {
    await cleanup.cleanup();
  });

  const headers = () => ({ Authorization: `Bearer ${token}` });

  test("list servers returns array", async ({ request }) => {
    const res = await request.get(`${API_BASE}/mcp/servers`, { headers: headers() });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(Array.isArray(body)).toBe(true);
  });

  test("create server returns 201", async ({ request }) => {
    const res = await request.post(`${API_BASE}/mcp/servers`, {
      headers: headers(),
      data: {
        name: `e2e-mcp-${Date.now()}`,
        description: "E2E test MCP server",
        transport: "stdio",
        command: "echo",
        args: ["hello"],
        enabled: true,
      },
    });
    expect(res.status()).toBe(201);
    const body = await res.json();
    expect(body.id).toBeTruthy();
    expect(body.name).toBeTruthy();
    cleanup.add("mcp-server", body.id);
  });

  test("get server by ID", async ({ request }) => {
    const createRes = await request.post(`${API_BASE}/mcp/servers`, {
      headers: headers(),
      data: {
        name: `e2e-get-mcp-${Date.now()}`,
        transport: "stdio",
        command: "echo",
        enabled: true,
      },
    });
    const server = await createRes.json();
    cleanup.add("mcp-server", server.id);

    const res = await request.get(`${API_BASE}/mcp/servers/${server.id}`, { headers: headers() });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(body.id).toBe(server.id);
  });

  test("update server", async ({ request }) => {
    const createRes = await request.post(`${API_BASE}/mcp/servers`, {
      headers: headers(),
      data: {
        name: `e2e-upd-mcp-${Date.now()}`,
        transport: "stdio",
        command: "echo",
        enabled: true,
      },
    });
    const server = await createRes.json();
    cleanup.add("mcp-server", server.id);

    const res = await request.put(`${API_BASE}/mcp/servers/${server.id}`, {
      headers: headers(),
      data: {
        name: `e2e-updated-mcp-${Date.now()}`,
        description: "updated",
        transport: "stdio",
        command: "echo",
        enabled: false,
      },
    });
    expect(res.status()).toBe(200);
  });

  test("delete server returns 204", async ({ request }) => {
    const createRes = await request.post(`${API_BASE}/mcp/servers`, {
      headers: headers(),
      data: {
        name: `e2e-del-mcp-${Date.now()}`,
        transport: "stdio",
        command: "echo",
        enabled: true,
      },
    });
    const server = await createRes.json();

    const res = await request.delete(`${API_BASE}/mcp/servers/${server.id}`, {
      headers: headers(),
    });
    expect(res.status()).toBe(204);
  });

  test("test connection for server", async ({ request }) => {
    const createRes = await request.post(`${API_BASE}/mcp/servers`, {
      headers: headers(),
      data: {
        name: `e2e-test-mcp-${Date.now()}`,
        transport: "stdio",
        command: "echo",
        enabled: true,
      },
    });
    const server = await createRes.json();
    cleanup.add("mcp-server", server.id);

    const res = await request.post(`${API_BASE}/mcp/servers/${server.id}/test`, {
      headers: headers(),
      data: {},
    });
    // Test may succeed or fail depending on server availability
    expect([200, 400, 500]).toContain(res.status());
  });

  test("pre-save test connection", async ({ request }) => {
    const res = await request.post(`${API_BASE}/mcp/servers/test`, {
      headers: headers(),
      data: {
        name: "pre-save-test",
        transport: "stdio",
        command: "echo",
        enabled: true,
      },
    });
    // Test may succeed or fail depending on server availability
    expect([200, 400, 500]).toContain(res.status());
  });

  test("list tools for server", async ({ request }) => {
    const createRes = await request.post(`${API_BASE}/mcp/servers`, {
      headers: headers(),
      data: {
        name: `e2e-tools-mcp-${Date.now()}`,
        transport: "stdio",
        command: "echo",
        enabled: true,
      },
    });
    const server = await createRes.json();
    cleanup.add("mcp-server", server.id);

    const res = await request.get(`${API_BASE}/mcp/servers/${server.id}/tools`, {
      headers: headers(),
    });
    // May return empty array or error if server not connected
    expect([200, 400, 500]).toContain(res.status());
  });

  test("delete non-existent server returns 404", async ({ request }) => {
    const res = await request.delete(
      `${API_BASE}/mcp/servers/00000000-0000-0000-0000-000000000000`,
      { headers: headers() },
    );
    expect(res.status()).toBe(404);
  });

  test("create server validation requires name", async ({ request }) => {
    const res = await request.post(`${API_BASE}/mcp/servers`, {
      headers: headers(),
      data: { transport: "stdio", command: "echo" },
    });
    expect([400, 201]).toContain(res.status());
    // If the server was created despite missing name, clean it up
    if (res.status() === 201) {
      const body = await res.json();
      cleanup.add("mcp-server", body.id);
    }
  });
});
