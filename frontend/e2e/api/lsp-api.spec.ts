import { test, expect } from "@playwright/test";
import { apiLogin, createProject, createCleanupTracker, API_BASE } from "../helpers/api-helpers";

test.describe("LSP API", () => {
  let token: string;
  let projectId: string;
  const cleanup = createCleanupTracker();

  test.beforeAll(async () => {
    const auth = await apiLogin("admin@localhost", "Changeme123");
    token = auth.accessToken;
    const proj = await createProject(`lsp-e2e-${Date.now()}`);
    projectId = proj.id;
    cleanup.add("project", projectId);
  });

  test.afterAll(async () => {
    // Stop LSP if running
    try {
      await fetch(`${API_BASE}/projects/${projectId}/lsp/stop`, {
        method: "POST",
        headers: {
          Authorization: `Bearer ${token}`,
          "Content-Type": "application/json",
        },
        body: JSON.stringify({}),
      });
    } catch {
      // best-effort
    }
    await cleanup.cleanup();
  });

  const headers = () => ({ Authorization: `Bearer ${token}` });
  const jsonHeaders = () => ({
    ...headers(),
    "Content-Type": "application/json",
  });

  test("POST /projects/{id}/lsp/start starts LSP servers", async () => {
    const res = await fetch(`${API_BASE}/projects/${projectId}/lsp/start`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({ languages: ["go"] }),
    });
    // 200 if started, 400 if no workspace, 503 if LSP not enabled
    expect([200, 400, 503]).toContain(res.status);
  });

  test("POST /projects/{id}/lsp/stop stops LSP servers", async () => {
    const res = await fetch(`${API_BASE}/projects/${projectId}/lsp/stop`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({}),
    });
    // 200 if stopped, 500 if nothing to stop, 503 if LSP not enabled
    expect([200, 500, 503]).toContain(res.status);
  });

  test("GET /projects/{id}/lsp/status returns server status", async () => {
    const res = await fetch(`${API_BASE}/projects/${projectId}/lsp/status`, {
      headers: headers(),
    });
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(Array.isArray(body)).toBe(true);
  });

  test("GET /projects/{id}/lsp/diagnostics returns diagnostics", async () => {
    const res = await fetch(`${API_BASE}/projects/${projectId}/lsp/diagnostics`, {
      headers: headers(),
    });
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(Array.isArray(body)).toBe(true);
  });

  test("POST /projects/{id}/lsp/definition looks up definition", async () => {
    const res = await fetch(`${API_BASE}/projects/${projectId}/lsp/definition`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({
        uri: "file:///main.go",
        line: 0,
        character: 0,
      }),
    });
    // 200 if LSP running, 503 if not enabled, 404 if not found
    expect([200, 404, 503]).toContain(res.status);
  });

  test("POST /projects/{id}/lsp/references looks up references", async () => {
    const res = await fetch(`${API_BASE}/projects/${projectId}/lsp/references`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({
        uri: "file:///main.go",
        line: 0,
        character: 0,
      }),
    });
    expect([200, 404, 503]).toContain(res.status);
  });

  test("POST /projects/{id}/lsp/symbols returns document symbols", async () => {
    const res = await fetch(`${API_BASE}/projects/${projectId}/lsp/symbols`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({ uri: "file:///main.go" }),
    });
    expect([200, 404, 503]).toContain(res.status);
  });

  test("POST /projects/{id}/lsp/hover returns hover info", async () => {
    const res = await fetch(`${API_BASE}/projects/${projectId}/lsp/hover`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({
        uri: "file:///main.go",
        line: 0,
        character: 0,
      }),
    });
    expect([200, 404, 503]).toContain(res.status);
  });

  test("POST /lsp/stop on non-running LSP returns appropriate status", async () => {
    // Create a fresh project with no LSP
    const proj2 = await createProject(`lsp-stop-e2e-${Date.now()}`);
    cleanup.add("project", proj2.id);

    const res = await fetch(`${API_BASE}/projects/${proj2.id}/lsp/stop`, {
      method: "POST",
      headers: jsonHeaders(),
      body: JSON.stringify({}),
    });
    // Should handle gracefully
    expect([200, 500, 503]).toContain(res.status);
  });

  test("GET /lsp/status when not running returns empty array", async () => {
    const proj3 = await createProject(`lsp-status-e2e-${Date.now()}`);
    cleanup.add("project", proj3.id);

    const res = await fetch(`${API_BASE}/projects/${proj3.id}/lsp/status`, {
      headers: headers(),
    });
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(Array.isArray(body)).toBe(true);
    expect(body.length).toBe(0);
  });
});
