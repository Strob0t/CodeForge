# API Client Decomposition — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Decompose `api/client.ts` (1481 lines, 30+ resource groups) into domain-specific resource modules with a thin core client.

**Architecture:** Keep `client.ts` as the core (fetch wrapper, auth, retry, cache, FetchError) and extract each resource group into its own file under `api/resources/`. Each resource file exports a factory function that receives the `request` function. The `api` object in `client.ts` composes all resources. All existing tests and E2E specs must continue to pass.

**Tech Stack:** TypeScript, Vitest. Run tests: `cd frontend && npm test`

---

## File Structure

| File | Responsibility | Lines (est.) |
|------|---------------|-------------|
| `api/client.ts` | Core: fetch wrapper, auth, retry, cache, FetchError, api composition | ~200 |
| `api/core.ts` (NEW) | `request()`, `executeRequest()`, `FetchError`, auth getter — shared by all resources | ~120 |
| `api/resources/projects.ts` (NEW) | projects CRUD + adopt, discover, setup | ~80 |
| `api/resources/conversations.ts` (NEW) | conversations CRUD, messages, send, stop, fork, rewind | ~80 |
| `api/resources/agents.ts` (NEW) | agents CRUD, orchestration | ~50 |
| `api/resources/benchmarks.ts` (NEW) | benchmarks CRUD, runs, events, export | ~80 |
| `api/resources/llm.ts` (NEW) | models, providers, routing, costs | ~80 |
| `api/resources/auth.ts` (NEW) | login, setup, refresh, forgot/reset password | ~60 |
| `api/resources/files.ts` (NEW) | file read/write/list, git operations | ~60 |
| `api/resources/settings.ts` (NEW) | settings, policies, modes, users, API keys | ~100 |
| `api/resources/roadmap.ts` (NEW) | roadmap, milestones, features, tasks, PM sync | ~100 |
| `api/resources/misc.ts` (NEW) | health, search, channels, notifications, scopes, LSP, MCP, A2A, etc. | ~150 |
| `api/resources/index.ts` (NEW) | Re-exports all resource factories | ~20 |

---

## Task 1: Extract Core Request Infrastructure

**Files:**
- Create: `frontend/src/api/core.ts`
- Modify: `frontend/src/api/client.ts`

- [ ] **Step 1: Read client.ts to identify the core**

Lines 1-230 approximately: imports, constants, `FetchError`, `executeRequest`, `request`, `get`/`post`/`put`/`del` helpers, auth getter/setter, cache integration.

- [ ] **Step 2: Create core.ts**

Extract:
- `FetchError` class
- `executeRequest()` function
- `request()` function with retry logic
- `get()`, `post()`, `put()`, `del()` helper functions
- `setAccessTokenGetter()`, `getAccessToken()`
- Constants: `BASE`, `MAX_RETRIES`, `RETRY_BASE_MS`, `RETRYABLE_STATUSES`

Export a `RequestFn` type:
```typescript
export type RequestFn = <T>(path: string, init?: RequestInit) => Promise<T>;
export type GetFn = <T>(path: string) => Promise<T>;
export type PostFn = <T>(path: string, body?: unknown) => Promise<T>;
export type PutFn = <T>(path: string, body?: unknown) => Promise<T>;
export type DelFn = <T>(path: string) => Promise<T>;

export interface CoreClient {
  request: RequestFn;
  get: GetFn;
  post: PostFn;
  put: PutFn;
  del: DelFn;
}
```

- [ ] **Step 3: Update client.ts to import from core**

```typescript
import { createCoreClient, FetchError, setAccessTokenGetter, getAccessToken } from "./core";
export { FetchError, setAccessTokenGetter, getAccessToken };

const core = createCoreClient();
```

- [ ] **Step 4: Run tests**

```bash
cd frontend && npm test
```

- [ ] **Step 5: Commit**

```bash
git add frontend/src/api/core.ts frontend/src/api/client.ts
git commit -m "refactor: extract API core (fetch, retry, auth, FetchError) into core.ts"
```

---

## Task 2: Extract Project + Conversation Resources

**Files:**
- Create: `frontend/src/api/resources/projects.ts`
- Create: `frontend/src/api/resources/conversations.ts`
- Modify: `frontend/src/api/client.ts`

- [ ] **Step 1: Create resources directory**

```bash
mkdir -p frontend/src/api/resources
```

- [ ] **Step 2: Create projects.ts**

```typescript
import type { CoreClient } from "../core";
import type { Project, CreateProjectRequest, /* ... */ } from "../types";

export function createProjectsResource(c: CoreClient) {
  return {
    list: () => c.get<Project[]>("/projects"),
    get: (id: string) => c.get<Project>(`/projects/${id}`),
    create: (data: CreateProjectRequest) => c.post<Project>("/projects", data),
    update: (id: string, data: Partial<Project>) => c.put<Project>(`/projects/${id}`, data),
    delete: (id: string) => c.del<void>(`/projects/${id}`),
    adopt: (id: string, data: { path: string }) => c.post<void>(`/projects/${id}/adopt`, data),
    // ... remaining project methods from client.ts
  };
}
```

- [ ] **Step 3: Create conversations.ts**

Same pattern — extract all `conversations.*` methods.

- [ ] **Step 4: Update client.ts**

Replace inline `projects: { ... }` and `conversations: { ... }` with:
```typescript
import { createProjectsResource } from "./resources/projects";
import { createConversationsResource } from "./resources/conversations";

projects: createProjectsResource(core),
conversations: createConversationsResource(core),
```

- [ ] **Step 5: Run tests**

```bash
cd frontend && npm test
```

- [ ] **Step 6: Commit**

```bash
git add frontend/src/api/resources/ frontend/src/api/client.ts
git commit -m "refactor: extract projects + conversations API resources"
```

---

## Task 3: Extract Auth + Files + Agents Resources

**Files:**
- Create: `frontend/src/api/resources/auth.ts`
- Create: `frontend/src/api/resources/files.ts`
- Create: `frontend/src/api/resources/agents.ts`
- Modify: `frontend/src/api/client.ts`

- [ ] **Step 1: Extract auth resource**

Login, setup, refresh, forgot/reset password, change password, VCS accounts, device flow.

- [ ] **Step 2: Extract files resource**

File read/write/list, git status, branches, commits.

- [ ] **Step 3: Extract agents resource**

Agents CRUD, orchestration endpoints.

- [ ] **Step 4: Update client.ts + run tests**

- [ ] **Step 5: Commit**

```bash
git add frontend/src/api/resources/ frontend/src/api/client.ts
git commit -m "refactor: extract auth, files, agents API resources"
```

---

## Task 4: Extract LLM + Benchmarks + Roadmap Resources

**Files:**
- Create: `frontend/src/api/resources/llm.ts`
- Create: `frontend/src/api/resources/benchmarks.ts`
- Create: `frontend/src/api/resources/roadmap.ts`
- Modify: `frontend/src/api/client.ts`

- [ ] **Step 1: Extract llm resource**

Models, providers, routing config, costs, usage.

- [ ] **Step 2: Extract benchmarks resource**

Benchmark suites, runs, events, evaluators, export.

- [ ] **Step 3: Extract roadmap resource**

Roadmap, milestones, features, tasks, PM sync, detect, import.

- [ ] **Step 4: Update client.ts + run tests**

- [ ] **Step 5: Commit**

```bash
git add frontend/src/api/resources/ frontend/src/api/client.ts
git commit -m "refactor: extract llm, benchmarks, roadmap API resources"
```

---

## Task 5: Extract Settings + Misc Resources

**Files:**
- Create: `frontend/src/api/resources/settings.ts`
- Create: `frontend/src/api/resources/misc.ts`
- Create: `frontend/src/api/resources/index.ts`
- Modify: `frontend/src/api/client.ts`

- [ ] **Step 1: Extract settings resource**

Settings CRUD, policies, modes, users, API keys.

- [ ] **Step 2: Extract misc resource**

Health, search, channels, notifications, scopes, LSP, MCP, A2A, quarantine, active work, trajectory, agentConfig, etc.

- [ ] **Step 3: Create index.ts**

```typescript
export { createProjectsResource } from "./projects";
export { createConversationsResource } from "./conversations";
export { createAuthResource } from "./auth";
// ... all others
```

- [ ] **Step 4: Update client.ts to compose from resources**

The `api` object should now only be:
```typescript
export const api = {
  projects: createProjectsResource(core),
  conversations: createConversationsResource(core),
  auth: createAuthResource(core),
  files: createFilesResource(core),
  // ... etc
};
```

- [ ] **Step 5: Run tests**

```bash
cd frontend && npm test
```

- [ ] **Step 6: Commit**

```bash
git add frontend/src/api/resources/ frontend/src/api/client.ts
git commit -m "refactor: extract settings, misc API resources + resource index"
```

---

## Task 6: Update Tests + Verify

**Files:**
- Modify: `frontend/src/api/client.test.ts`

- [ ] **Step 1: Update client.test.ts**

Existing tests import from `./client` — they should still work since `api` is still exported from `client.ts`. Add tests for `core.ts`:

```typescript
describe("API Core", () => {
  it("should export FetchError", async () => {
    const mod = await import("./core");
    expect(mod.FetchError).toBeDefined();
  });

  it("should export createCoreClient", async () => {
    const mod = await import("./core");
    expect(typeof mod.createCoreClient).toBe("function");
  });
});

describe("API Resources", () => {
  it("should export all resource factories", async () => {
    const mod = await import("./resources");
    expect(mod.createProjectsResource).toBeDefined();
    expect(mod.createConversationsResource).toBeDefined();
    // ... etc
  });
});
```

- [ ] **Step 2: Run full test suite + type check**

```bash
cd frontend && npm test && npx tsc --noEmit
```

- [ ] **Step 3: Verify client.ts is under 250 lines**

```bash
wc -l frontend/src/api/client.ts
```
Expected: ~200 lines (down from 1481).

- [ ] **Step 4: Final commit**

```bash
git add frontend/src/api/
git commit -m "refactor: API client decomposition complete — 1481 LOC → 12 focused modules"
```
