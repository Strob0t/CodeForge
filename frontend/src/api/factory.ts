/**
 * Tagged template literal for building URL paths with automatic encoding.
 *
 * Usage:
 * ```ts
 * const path = url`/projects/${id}/agents`;
 * // Equivalent to `/projects/${encodeURIComponent(id)}/agents`
 * ```
 */
export function url(strings: TemplateStringsArray, ...values: (string | number)[]): string {
  let result = strings[0];
  for (let i = 0; i < values.length; i++) {
    result += encodeURIComponent(String(values[i])) + strings[i + 1];
  }
  return result;
}

/** Configuration for a top-level CRUD client. */
interface CRUDConfig {
  /** Base path, e.g. "/modes" */
  basePath: string;
}

/** Configuration for a nested (parent-scoped) CRUD client. */
interface NestedCRUDConfig {
  /** Parent path, e.g. "/projects" */
  parentPath: string;
  /** Child resource name, e.g. "agents" */
  childResource: string;
  /** Optional standalone path prefix for get/update/delete (e.g. "/agents"). */
  standalonePath?: string;
}

interface CRUDClient<T, TCreate> {
  list: () => Promise<T[]>;
  get: (id: string) => Promise<T>;
  create: (data: TCreate) => Promise<T>;
  update: (id: string, data: TCreate) => Promise<T>;
  delete: (id: string) => Promise<undefined>;
}

interface NestedCRUDClient<T, TCreate> {
  list: (parentId: string) => Promise<T[]>;
  get: (id: string) => Promise<T>;
  create: (parentId: string, data: TCreate) => Promise<T>;
  update: (id: string, data: TCreate) => Promise<T>;
  delete: (id: string) => Promise<undefined>;
}

type RequestFn = <T>(path: string, init?: RequestInit) => Promise<T>;

/**
 * Create a standard CRUD client for a top-level resource.
 *
 * Usage:
 * ```ts
 * const modes = createCRUDClient<Mode, CreateModeRequest>(request, { basePath: "/modes" });
 * ```
 */
export function createCRUDClient<T, TCreate>(
  request: RequestFn,
  config: CRUDConfig,
): CRUDClient<T, TCreate> {
  const { basePath } = config;
  return {
    list: () => request<T[]>(basePath),
    get: (id: string) => request<T>(url`${basePath}/${id}`),
    create: (data: TCreate) =>
      request<T>(basePath, {
        method: "POST",
        body: JSON.stringify(data),
      }),
    update: (id: string, data: TCreate) =>
      request<T>(url`${basePath}/${id}`, {
        method: "PUT",
        body: JSON.stringify(data),
      }),
    delete: (id: string) => request<undefined>(url`${basePath}/${id}`, { method: "DELETE" }),
  };
}

/**
 * Create a CRUD client for a resource nested under a parent.
 *
 * Usage:
 * ```ts
 * const agents = createNestedCRUDClient<Agent, CreateAgentRequest>(request, {
 *   parentPath: "/projects",
 *   childResource: "agents",
 *   standalonePath: "/agents",
 * });
 * ```
 */
export function createNestedCRUDClient<T, TCreate>(
  request: RequestFn,
  config: NestedCRUDConfig,
): NestedCRUDClient<T, TCreate> {
  const { parentPath, childResource, standalonePath } = config;
  const itemPath = standalonePath ?? `${parentPath}/${childResource}`;
  return {
    list: (parentId: string) => request<T[]>(url`${parentPath}/${parentId}/${childResource}`),
    get: (id: string) => request<T>(url`${itemPath}/${id}`),
    create: (parentId: string, data: TCreate) =>
      request<T>(url`${parentPath}/${parentId}/${childResource}`, {
        method: "POST",
        body: JSON.stringify(data),
      }),
    update: (id: string, data: TCreate) =>
      request<T>(url`${itemPath}/${id}`, {
        method: "PUT",
        body: JSON.stringify(data),
      }),
    delete: (id: string) => request<undefined>(url`${itemPath}/${id}`, { method: "DELETE" }),
  };
}
