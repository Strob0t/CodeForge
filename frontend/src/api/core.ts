import { MAX_RETRIES, RETRY_BASE_MS, RETRYABLE_STATUSES } from "~/config/constants";
import { logError } from "~/lib/errorUtils";

import { getCached, invalidateCache, processQueue, queueAction, setCached } from "./cache";
import type { ApiError } from "./types";

const BASE = "/api/v1";

// ---------------------------------------------------------------------------
// Access token management
// ---------------------------------------------------------------------------

let accessTokenGetter: (() => string | null) | null = null;

/** Set the function that provides the current access token. */
export function setAccessTokenGetter(fn: () => string | null): void {
  accessTokenGetter = fn;
}

/** Return the current access token (used by WebSocket to append ?token=). */
export function getAccessToken(): string | null {
  return accessTokenGetter?.() ?? null;
}

// ---------------------------------------------------------------------------
// FetchError
// ---------------------------------------------------------------------------

export class FetchError extends Error {
  constructor(
    public readonly status: number,
    public readonly body: ApiError,
  ) {
    super(body.error);
    this.name = "FetchError";
  }
}

// ---------------------------------------------------------------------------
// Core request infrastructure
// ---------------------------------------------------------------------------

function isRetryable(status: number, method: string): boolean {
  if (method === "POST") return false;
  return RETRYABLE_STATUSES.has(status);
}

function isOffline(): boolean {
  return !navigator.onLine;
}

async function executeRequest<T>(path: string, init?: RequestInit): Promise<T> {
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    ...(init?.headers as Record<string, string>),
  };

  const token = accessTokenGetter?.();
  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }

  const res = await fetch(`${BASE}${path}`, {
    ...init,
    headers,
    credentials: "include",
  });

  if (!res.ok) {
    let body: ApiError;
    try {
      body = (await res.json()) as ApiError;
    } catch (err) {
      logError("api.parseErrorBody", err);
      body = { error: `HTTP ${res.status} ${res.statusText || "Error"}` };
    }
    throw new FetchError(res.status, body);
  }

  if (res.status === 204) {
    return undefined as T;
  }

  return res.json() as Promise<T>;
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const method = init?.method ?? "GET";
  let lastError: unknown;

  for (let attempt = 0; attempt <= MAX_RETRIES; attempt++) {
    try {
      const result = await executeRequest<T>(path, init);

      if (method === "GET") {
        setCached(path, result);
      }

      return result;
    } catch (err) {
      if (err instanceof FetchError) {
        if (attempt < MAX_RETRIES && isRetryable(err.status, method)) {
          lastError = err;
          await new Promise((r) => setTimeout(r, RETRY_BASE_MS * 2 ** attempt));
          continue;
        }
        throw err;
      }

      if (err instanceof TypeError) {
        if (attempt < MAX_RETRIES) {
          lastError = err;
          await new Promise((r) => setTimeout(r, RETRY_BASE_MS * 2 ** attempt));
          continue;
        }

        if (method === "GET") {
          const cached = getCached<T>(path);
          if (cached !== undefined) return cached;
        }

        if (method !== "GET" && isOffline() && init) {
          return queueAction(path, init) as Promise<T>;
        }

        throw err;
      }

      throw err;
    }
  }

  throw lastError;
}

// Process queued actions when coming back online
if (typeof window !== "undefined") {
  window.addEventListener("online", () => {
    void processQueue((path, init) => executeRequest(path, init));
  });
}

// ---------------------------------------------------------------------------
// Core client type + factory
// ---------------------------------------------------------------------------

export type RequestFn = <T>(path: string, init?: RequestInit) => Promise<T>;
export type GetFn = <T>(path: string) => Promise<T>;
export type PostFn = <T>(path: string, body?: unknown) => Promise<T>;
export type PutFn = <T>(path: string, body?: unknown) => Promise<T>;
export type DelFn = <T>(path: string) => Promise<T>;

export type PatchFn = <T>(path: string, body?: unknown) => Promise<T>;

export interface CoreClient {
  request: RequestFn;
  get: GetFn;
  post: PostFn;
  put: PutFn;
  patch: PatchFn;
  del: DelFn;
  /** The API base path (e.g. "/api/v1") for building raw URLs. */
  BASE: string;
  /** Cache invalidation helper. */
  invalidateCache: (pathPrefix: string) => void;
}

function get<T>(path: string): Promise<T> {
  return request<T>(path);
}

function post<T>(path: string, body?: unknown): Promise<T> {
  return request<T>(path, {
    method: "POST",
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
}

function put<T>(path: string, body?: unknown): Promise<T> {
  return request<T>(path, {
    method: "PUT",
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
}

function patch<T>(path: string, body?: unknown): Promise<T> {
  return request<T>(path, {
    method: "PATCH",
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
}

function del<T>(path: string): Promise<T> {
  return request<T>(path, { method: "DELETE" });
}

export function createCoreClient(): CoreClient {
  return { request, get, post, put, patch, del, BASE, invalidateCache };
}
