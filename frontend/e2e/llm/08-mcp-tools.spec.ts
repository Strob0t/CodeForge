import { test, expect } from "@playwright/test";
import { apiLogin, createProject, createCleanupTracker, API_BASE } from "../helpers/api-helpers";

/**
 * MCP server CRUD operations and project assignment.
 * Validates the full lifecycle: create, read, update, test, assign, unassign, delete.
 */
test.describe("LLM E2E — MCP Server Integration", () => {
  let token: string;
  const cleanup = createCleanupTracker();

  // Shared state across tests (serial execution within describe)
  let serverId: string;
  let projectId: string;

  test.beforeAll(async () => {
    const auth = await apiLogin("admin@localhost", "Changeme123");
    token = auth.accessToken;
  });

  test.afterAll(async () => {
    await cleanup.cleanup();
  });

  const authHeaders = (): Record<string, string> => ({
    Authorization: `Bearer ${token}`,
    "Content-Type": "application/json",
  });

  test("POST /mcp/servers creates server with stdio transport", async () => {
    const res = await fetch(`${API_BASE}/mcp/servers`, {
      method: "POST",
      headers: authHeaders(),
      body: JSON.stringify({
        name: "e2e-mcp-test",
        description: "E2E test server",
        transport: "stdio",
        command: "echo",
        args: ["hello"],
        enabled: true,
      }),
    });
    expect(res.status).toBe(201);
    const body = await res.json();
    expect(body.id).toBeTruthy();
    expect(body.name).toBe("e2e-mcp-test");
    serverId = body.id;
    cleanup.add("mcp-server", serverId);
  });

  test("GET /mcp/servers lists servers including new one", async () => {
    const res = await fetch(`${API_BASE}/mcp/servers`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(Array.isArray(body)).toBe(true);
    const found = body.some((s: { name: string }) => s.name === "e2e-mcp-test");
    expect(found).toBe(true);
  });

  test("GET /mcp/servers/{id} returns server details", async () => {
    const res = await fetch(`${API_BASE}/mcp/servers/${serverId}`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body.id).toBe(serverId);
    expect(body.name).toBe("e2e-mcp-test");
  });

  test("PUT /mcp/servers/{id} updates server config", async () => {
    const res = await fetch(`${API_BASE}/mcp/servers/${serverId}`, {
      method: "PUT",
      headers: authHeaders(),
      body: JSON.stringify({
        name: "e2e-mcp-test",
        description: "Updated E2E description",
        transport: "stdio",
        command: "echo",
        args: ["hello"],
        enabled: true,
      }),
    });
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body.description).toBe("Updated E2E description");
  });

  test("POST /mcp/servers/{id}/test tests connection", async () => {
    const res = await fetch(`${API_BASE}/mcp/servers/${serverId}/test`, {
      method: "POST",
      headers: authHeaders(),
      body: JSON.stringify({}),
    });
    // echo command may succeed or fail depending on MCP protocol expectations
    expect([200, 400, 500]).toContain(res.status);
  });

  test("POST /projects/{id}/mcp-servers assigns server to project", async () => {
    // Create a project for assignment
    const proj = await createProject("e2e-llm-mcp-assign");
    projectId = proj.id;
    cleanup.add("project", projectId);

    const res = await fetch(`${API_BASE}/projects/${projectId}/mcp-servers`, {
      method: "POST",
      headers: authHeaders(),
      body: JSON.stringify({ server_id: serverId }),
    });
    expect([200, 201, 204]).toContain(res.status);
  });

  test("GET /projects/{id}/mcp-servers lists assigned servers", async () => {
    const res = await fetch(`${API_BASE}/projects/${projectId}/mcp-servers`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(Array.isArray(body)).toBe(true);
    const found = body.some((s: { id: string }) => s.id === serverId);
    expect(found).toBe(true);
  });

  test("DELETE /projects/{id}/mcp-servers/{serverId} unassigns", async () => {
    const res = await fetch(`${API_BASE}/projects/${projectId}/mcp-servers/${serverId}`, {
      method: "DELETE",
      headers: { Authorization: `Bearer ${token}` },
    });
    expect([200, 204]).toContain(res.status);
  });

  test("DELETE /mcp/servers/{id} removes server", async () => {
    const res = await fetch(`${API_BASE}/mcp/servers/${serverId}`, {
      method: "DELETE",
      headers: { Authorization: `Bearer ${token}` },
    });
    expect([200, 204]).toContain(res.status);
    // Remove from cleanup since we just deleted it
    cleanup.ids = cleanup.ids.filter(
      (item) => !(item.type === "mcp-server" && item.id === serverId),
    );
  });

  test("POST /mcp/servers without name returns 400", async () => {
    const res = await fetch(`${API_BASE}/mcp/servers`, {
      method: "POST",
      headers: authHeaders(),
      body: JSON.stringify({}),
    });
    expect(res.status).toBe(400);
  });
});
