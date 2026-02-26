/**
 * Direct HTTP helpers for API-level E2E tests.
 * Wraps fetch with auth, JSON handling, and typed responses.
 */

const API_BASE = "http://localhost:8080/api/v1";
const ADMIN_EMAIL = "admin@localhost";
const ADMIN_PASS = "Changeme123";

export interface AuthTokens {
  accessToken: string;
  user: { id: string; email: string; role: string };
}

let cachedAdminTokens: AuthTokens | null = null;

/** Login via API and return tokens + user info.
 *  If the account requires a password change (seeded admin), performs the change automatically.
 */
export async function apiLogin(email: string, password: string): Promise<AuthTokens> {
  const res = await fetch(`${API_BASE}/auth/login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email, password }),
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(`Login failed (${res.status}): ${text}`);
  }
  const body = (await res.json()) as {
    access_token: string;
    user: { id: string; email: string; role: string; must_change_password?: boolean };
  };
  const token = body.access_token;

  // Handle forced password change for seeded admin
  if (body.user.must_change_password) {
    const cpRes = await fetch(`${API_BASE}/auth/change-password`, {
      method: "POST",
      headers: { "Content-Type": "application/json", Authorization: `Bearer ${token}` },
      body: JSON.stringify({ old_password: password, new_password: password }),
    });
    if (!cpRes.ok) {
      const cpText = await cpRes.text();
      throw new Error(`Password change failed (${cpRes.status}): ${cpText}`);
    }
    // Re-login to get a fresh token without must_change_password
    const reRes = await fetch(`${API_BASE}/auth/login`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email, password }),
    });
    if (!reRes.ok) throw new Error(`Re-login failed (${reRes.status}): ${await reRes.text()}`);
    const reBody = (await reRes.json()) as {
      access_token: string;
      user: { id: string; email: string; role: string };
    };
    return { accessToken: reBody.access_token, user: reBody.user };
  }

  return { accessToken: token, user: body.user };
}

/** Get admin tokens (cached per process). */
export async function getAdminTokens(): Promise<AuthTokens> {
  if (!cachedAdminTokens) {
    cachedAdminTokens = await apiLogin(ADMIN_EMAIL, ADMIN_PASS);
  }
  return cachedAdminTokens;
}

/** Generic authenticated fetch helper. */
export async function apiFetch(
  path: string,
  options: RequestInit = {},
  token?: string,
): Promise<Response> {
  const auth = token ?? (await getAdminTokens()).accessToken;
  const headers: Record<string, string> = {
    ...(options.headers as Record<string, string> | undefined),
    Authorization: `Bearer ${auth}`,
  };
  if (options.body && typeof options.body === "string") {
    headers["Content-Type"] = "application/json";
  }
  return fetch(`${API_BASE}${path}`, { ...options, headers });
}

/** GET with JSON parsing. */
export async function apiGet<T = unknown>(path: string, token?: string): Promise<T> {
  const res = await apiFetch(path, { method: "GET" }, token);
  if (!res.ok) throw new Error(`GET ${path} failed (${res.status}): ${await res.text()}`);
  return (await res.json()) as T;
}

/** POST with JSON body. */
export async function apiPost<T = unknown>(
  path: string,
  data: unknown,
  token?: string,
): Promise<T> {
  const res = await apiFetch(path, { method: "POST", body: JSON.stringify(data) }, token);
  if (!res.ok) throw new Error(`POST ${path} failed (${res.status}): ${await res.text()}`);
  const text = await res.text();
  return text ? (JSON.parse(text) as T) : ({} as T);
}

/** PUT with JSON body. */
export async function apiPut<T = unknown>(path: string, data: unknown, token?: string): Promise<T> {
  const res = await apiFetch(path, { method: "PUT", body: JSON.stringify(data) }, token);
  if (!res.ok) throw new Error(`PUT ${path} failed (${res.status}): ${await res.text()}`);
  const text = await res.text();
  return text ? (JSON.parse(text) as T) : ({} as T);
}

/** DELETE — returns status code. */
export async function apiDelete(path: string, token?: string): Promise<number> {
  const res = await apiFetch(path, { method: "DELETE" }, token);
  return res.status;
}

/** Raw fetch returning Response (for status code assertions). */
export async function apiRaw(
  path: string,
  options: RequestInit = {},
  token?: string,
): Promise<Response> {
  return apiFetch(path, options, token);
}

// --- Entity helpers with cleanup tracking ---

export interface CleanupTracker {
  ids: Array<{ type: string; id: string; projectId?: string }>;
  add(type: string, id: string, projectId?: string): void;
  cleanup(): Promise<void>;
}

export function createCleanupTracker(): CleanupTracker {
  const tracker: CleanupTracker = {
    ids: [],
    add(type: string, id: string, projectId?: string) {
      tracker.ids.push({ type, id, projectId });
    },
    async cleanup() {
      // Reverse order to handle dependencies
      for (const item of [...tracker.ids].reverse()) {
        try {
          switch (item.type) {
            case "project":
              await apiDelete(`/projects/${item.id}`);
              break;
            case "agent":
              await apiDelete(`/agents/${item.id}`);
              break;
            case "scope":
              await apiDelete(`/scopes/${item.id}`);
              break;
            case "knowledge-base":
              await apiDelete(`/knowledge-bases/${item.id}`);
              break;
            case "mcp-server":
              await apiDelete(`/mcp/servers/${item.id}`);
              break;
            case "policy":
              await apiDelete(`/policies/${item.id}`);
              break;
            case "user":
              await apiDelete(`/users/${item.id}`);
              break;
            case "team":
              await apiDelete(`/teams/${item.id}`);
              break;
            case "conversation":
              await apiDelete(`/conversations/${item.id}`);
              break;
            case "prompt-section":
              await apiDelete(`/prompt-sections/${item.id}`);
              break;
          }
        } catch {
          // Best-effort cleanup
        }
      }
      tracker.ids = [];
    },
  };
  return tracker;
}

// --- Convenience create helpers ---

export interface ProjectData {
  id: string;
  name: string;
}

export async function createProject(name: string, description = ""): Promise<ProjectData> {
  return apiPost<ProjectData>("/projects", {
    name,
    description,
    repo_url: "",
    provider: "",
    config: {},
  });
}

export async function createAgent(
  projectId: string,
  name: string,
  backend = "aider",
): Promise<{ id: string; name: string }> {
  return apiPost<{ id: string; name: string }>(`/projects/${projectId}/agents`, { name, backend });
}

export async function createTask(
  projectId: string,
  title: string,
  prompt = "test prompt",
): Promise<{ id: string }> {
  return apiPost<{ id: string }>(`/projects/${projectId}/tasks`, { title, prompt });
}

export async function createUser(
  email: string,
  password: string,
  role = "viewer",
): Promise<{ id: string; email: string }> {
  return apiPost<{ id: string; email: string }>("/users", {
    email,
    name: email.split("@")[0],
    password,
    role,
  });
}

export { API_BASE, ADMIN_EMAIL, ADMIN_PASS };
