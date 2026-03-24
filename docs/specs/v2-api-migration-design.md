# v2 API Migration Design

## Status

Draft -- 2026-03-24

## Overview

Design for migrating the CodeForge REST API from v1 (`/api/v1/`) to v2 (`/api/v2/`), addressing five documented issues from the code audit:

| Issue | Summary |
|-------|---------|
| **FIX-061** | Verb-based URLs (`/detect-stack`, `/parse-repo-url`, `/discover`, `/decompose`) must become noun-based resources |
| **FIX-063** | List endpoints return inconsistent response shapes; standardize on a pagination envelope |
| **FIX-095** | CSRF strategy undocumented; formalize the SameSite=Lax decision for JSON-only API |
| **FIX-098** | POST used for DELETE operations (`/llm/models/delete`, `/projects/batch/delete`) must use HTTP DELETE |
| **FIX-100** | PUT used for partial updates must use PATCH |

**Scope:** This document covers API surface changes only. Backend handler implementations, database queries, and service-layer code remain unchanged unless a handler must be split for a new URL path or HTTP method.

---

## Versioning Strategy

### Approach: URL Path Versioning

All v2 routes live under `/api/v2/`. This is the most widely adopted REST versioning strategy and has the following advantages:

- **Cache-friendly** -- CDN, reverse proxy, and browser caches key on URL path; header-based versioning breaks this.
- **Explicit** -- every request declares its API version; no hidden `Accept` header negotiation.
- **Router-native** -- chi `r.Route("/api/v2", ...)` gives clean separation with zero middleware overhead.
- **Discoverable** -- `GET /api/v2/` returns version metadata, same as v1.

### Parallel Operation

v1 and v2 run simultaneously. Both route groups are mounted on the same chi router:

```go
r.Route("/api/v1", func(r chi.Router) {
    r.Use(middleware.Deprecation(sunsetDate))
    // ... all existing v1 routes, unchanged ...
})

r.Route("/api/v2", func(r chi.Router) {
    // ... v2 routes with all FIX changes applied ...
})
```

All v1 handlers are reused in v2 where the endpoint is unchanged. Only endpoints affected by FIX-061/063/098/100 get new or modified handlers.

### Deprecation Timeline

| Milestone | Date | Action |
|-----------|------|--------|
| v2 GA release | T+0 | v2 routes available; v1 gets `Deprecation` headers |
| Migration window | T+0 to T+12mo | Both versions served; v1 emits deprecation headers on every response |
| v1 sunset | T+12mo | v1 routes return `410 Gone` with `Link` to migration guide |
| v1 removal | T+15mo | v1 route group deleted from source |

---

## Deprecation Middleware

When v2 is released, the v1 route group receives deprecation middleware that sets three headers on every response, per RFC 9745 (Deprecation header) and RFC 8594 (Sunset header):

| Header | Value | RFC |
|--------|-------|-----|
| `Deprecation` | `true` | RFC 9745 |
| `Sunset` | RFC 7231 HTTP-date (12 months from v2 GA) | RFC 8594 |
| `Link` | `</docs/v2-migration>; rel="sunset"` | RFC 8288 |

### Go Implementation

```go
package middleware

import (
    "net/http"
    "time"
)

// Deprecation returns middleware that sets RFC 9745 Deprecation,
// RFC 8594 Sunset, and RFC 8288 Link headers on every response.
// sunsetDate is the date after which v1 will no longer be served.
func Deprecation(sunsetDate time.Time) func(http.Handler) http.Handler {
    // Pre-format the Sunset value once at startup.
    sunsetValue := sunsetDate.UTC().Format(http.TimeFormat)

    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            w.Header().Set("Deprecation", "true")
            w.Header().Set("Sunset", sunsetValue)
            w.Header().Set("Link", `</docs/v2-migration>; rel="sunset"`)
            next.ServeHTTP(w, r)
        })
    }
}
```

Usage in `routes.go`:

```go
sunsetDate := time.Date(2028, 3, 24, 0, 0, 0, 0, time.UTC) // 12 months from v2 GA

r.Route("/api/v1", func(r chi.Router) {
    r.Use(middleware.Deprecation(sunsetDate))
    // ... existing v1 routes ...
})
```

---

## Endpoint Changes

### FIX-061: Verb-Based URLs to Noun-Based Resources

These endpoints use verbs in their URL path. In v2 they are renamed to noun-based resource paths.

| v1 Endpoint | v2 Endpoint | Rationale |
|-------------|-------------|-----------|
| `POST /detect-stack` | `POST /stack-detection` | Noun resource; accepts `{"path": "..."}` body, returns detected stack. POST retained because the request triggers server-side analysis. |
| `GET /projects/{id}/detect-stack` | `GET /projects/{id}/stack` | The stack *is* a sub-resource of the project. GET is idempotent -- detection is cached. |
| `POST /parse-repo-url` | `POST /repos/parse` | Groups under `/repos` namespace. POST retained because it accepts a body `{"url": "..."}`. |
| `GET /llm/discover` | `GET /llm/models/discovered` | Discovery results are a sub-resource of models. GET because the result is cacheable. |
| `POST /projects/{id}/decompose` | `POST /projects/{id}/decomposition` | Noun resource. POST triggers an LLM-powered decomposition (side effect). |
| `POST /projects/{id}/plan-feature` | `POST /projects/{id}/feature-plans` | Creates a new feature plan resource. |
| `POST /projects/{id}/roadmap/detect` | `POST /projects/{id}/roadmap/detection` | Noun resource. |
| `POST /projects/{id}/roadmap/sync` | `POST /projects/{id}/roadmap/syncs` | Creates a sync operation resource. |
| `POST /projects/{id}/roadmap/sync-to-file` | `POST /projects/{id}/roadmap/file-syncs` | Creates a file sync operation resource. |
| `POST /projects/{id}/roadmap/import` | `POST /projects/{id}/roadmap/imports` | Creates an import resource. |
| `POST /projects/{id}/roadmap/import/pm` | `POST /projects/{id}/roadmap/pm-imports` | Creates a PM import resource. |
| `POST /projects/{id}/graph/build` | `POST /projects/{id}/graph/builds` | Creates a graph build resource. |
| `POST /projects/{id}/graph/search` | `POST /projects/{id}/graph/searches` | Creates a search (POST because of complex query body). |
| `POST /projects/{id}/search` | `POST /projects/{id}/searches` | Noun resource for search results. |
| `POST /projects/{id}/search/agent` | `POST /projects/{id}/searches/agent` | Agent-powered search variant. |
| `POST /projects/{id}/index` | `POST /projects/{id}/indexes` | Creates an index build. |
| `POST /projects/{id}/boundaries/analyze` | `POST /projects/{id}/boundary-analyses` | Creates an analysis resource. |
| `POST /projects/{id}/review-refactor` | `POST /projects/{id}/review-refactors` | Creates a review-refactor operation. |
| `POST /projects/{id}/auto-agent/start` | `POST /projects/{id}/auto-agent` | Start = create the resource. |
| `POST /projects/{id}/auto-agent/stop` | `DELETE /projects/{id}/auto-agent` | Stop = delete the resource. |
| `POST /projects/{id}/lsp/start` | `POST /projects/{id}/lsp` | Start = create the LSP session. |
| `POST /projects/{id}/lsp/stop` | `DELETE /projects/{id}/lsp` | Stop = delete the LSP session. |
| `POST /projects/{id}/goals/detect` | `POST /projects/{id}/goals/detection` | Noun resource. |
| `POST /projects/{id}/goals/ai-discover` | `POST /projects/{id}/goals/ai-discoveries` | Noun resource. |
| `POST /runs/{id}/cancel` | `POST /runs/{id}/cancellation` | Noun resource for the cancellation action. |
| `POST /conversations/{id}/stop` | `POST /conversations/{id}/cancellation` | Noun resource. |
| `POST /conversations/{id}/compact` | `POST /conversations/{id}/compaction` | Noun resource. |
| `POST /conversations/{id}/clear` | `DELETE /conversations/{id}/messages` | Clear = delete all messages. |
| `POST /conversations/{id}/fork` | `POST /conversations/{id}/forks` | Creates a fork resource. |
| `POST /conversations/{id}/rewind` | `POST /conversations/{id}/rewinds` | Creates a rewind operation. |
| `POST /conversations/{id}/bypass-approvals` | `POST /conversations/{id}/approval-bypasses` | Noun resource. |
| `POST /conversations/{id}/mode` | `PUT /conversations/{id}/mode` | Setting mode is idempotent. |
| `POST /conversations/{id}/model` | `PUT /conversations/{id}/model` | Setting model is idempotent. |
| `POST /runs/{id}/resume` | `POST /runs/{id}/resumptions` | Noun resource. |
| `POST /runs/{id}/fork` | `POST /runs/{id}/forks` | Creates a fork. |
| `POST /runs/{id}/rewind` | `POST /runs/{id}/rewinds` | Creates a rewind. |
| `POST /runs/{id}/replay` | `POST /runs/{id}/replays` | Creates a replay. |
| `POST /review-policies/{id}/trigger` | `POST /review-policies/{id}/triggers` | Creates a trigger event. |
| `POST /policies/{name}/evaluate` | `POST /policies/{name}/evaluations` | Creates an evaluation. |
| `POST /policies/allow-always` | `POST /policies/permanent-allows` | Noun resource. |
| `POST /prompt-evolution/reflect` | `POST /prompt-evolution/reflections` | Creates a reflection. |
| `POST /prompt-evolution/revert/{modeId}` | `POST /prompt-evolution/reverts` | Creates a revert; `modeId` in body. |
| `POST /prompt-evolution/promote/{variantId}` | `POST /prompt-evolution/promotions` | Creates a promotion; `variantId` in body. |
| `POST /llm/refresh` | `POST /llm/model-refreshes` | Creates a refresh operation. |
| `POST /routing/stats/refresh` | `POST /routing/stat-refreshes` | Noun resource. |
| `POST /routing/seed-from-benchmarks` | `POST /routing/benchmark-seeds` | Noun resource. |
| `POST /agents/{id}/dispatch` | `POST /agents/{id}/dispatches` | Creates a dispatch. |
| `POST /agents/{id}/stop` | `POST /agents/{id}/stops` | Creates a stop request. |
| `POST /agents/{id}/inbox/{msgId}/read` | `PATCH /agents/{id}/inbox/{msgId}` | Partial update (mark as read). |
| `POST /dev/benchmark` | `POST /dev/benchmarks` | Noun resource. |
| `POST /mcp/servers/test` | `POST /mcp/server-tests` | Noun resource for pre-save test. |
| `POST /mcp/servers/{id}/test` | `POST /mcp/servers/{id}/tests` | Noun resource for server test. |
| `POST /vcs-accounts/{id}/test` | `POST /vcs-accounts/{id}/tests` | Noun resource. |
| `POST /copilot/exchange` | `POST /copilot/token-exchanges` | Noun resource. |
| `POST /scopes/{id}/search` | `POST /scopes/{id}/searches` | Noun resource. |
| `POST /scopes/{id}/graph/search` | `POST /scopes/{id}/graph/searches` | Noun resource. |
| `POST /users/{id}/force-password-change` | `POST /users/{id}/password-resets` | Noun resource. |
| `POST /users/{id}/export` | `POST /users/{id}/data-exports` | Noun resource. |
| `POST /a2a/agents/{id}/discover` | `POST /a2a/agents/{id}/discoveries` | Noun resource. |
| `POST /a2a/agents/{id}/send` | `POST /a2a/agents/{id}/tasks` | Creates a task. |
| `POST /a2a/tasks/{id}/cancel` | `POST /a2a/tasks/{id}/cancellation` | Noun resource. |
| `POST /benchmarks/compare` | `POST /benchmarks/comparisons` | Noun resource. |
| `POST /benchmarks/compare-multi` | `POST /benchmarks/multi-comparisons` | Noun resource. |
| `POST /benchmarks/runs/{id}/analyze` | `POST /benchmarks/runs/{id}/analyses` | Noun resource. |
| `POST /plans/{id}/start` | `POST /plans/{id}/executions` | Creates an execution. |
| `POST /plans/{id}/cancel` | `POST /plans/{id}/cancellation` | Noun resource. |
| `POST /plans/{id}/steps/{stepId}/evaluate` | `POST /plans/{id}/steps/{stepId}/evaluations` | Noun resource. |
| `POST /pipelines/{id}/instantiate` | `POST /pipelines/{id}/instances` | Creates an instance. |

**Endpoints that remain unchanged in v2** (already use proper noun-based naming):

All standard CRUD endpoints (`GET/POST/PUT/DELETE` on `/projects`, `/agents`, `/tasks`, `/runs`, `/modes`, `/pipelines`, `/scopes`, `/knowledge-bases`, `/milestones`, `/features`, `/channels`, `/tenants`, `/users`, `/policies`, `/mcp/servers`, `/microagents`, `/skills`, `/goals`, `/a2a/agents`, `/a2a/tasks`, `/benchmarks/suites`, `/benchmarks/runs`, `/branch-rules`, `/review-policies`, `/vcs-accounts`, `/llm-keys`, `/auth/*`, `/prompt-sections`, `/conversations`, `/settings`, etc.) keep the same paths.

---

### FIX-063: Pagination Envelope

#### Problem

Most list endpoints return a bare JSON array:

```json
[{"id": "abc", ...}, {"id": "def", ...}]
```

This makes it impossible for clients to know the total result count, or whether more pages exist, without counting the returned items and guessing.

A few endpoints (e.g., `POST /search`, `POST /search/conversations`, PM import with cursor pagination) already return enveloped responses, but each uses a different shape.

#### v2 Standard Envelope

All v2 list endpoints return:

```json
{
  "items": [...],
  "total": 42,
  "limit": 20,
  "offset": 0
}
```

| Field | Type | Description |
|-------|------|-------------|
| `items` | `[]T` | The page of results (never `null`, always `[]` when empty) |
| `total` | `int` | Total count of matching resources (before pagination) |
| `limit` | `int` | Maximum items per page (echo of request param) |
| `offset` | `int` | Number of items skipped (echo of request param) |

#### Go Helper

```go
// PagedResponse is the standard v2 pagination envelope.
type PagedResponse[T any] struct {
    Items  []T `json:"items"`
    Total  int `json:"total"`
    Limit  int `json:"limit"`
    Offset int `json:"offset"`
}

// writePaged writes a paginated response, applying offset/limit to the full
// result set and returning the standard envelope.
func writePaged[T any](w http.ResponseWriter, r *http.Request, items []T, maxLimit int) {
    total := len(items)
    limit, offset := parsePagination(r, maxLimit)
    items = applyPagination(items, limit, offset)
    writeJSON(w, http.StatusOK, PagedResponse[T]{
        Items:  items,
        Total:  total,
        Limit:  limit,
        Offset: offset,
    })
}
```

#### Affected Endpoints

All endpoints that return a list of resources. The following is a comprehensive catalog:

| Endpoint | Current Response | v2 Response |
|----------|-----------------|-------------|
| `GET /projects` | `[Project]` | `{items, total, limit, offset}` |
| `GET /projects/{id}/agents` | `[Agent]` | `{items, total, limit, offset}` |
| `GET /projects/{id}/tasks` | `[Task]` | `{items, total, limit, offset}` |
| `GET /projects/{id}/conversations` | `[Conversation]` | `{items, total, limit, offset}` |
| `GET /projects/{id}/files` | `[FileEntry]` | `{items, total, limit, offset}` |
| `GET /projects/{id}/files/tree` | `[TreeNode]` | `{items, total, limit, offset}` |
| `GET /projects/{id}/plans` | `[Plan]` | `{items, total, limit, offset}` |
| `GET /projects/{id}/sessions` | `[Session]` | `{items, total, limit, offset}` |
| `GET /projects/{id}/reviews` | `[Review]` | `{items, total, limit, offset}` |
| `GET /projects/{id}/review-policies` | `[ReviewPolicy]` | `{items, total, limit, offset}` |
| `GET /projects/{id}/branch-rules` | `[BranchRule]` | `{items, total, limit, offset}` |
| `GET /projects/{id}/memories` | `[Memory]` | `{items, total, limit, offset}` |
| `GET /projects/{id}/experience` | `[ExperienceEntry]` | `{items, total, limit, offset}` |
| `GET /projects/{id}/microagents` | `[Microagent]` | `{items, total, limit, offset}` |
| `GET /projects/{id}/skills` | `[Skill]` | `{items, total, limit, offset}` |
| `GET /projects/{id}/goals` | `[Goal]` | `{items, total, limit, offset}` |
| `GET /projects/{id}/mcp-servers` | `[MCPServer]` | `{items, total, limit, offset}` |
| `GET /projects/{id}/agents/active` | `[ActiveAgent]` | `{items, total, limit, offset}` |
| `GET /projects/{id}/active-work` | `[ActiveWork]` | `{items, total, limit, offset}` |
| `GET /conversations/{id}/messages` | `[Message]` | `{items, total, limit, offset}` |
| `GET /tasks/{id}/events` | `[Event]` | `{items, total, limit, offset}` |
| `GET /tasks/{id}/runs` | `[Run]` | `{items, total, limit, offset}` |
| `GET /runs/{id}/events` | `[Event]` | `{items, total, limit, offset}` |
| `GET /runs/{id}/checkpoints` | `[Checkpoint]` | `{items, total, limit, offset}` |
| `GET /runs/{id}/feedback` | `[FeedbackEntry]` | `{items, total, limit, offset}` |
| `GET /agents/{id}/inbox` | `[InboxMessage]` | `{items, total, limit, offset}` |
| `GET /modes` | `[Mode]` | `{items, total, limit, offset}` |
| `GET /pipelines` | `[Pipeline]` | `{items, total, limit, offset}` |
| `GET /scopes` | `[Scope]` | `{items, total, limit, offset}` |
| `GET /knowledge-bases` | `[KnowledgeBase]` | `{items, total, limit, offset}` |
| `GET /scopes/{id}/knowledge-bases` | `[KnowledgeBase]` | `{items, total, limit, offset}` |
| `GET /policies` | `{names: [...]}` | `{items, total, limit, offset}` |
| `GET /llm/models` | `[Model]` | `{items, total, limit, offset}` |
| `GET /llm/available` | `[Model]` | `{items, total, limit, offset}` |
| `GET /llm-keys` | `[LLMKey]` | `{items, total, limit, offset}` |
| `GET /vcs-accounts` | `[VCSAccount]` | `{items, total, limit, offset}` |
| `GET /mcp/servers` | `[MCPServer]` | `{items, total, limit, offset}` |
| `GET /mcp/servers/{id}/tools` | `[MCPTool]` | `{items, total, limit, offset}` |
| `GET /channels` | `[Channel]` | `{items, total, limit, offset}` |
| `GET /channels/{id}/messages` | `[ChannelMessage]` | `{items, total, limit, offset}` |
| `GET /tenants` | `[Tenant]` | `{items, total, limit, offset}` |
| `GET /users` | `[User]` | `{items, total, limit, offset}` |
| `GET /auth/api-keys` | `[APIKey]` | `{items, total, limit, offset}` |
| `GET /auth/providers` | `[Provider]` | `{items, total, limit, offset}` |
| `GET /commands` | `[Command]` | `{items, total, limit, offset}` |
| `GET /prompt-sections` | `[PromptSection]` | `{items, total, limit, offset}` |
| `GET /prompt-evolution/variants` | `[Variant]` | `{items, total, limit, offset}` |
| `GET /quarantine` | `[QuarantinedMsg]` | `{items, total, limit, offset}` |
| `GET /audit` | `[AuditEntry]` | `{items, total, limit, offset}` |
| `GET /projects/{id}/audit` | `[AuditEntry]` | `{items, total, limit, offset}` |
| `GET /audit-logs` | `[AuditLog]` | `{items, total, limit, offset}` |
| `GET /a2a/agents` | `[RemoteAgent]` | `{items, total, limit, offset}` |
| `GET /a2a/tasks` | `[A2ATask]` | `{items, total, limit, offset}` |
| `GET /a2a/tasks/{id}/push-config` | `[PushConfig]` | `{items, total, limit, offset}` |
| `GET /benchmarks/suites` | `[Suite]` | `{items, total, limit, offset}` |
| `GET /benchmarks/runs` | `[BenchmarkRun]` | `{items, total, limit, offset}` |
| `GET /benchmarks/runs/{id}/results` | `[Result]` | `{items, total, limit, offset}` |
| `GET /benchmarks/datasets` | `[Dataset]` | `{items, total, limit, offset}` |
| `GET /routing/stats` | `[RoutingStat]` | `{items, total, limit, offset}` |
| `GET /routing/outcomes` | `[Outcome]` | `{items, total, limit, offset}` |
| `GET /providers/git` | `[string]` | `{items, total, limit, offset}` |
| `GET /providers/agent` | `[string]` | `{items, total, limit, offset}` |
| `GET /providers/spec` | `[string]` | `{items, total, limit, offset}` |
| `GET /providers/pm` | `[string]` | `{items, total, limit, offset}` |

**Exceptions** (singleton resources, not lists -- no pagination needed):

- `GET /agent-config` -- returns a single config object.
- `GET /llm/health` -- returns a single health status.
- `GET /backends/health` -- returns a single health status.
- `GET /dashboard/stats` -- returns a single stats object.
- `GET /dashboard/charts/*` -- each returns a single chart data object.
- `GET /projects/{id}/health` -- returns a single health object.
- `GET /settings` -- returns a single settings object.
- `GET /quarantine/stats` -- returns a single stats object.
- `GET /projects/{id}/repomap` -- returns a single repomap.
- `GET /projects/{id}/index` -- returns a single index status.
- `GET /projects/{id}/graph/status` -- returns a single graph status.
- `GET /projects/{id}/workspace` -- returns a single workspace info.
- `GET /projects/{id}/git/status` -- returns a single git status.
- `GET /projects/{id}/roadmap` -- returns a single roadmap.
- `GET /projects/{id}/roadmap/ai` -- returns AI recommendations.
- `GET /projects/{id}/boundaries` -- returns a single boundaries object.
- `GET /conversations/{id}/session` -- returns a single session.
- `GET /prompt-evolution/status` -- returns a single status object.
- `GET /auto-agent/status` -- returns a single status object.
- `GET /agents/{id}/state` -- returns a single state object.
- `GET /runs/{id}/trajectory` -- returns a single trajectory.
- `GET /runs/{id}/trajectory/export` -- returns export data.
- `GET /tasks/{id}/context` -- returns a single context pack.
- `GET /plans/{id}/graph` -- returns a single graph.
- Search endpoints (`POST .../searches`) return their own envelope with `query`, `items`, `total`.

---

### FIX-095: CSRF Strategy

#### Decision: No CSRF Token Required for v2

**Rationale:**

1. **JSON-only API** -- All CodeForge API endpoints consume and produce `application/json`. Browsers cannot send JSON bodies via HTML `<form>` elements (forms only support `application/x-www-form-urlencoded` and `multipart/form-data`).

2. **SameSite=Lax cookies** -- The existing JWT access token is stored in a `SameSite=Lax` cookie, which prevents the browser from including it in cross-origin POST/PUT/DELETE/PATCH requests. This blocks CSRF for all state-changing methods.

3. **Content-Type enforcement** -- All mutating endpoints require `Content-Type: application/json` via the `readJSON` helper. Cross-origin `<form>` or `<img>` requests cannot set this header.

4. **No HTML form endpoints** -- CodeForge has zero HTML-form-based endpoints. The SolidJS frontend communicates exclusively via `fetch()` with JSON bodies.

**Documented safeguards (already in place):**

| Layer | Protection |
|-------|-----------|
| Cookie | `SameSite=Lax; HttpOnly; Secure` (in production) |
| Content-Type | `readJSON` rejects non-JSON bodies |
| CORS | Strict `Access-Control-Allow-Origin` (not `*`) |
| Auth | Bearer token alternative for API keys (no cookies) |

**Future consideration:** If CodeForge ever adds HTML form-based endpoints (e.g., OAuth callback with form POST), a CSRF token middleware (`gorilla/csrf` or custom double-submit cookie) must be added to those specific routes. This is tracked as a conditional TODO, not a v2 blocker.

---

### FIX-098: POST Used for DELETE Operations

Two endpoints use POST to perform delete operations, violating HTTP method semantics.

| v1 Endpoint | v1 Method | v2 Endpoint | v2 Method | Notes |
|-------------|-----------|-------------|-----------|-------|
| `/llm/models/delete` | `POST` (body: `{"id": "..."}`) | `/llm/models/{id}` | `DELETE` | ID moves from body to URL path param |
| `/projects/batch/delete` | `POST` (body: `{"ids": [...]}`) | `/projects/batch` | `DELETE` (body: `{"ids": [...]}`) | Batch delete keeps body for ID list; method changes to DELETE |

#### Handler Changes

**`DELETE /llm/models/{id}`**: The handler extracts the model ID from the URL path (`chi.URLParam(r, "id")`) instead of parsing a JSON body. The existing `DeleteModel` service call is unchanged.

**`DELETE /projects/batch`**: The handler continues to read `{"ids": [...]}` from the request body (HTTP DELETE with body is allowed per RFC 9110 Section 9.3.5, though semantics are implementation-defined). The existing `BatchDeleteProjects` service call is unchanged.

---

### FIX-100: PUT Used for Partial Updates

PUT semantics require a complete resource representation. Several endpoints accept partial payloads (only the fields being changed), which should use PATCH.

#### Analysis

The following PUT endpoints accept partial payloads and must change to PATCH in v2:

| v1 Endpoint | v1 Method | v2 Method | Rationale |
|-------------|-----------|-----------|-----------|
| `PUT /projects/{id}` | PUT | PATCH | Clients send only changed fields (e.g., `{"name": "new-name"}`) |
| `PUT /agents/{id}/state` | PUT | PATCH | Partial state update |
| `PUT /modes/{id}` | PUT | PATCH | Partial mode update |
| `PUT /scopes/{id}` | PUT | PATCH | Partial scope update |
| `PUT /knowledge-bases/{id}` | PUT | PATCH | Partial KB update |
| `PUT /projects/{id}/roadmap` | PUT | PATCH | Partial roadmap update |
| `PUT /milestones/{id}` | PUT | PATCH | Partial milestone update |
| `PUT /features/{id}` | PUT | PATCH | Partial feature update |
| `PUT /tenants/{id}` | PUT | PATCH | Partial tenant update |
| `PUT /branch-rules/{id}` | PUT | PATCH | Partial rule update |
| `PUT /review-policies/{id}` | PUT | PATCH | Partial policy update |
| `PUT /settings` | PUT | PATCH | Partial settings update |
| `PUT /mcp/servers/{id}` | PUT | PATCH | Partial server config update |
| `PUT /users/{id}` | PUT | PATCH | Partial user update |
| `PUT /microagents/{id}` | PUT | PATCH | Partial microagent update |
| `PUT /skills/{id}` | PUT | PATCH | Partial skill update |
| `PUT /goals/{id}` | PUT | PATCH | Partial goal update |
| `PUT /channels/{id}/members/{uid}` | PUT | PATCH | Partial notification settings update |
| `PUT /projects/{id}/boundaries` | PUT | PATCH | Partial boundaries update |
| `PUT /benchmarks/suites/{id}` | PUT | PATCH | Partial suite update |

**Endpoints that remain PUT** (full replacement semantics):

| Endpoint | Rationale |
|----------|-----------|
| `PUT /projects/{id}/files/content` | Full file content replacement (write the entire file) |
| `PUT /prompt-sections` | Upsert -- full section replacement |

#### Content-Type

PATCH requests use `Content-Type: application/json` (not `application/merge-patch+json` or `application/json-patch+json`). The merge semantics are implicit: provided fields overwrite, omitted fields are left unchanged. This matches the existing handler behavior.

---

## Complete v2 Route Map

For reference, below is the complete v2 route structure showing only endpoints that changed. All other endpoints retain their v1 paths under the `/api/v2/` prefix.

```
CHANGED ENDPOINTS (v1 -> v2):

DELETE /api/v2/llm/models/{id}                          (was: POST /llm/models/delete)
DELETE /api/v2/projects/batch                            (was: POST /projects/batch/delete)

POST   /api/v2/stack-detection                           (was: POST /detect-stack)
GET    /api/v2/projects/{id}/stack                       (was: GET  /projects/{id}/detect-stack)
POST   /api/v2/repos/parse                               (was: POST /parse-repo-url)
GET    /api/v2/llm/models/discovered                     (was: GET  /llm/discover)
POST   /api/v2/projects/{id}/decomposition               (was: POST /projects/{id}/decompose)
POST   /api/v2/projects/{id}/feature-plans               (was: POST /projects/{id}/plan-feature)

PATCH  /api/v2/projects/{id}                             (was: PUT  /projects/{id})
PATCH  /api/v2/agents/{id}/state                         (was: PUT  /agents/{id}/state)
PATCH  /api/v2/modes/{id}                                (was: PUT  /modes/{id})
PATCH  /api/v2/scopes/{id}                               (was: PUT  /scopes/{id})
PATCH  /api/v2/knowledge-bases/{id}                      (was: PUT  /knowledge-bases/{id})
PATCH  /api/v2/projects/{id}/roadmap                     (was: PUT  /projects/{id}/roadmap)
PATCH  /api/v2/milestones/{id}                           (was: PUT  /milestones/{id})
PATCH  /api/v2/features/{id}                             (was: PUT  /features/{id})
PATCH  /api/v2/tenants/{id}                              (was: PUT  /tenants/{id})
PATCH  /api/v2/branch-rules/{id}                         (was: PUT  /branch-rules/{id})
PATCH  /api/v2/review-policies/{id}                      (was: PUT  /review-policies/{id})
PATCH  /api/v2/settings                                  (was: PUT  /settings)
PATCH  /api/v2/mcp/servers/{id}                          (was: PUT  /mcp/servers/{id})
PATCH  /api/v2/users/{id}                                (was: PUT  /users/{id})
PATCH  /api/v2/microagents/{id}                          (was: PUT  /microagents/{id})
PATCH  /api/v2/skills/{id}                               (was: PUT  /skills/{id})
PATCH  /api/v2/goals/{id}                                (was: PUT  /goals/{id})
PATCH  /api/v2/channels/{id}/members/{uid}               (was: PUT  /channels/{id}/members/{uid})
PATCH  /api/v2/projects/{id}/boundaries                  (was: PUT  /projects/{id}/boundaries)
PATCH  /api/v2/benchmarks/suites/{id}                    (was: PUT  /benchmarks/suites/{id})

ALL LIST ENDPOINTS: Response shape changes from [...] to {items, total, limit, offset}
```

---

## Migration Guide Outline

The following sections will form the client migration guide, published at `/docs/v2-migration` and linked from the `Sunset` header.

### 1. URL Changes

Find-and-replace table for client code. Every changed URL is listed with its v1 and v2 form. Clients using SDK/client libraries only need to update the base path from `/api/v1/` to `/api/v2/`.

### 2. HTTP Method Changes

| Change | Endpoints | Client Action |
|--------|-----------|---------------|
| POST to DELETE | `/llm/models/delete`, `/projects/batch/delete` | Change `fetch()` method to `DELETE`; for `/llm/models/{id}`, move ID from body to URL |
| PUT to PATCH | 20 endpoints (see FIX-100 table) | Change `fetch()` method to `PATCH`; no body changes needed |
| POST to PUT | `/conversations/{id}/mode`, `/conversations/{id}/model` | Change `fetch()` method to `PUT` |
| POST to DELETE | `/projects/{id}/auto-agent/stop`, `/projects/{id}/lsp/stop`, `/conversations/{id}/clear` | Change `fetch()` method to `DELETE`; adjust URL if path changed |

### 3. Response Format Changes

All list endpoints now return `{items, total, limit, offset}` instead of a bare array.

**Before (v1):**
```javascript
const projects = await fetch('/api/v1/projects').then(r => r.json());
// projects is an array: [{id: "abc"}, ...]
for (const p of projects) { ... }
```

**After (v2):**
```javascript
const response = await fetch('/api/v2/projects').then(r => r.json());
// response is an envelope: {items: [{id: "abc"}, ...], total: 42, limit: 100, offset: 0}
for (const p of response.items) { ... }
```

### 4. Timeline and Sunset Dates

| Date | Event |
|------|-------|
| v2 GA | v2 available, v1 deprecated (headers added) |
| v2 GA + 6mo | Reminder notice in changelog, email to registered API key owners |
| v2 GA + 12mo | v1 returns `410 Gone` |
| v2 GA + 15mo | v1 routes removed from source |

### 5. Deprecation Header Detection

Clients should monitor for the `Deprecation: true` header and log a warning:

```javascript
const res = await fetch('/api/v1/projects');
if (res.headers.get('Deprecation') === 'true') {
    console.warn(`API v1 is deprecated. Sunset: ${res.headers.get('Sunset')}. Migrate to v2.`);
}
```

### 6. Frontend Migration

The SolidJS frontend will be updated to use v2 endpoints as part of the v2 rollout. The `apiClient` wrapper in `frontend/src/lib/api.ts` will switch its base URL from `/api/v1` to `/api/v2`. All `fetch()` calls route through this wrapper, so the migration is a single-line change plus response unpacking for list endpoints.

---

## Implementation Notes

### Phase 1: Infrastructure (No breaking changes)

1. Add `Deprecation` middleware to `internal/middleware/deprecation.go`
2. Add `PagedResponse[T]` type and `writePaged` helper to `internal/adapter/http/helpers.go`
3. Add v2 route group skeleton in `routes.go` (initially empty)

### Phase 2: v2 Routes (Breaking changes behind `/api/v2/`)

1. Mount all unchanged endpoints in the v2 route group (reusing existing handlers)
2. Create new handlers or wrapper handlers for changed endpoints:
   - `DeleteLLMModelV2` -- reads ID from URL path
   - `BatchDeleteProjectsV2` -- uses `r.Delete` instead of `r.Post`
   - All list handlers wrapped with `writePaged` instead of `writeJSONList`/`writeJSON`
3. Wire renamed noun-based routes to existing handlers
4. Wire PATCH routes to existing update handlers

### Phase 3: Frontend Migration

1. Update `apiClient` base URL
2. Update list response consumers to read from `.items`
3. Update HTTP methods for affected endpoints

### Phase 4: Deprecation Enforcement

1. Apply `Deprecation` middleware to v1 route group
2. After 12 months, replace v1 route group with `410 Gone` handler
3. After 15 months, remove v1 code

---

## References

- RFC 9745 -- The Deprecation HTTP Header Field
- RFC 8594 -- The Sunset HTTP Header Field
- RFC 8288 -- Web Linking (Link header)
- RFC 9110 Section 9.3.5 -- DELETE method semantics
- RFC 5789 -- PATCH Method for HTTP
- `internal/adapter/http/routes.go` -- current v1 route definitions
- `internal/adapter/http/helpers.go` -- `parsePagination`, `applyPagination`, `writeJSONList`
- `internal/adapter/http/crud.go` -- generic CRUD handler factories
