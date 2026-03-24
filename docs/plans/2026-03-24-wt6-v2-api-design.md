# WT6: v2 API Design Document

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Produce a comprehensive v2 API migration design document covering FIX-061, FIX-063, FIX-095, FIX-098, and FIX-100. No code changes — design doc only.

**Architecture:** URL path versioning (`/api/v2/`), parallel operation with v1, `Deprecation` + `Sunset` headers on v1 via middleware.

**Tech Stack:** Go, chi v5, REST

**Best Practices Applied:**
- URL path versioning (most cache-friendly, most widely adopted)
- `Deprecation` + `Sunset` + `Link` headers per RFC 9745 / RFC 8594
- 12-month deprecation window for v1
- Semantic versioning at API level, not resource level

---

### Task 1: Audit current v1 endpoints for v2 changes

**Files:**
- Read: `internal/adapter/http/routes.go` (full file)

- [ ] **Step 1: Catalog all endpoints needing v2 changes**

Read `routes.go` fully and create a table of every endpoint that needs to change:

| Current v1 Endpoint | Issue | v2 Endpoint | FIX |
|---------------------|-------|-------------|-----|
| `POST /detect-stack` | Verb URL | `GET /projects/{id}/stack` | FIX-061 |
| `POST /parse-repo-url` | Verb URL | `GET /repos/parse?url=...` | FIX-061 |
| `POST /discover` | Verb URL | `GET /projects/{id}/discovery` | FIX-061 |
| `POST /decompose` | Verb URL | `POST /projects/{id}/tasks` | FIX-061 |
| `POST /llm/models/delete` | POST for DELETE | `DELETE /llm/models/{id}` | FIX-098 |
| `POST /projects/batch/delete` | POST for DELETE | `DELETE /projects/batch` | FIX-098 |
| List endpoints missing pagination | No envelope | Add `{items, total, limit, offset}` | FIX-063 |
| PUT endpoints with partial payloads | PUT for PATCH | `PATCH /...` | FIX-100 |

Verify each endpoint against the actual routes.

- [ ] **Step 2: Document all list endpoints and their current pagination status**

Grep for list/collection endpoints and check whether they return a pagination envelope.

---

### Task 2: Write the v2 API design document

**Files:**
- Create: `docs/specs/v2-api-migration-design.md`

- [ ] **Step 1: Write the design document**

```markdown
# v2 API Migration Design

## Status
Draft — 2026-03-24

## Overview
Design for migrating CodeForge REST API from v1 to v2, addressing 5 documented issues:
- **FIX-061:** Verb-based URLs → noun-based resources
- **FIX-063:** Inconsistent pagination → standard envelope
- **FIX-095:** CSRF token strategy (SameSite sufficient for JSON API)
- **FIX-098:** POST for DELETE → proper HTTP DELETE
- **FIX-100:** PUT for partial updates → PATCH

## Versioning Strategy
- **URL path versioning:** `/api/v2/` prefix
- **Parallel operation:** v1 and v2 run simultaneously
- **Deprecation timeline:** 12 months from v2 GA to v1 sunset

## Deprecation Middleware
v1 routes get `Deprecation` middleware that sets:
- `Deprecation: true` header
- `Sunset: <date>` header (12 months from v2 release)
- `Link: <docs/migration-guide>; rel="sunset"` header

```go
// middleware.Deprecation returns middleware that sets deprecation headers.
func Deprecation(sunset time.Time) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            w.Header().Set("Deprecation", "true")
            w.Header().Set("Sunset", sunset.Format(http.TimeFormat))
            w.Header().Set("Link", `</docs/v2-migration>; rel="sunset"`)
            next.ServeHTTP(w, r)
        })
    }
}
```

## Endpoint Changes

### FIX-061: Verb URLs → Noun Resources
[Table from Task 1 Step 1]

### FIX-063: Pagination Envelope
All list endpoints return:
```json
{
  "items": [...],
  "total": 42,
  "limit": 20,
  "offset": 0
}
```

### FIX-095: CSRF Strategy
SameSite=Lax is sufficient for JSON API-only endpoints.
Decision: No CSRF token needed for v2 (no HTML form posts).
If HTML form support is ever added, add CSRF middleware at that point.

### FIX-098: POST → DELETE
[Table of affected endpoints]

### FIX-100: PUT → PATCH
[Table of affected endpoints]

## Route Structure
```go
r.Route("/api/v1", func(r chi.Router) {
    r.Use(middleware.Deprecation(sunsetDate))
    // ... existing v1 routes unchanged ...
})

r.Route("/api/v2", func(r chi.Router) {
    // ... new v2 routes with fixes applied ...
})
```

## Migration Guide Outline
1. URL changes (find/replace table)
2. HTTP method changes (DELETE, PATCH)
3. Response format changes (pagination envelope)
4. Timeline and sunset dates
```

- [ ] **Step 2: Review the document for completeness**

Verify every FIX is addressed with specific before/after examples.

- [ ] **Step 3: Commit**

```bash
git add docs/specs/v2-api-migration-design.md
git commit -m "docs: v2 API migration design covering FIX-061/063/095/098/100"
```

---

### Task 3: Update docs/todo.md

**Files:**
- Modify: `docs/todo.md`

- [ ] **Step 1: Add v2 API design reference**

Add a new entry under the appropriate section:
```markdown
- [x] [2026-03-24] v2 API migration design document (`docs/specs/v2-api-migration-design.md`) — FIX-061/063/095/098/100
```

- [ ] **Step 2: Commit**

```bash
git add docs/todo.md
git commit -m "docs: add v2 API design doc reference to todo.md"
```
