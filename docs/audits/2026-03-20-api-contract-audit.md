# API Contract Audit Report

**Date:** 2026-03-20
**Scope:** Architecture + Code Review
**Files Reviewed:** 37 files (routes.go, helpers.go, crud.go, handlers.go, 30 handlers_*.go, middleware.go)
**Score: 72/100 -- Grade: C** (post-fix: 96/100 -- Grade: A)

---

## Executive Summary

| Severity | Count | Category Breakdown |
|----------|------:|---------------------|
| CRITICAL | 1     | Internal error leakage |
| HIGH     | 4     | PathValue/URLParam mix, http.Error inconsistency, delete response inconsistency, pagination absence |
| MEDIUM   | 5     | Verb-in-URL violations, queryInt duplication, missing list pagination, error msg leak in search, no 422 usage |
| LOW      | 4     | Quarantine lowercase handlers, batch POST-for-delete, ad-hoc status messages, missing PATCH usage |
| **Total**| **14** |                     |

### Positive Findings

- **Consistent API prefix:** All routes are mounted under `/api/v1/` with clear versioning strategy (v2 deprecation pattern documented in routes.go)
- **Unified error structure:** Single `errorResponse{Error string}` JSON shape used across all handlers via `writeError()`, `writeDomainError()`, `writeInternalError()`
- **Generic CRUD factories:** `handleList`, `handleGet`, `handleCreate`, `handleUpdate`, `handleDelete` in `crud.go` reduce boilerplate and enforce consistency for simple resources
- **Null-safe list responses:** `writeJSONList()` and manual nil-to-empty-slice conversions consistently prevent `null` JSON arrays
- **Proper HTTP semantics for most CRUD:** Creates return 201, deletes return 204 (mostly), GETs return 200
- **Security headers middleware:** Comprehensive CSP, X-Frame-Options, nosniff headers applied globally
- **Domain error mapping:** `writeDomainError()` correctly maps `ErrNotFound` -> 404, `ErrConflict` -> 409, `ErrValidation` -> 400, unique constraint -> 409
- **Body size limiting:** All `readJSON` calls use `MaxBytesReader` with configurable limits
- **Resource naming:** Plural nouns used consistently (projects, agents, tasks, runs, modes, pipelines, scopes, etc.)
- **Nested resource pattern:** Well-structured nesting (e.g., `/projects/{id}/conversations`, `/projects/{id}/agents`)
- **Role-based access control:** `middleware.RequireRole()` applied consistently for mutation endpoints

---

## Endpoint Inventory

### Projects (8 endpoints)
| Method | Path | Auth | Status Codes | Handler File |
|--------|------|------|-------------|-------------|
| GET | /projects | yes | 200, 500 | handlers.go:108 |
| POST | /projects | editor+ | 201, 400, 409, 500 | handlers.go:132 |
| GET | /projects/{id} | yes | 200, 404 | handlers.go:121 |
| PUT | /projects/{id} | editor+ | 200, 400, 404 | handlers.go:162 |
| DELETE | /projects/{id} | admin | 204, 404 | handlers.go:152 |
| GET | /projects/remote-branches | yes | 200, 400, 502 | handlers.go:498 |
| POST | /projects/batch/delete | editor+ | 200, 400 | handlers_batch.go:32 |
| POST | /projects/batch/pull | editor+ | 200, 400 | handlers_batch.go:49 |
| POST | /projects/batch/status | editor+ | 200, 400 | handlers_batch.go:67 |

### Workspace Operations (5 endpoints)
| Method | Path | Auth | Status Codes | Handler File |
|--------|------|------|-------------|-------------|
| POST | /projects/{id}/clone | editor+ | 200, 404, 500 | handlers.go:298 |
| POST | /projects/{id}/adopt | editor+ | 200, 400, 404 | handlers.go:321 |
| POST | /projects/{id}/setup | editor+ | 200, 404, 500 | handlers.go:371 |
| POST | /projects/{id}/init-workspace | editor+ | 200, 404 | handlers.go:357 |
| GET | /projects/{id}/workspace | yes | 200, 404 | handlers.go:396 |

### File Operations (6 endpoints)
| Method | Path | Auth | Status Codes | Handler File |
|--------|------|------|-------------|-------------|
| GET | /projects/{id}/files | yes | 200, 404 | handlers_files.go:10 |
| GET | /projects/{id}/files/tree | yes | 200, 404 | handlers_files.go:26 |
| GET | /projects/{id}/files/content | yes | 200, 400, 404 | handlers_files.go:42 |
| PUT | /projects/{id}/files/content | editor+ | 200, 400, 404 | handlers_files.go:59 |
| DELETE | /projects/{id}/files | editor+ | 200, 400, 404 | handlers_files.go:88 |
| PATCH | /projects/{id}/files/rename | editor+ | 200, 400, 404 | handlers_files.go:104 |

### Stack Detection (2 endpoints)
| Method | Path | Auth | Status Codes | Handler File |
|--------|------|------|-------------|-------------|
| GET | /projects/{id}/detect-stack | yes | 200, 404 | handlers.go:408 |
| POST | /detect-stack | yes | 200, 400 | handlers.go:419 |

### Git Operations (4 endpoints)
| Method | Path | Auth | Status Codes | Handler File |
|--------|------|------|-------------|-------------|
| GET | /projects/{id}/git/status | yes | 200, 404 | handlers.go:444 |
| POST | /projects/{id}/git/pull | editor+ | 200, 404 | handlers.go:455 |
| GET | /projects/{id}/git/branches | yes | 200, 404 | handlers.go:465 |
| POST | /projects/{id}/git/checkout | editor+ | 200, 400, 404 | handlers.go:476 |

### Agents (9 endpoints)
| Method | Path | Auth | Status Codes | Handler File |
|--------|------|------|-------------|-------------|
| GET | /projects/{id}/agents | yes | 200, 404 | handlers.go:559 |
| POST | /projects/{id}/agents | editor+ | 201, 400, 404 | handlers.go:573 |
| GET | /agents/{id} | yes | 200, 404 | handlers.go:603 |
| DELETE | /agents/{id} | admin | 204, 404 | handlers.go:614 |
| POST | /agents/{id}/dispatch | editor+ | 200, 400, 404 | handlers.go:624 |
| POST | /agents/{id}/stop | editor+ | 200, 400, 404 | handlers.go:646 |
| GET | /agents/{id}/inbox | yes | 200, 404 | handlers.go:668 |
| POST | /agents/{id}/inbox | editor+ | 201, 400, 404 | handlers.go:681 |
| POST | /agents/{id}/inbox/{msgId}/read | yes | 200, 404 | handlers.go:707 |

### Agent State & Identity (3 endpoints)
| Method | Path | Auth | Status Codes | Handler File |
|--------|------|------|-------------|-------------|
| GET | /agents/{id}/state | yes | 200, 404 | handlers.go:717 |
| PUT | /agents/{id}/state | editor+ | 200, 400, 404 | handlers.go:728 |
| GET | /projects/{id}/agents/active | yes | 200, 404 | handlers.go:1061 |

### Tasks (5 endpoints)
| Method | Path | Auth | Status Codes | Handler File |
|--------|------|------|-------------|-------------|
| GET | /projects/{id}/tasks | yes | 200, 404 | handlers.go:213 |
| POST | /projects/{id}/tasks | yes | 201, 400 | handlers.go:227 |
| GET | /tasks/{id} | yes | 200, 404 | handlers.go:250 |
| GET | /tasks/{id}/events | yes | 200, 404 | handlers.go:743 |
| GET | /tasks/{id}/runs | yes | 200, 404 | handlers.go:1009 |

### Task Context & Claim (3 endpoints)
| Method | Path | Auth | Status Codes | Handler File |
|--------|------|------|-------------|-------------|
| GET | /tasks/{id}/context | yes | 200, 404 | handlers_orchestration.go:242 |
| POST | /tasks/{id}/context | yes | 201, 400, 404 | handlers_orchestration.go:253 |
| POST | /tasks/{id}/claim | yes | 200, 400, 409 | handlers.go:1078 |

### Runs (8 endpoints)
| Method | Path | Auth | Status Codes | Handler File |
|--------|------|------|-------------|-------------|
| POST | /runs | yes | 201, 400, 404 | handlers.go:965 |
| GET | /runs/{id} | yes | 200, 404 | handlers.go:988 |
| POST | /runs/{id}/cancel | yes | 200, 404 | handlers.go:999 |
| GET | /runs/{id}/events | yes | 200, 404 | handlers.go:1023 |
| POST | /runs/{id}/approve | editor+ | 200, 400, 404 | handlers_review.go:75 |
| POST | /runs/{id}/reject | editor+ | 200, 400, 404 | handlers_review.go:92 |
| POST | /runs/{id}/approve-partial | editor+ | 200, 400, 404 | handlers_review.go:109 |
| POST | /runs/{id}/approve/{callId} | yes | 200, 400, 404 | handlers_conversation.go:127 |

### Conversations (14 endpoints)
| Method | Path | Auth | Status Codes | Handler File |
|--------|------|------|-------------|-------------|
| POST | /projects/{id}/conversations | yes | 201, 400 | handlers_conversation.go:12 |
| GET | /projects/{id}/conversations | yes | 200, 404 | handlers_conversation.go:28 |
| GET | /conversations/{id} | yes | 200, 404 | handlers_conversation.go:39 |
| DELETE | /conversations/{id} | yes | 204, 404 | handlers_conversation.go:49 |
| GET | /conversations/{id}/messages | yes | 200, 404 | handlers_conversation.go:59 |
| POST | /conversations/{id}/messages | yes | 202, 400, 404 | handlers_conversation.go:74 |
| POST | /conversations/{id}/stop | yes | 200, 404 | handlers_conversation.go:107 |
| POST | /conversations/{id}/bypass-approvals | yes | 200 | handlers_conversation.go:159 |
| POST | /conversations/{id}/fork | yes | 201, 404 | handlers_session.go:237 |
| POST | /conversations/{id}/rewind | yes | 201, 404 | handlers_session.go:254 |
| POST | /conversations/{id}/compact | yes | 200, 404 | handlers_conversation.go:169 |
| POST | /conversations/{id}/clear | yes | 200, 404 | handlers_conversation.go:183 |
| POST | /conversations/{id}/mode | yes | 200, 400, 404 | handlers_conversation.go:197 |
| POST | /conversations/{id}/model | yes | 200, 400, 404 | handlers_conversation.go:226 |

### Sessions (5 endpoints)
| Method | Path | Auth | Status Codes | Handler File |
|--------|------|------|-------------|-------------|
| POST | /runs/{id}/resume | yes | 201, 404 | handlers_session.go:150 |
| POST | /runs/{id}/fork | yes | 201, 404 | handlers_session.go:168 |
| POST | /runs/{id}/rewind | yes | 201, 404 | handlers_session.go:186 |
| GET | /projects/{id}/sessions | yes | 200, 404 | handlers_session.go:204 |
| GET | /sessions/{id} | yes | 200, 404 | handlers_session.go:215 |

### Trajectory & Replay (5 endpoints)
| Method | Path | Auth | Status Codes | Handler File |
|--------|------|------|-------------|-------------|
| GET | /runs/{id}/trajectory | yes | 200, 500 | handlers_roadmap.go:416 |
| GET | /runs/{id}/trajectory/export | yes | 200, 404, 500 | handlers_roadmap.go:468 |
| GET | /runs/{id}/checkpoints | yes | 200, 404 | handlers_session.go:86 |
| POST | /runs/{id}/replay | yes | 200, 400, 404 | handlers_session.go:97 |
| POST | /runs/{id}/revert/{callId} | yes | 200, 404, 500 | handlers_conversation.go:255 |

### LLM Management (7 endpoints)
| Method | Path | Auth | Status Codes | Handler File |
|--------|------|------|-------------|-------------|
| GET | /llm/models | yes | 200, 502 | handlers_llm.go:12 |
| POST | /llm/models | yes | 201, 400, 502 | handlers_llm.go:23 |
| POST | /llm/models/delete | yes | 200, 400, 502 | handlers_llm.go:42 |
| GET | /llm/health | yes | 200 | handlers_llm.go:63 |
| GET | /llm/discover | yes | 200, 502 | handlers_llm.go:74 |
| GET | /llm/available | yes | 200, 503 | handlers_llm.go:111 |
| POST | /llm/refresh | yes | 200, 500, 503 | handlers_llm.go:127 |

### LLM Keys (3 endpoints)
| Method | Path | Auth | Status Codes | Handler File |
|--------|------|------|-------------|-------------|
| GET | /llm-keys | yes | 200, 401, 500 | handlers_llm_keys.go:13 |
| POST | /llm-keys | yes | 201, 400, 401 | handlers_llm_keys.go:29 |
| DELETE | /llm-keys/{id} | yes | 204, 401, 404 | handlers_llm_keys.go:50 |

### Copilot (1 endpoint)
| Method | Path | Auth | Status Codes | Handler File |
|--------|------|------|-------------|-------------|
| POST | /copilot/exchange | yes | 200, 404, 502 | handlers_llm.go:142 |

### Policies (5 endpoints)
| Method | Path | Auth | Status Codes | Handler File |
|--------|------|------|-------------|-------------|
| GET | /policies | yes | 200 | handlers.go:775 |
| POST | /policies | yes | 201, 400, 404 | handlers.go:815 |
| POST | /policies/allow-always | yes | 200, 400, 404, 500 | handlers.go:872 |
| GET | /policies/{name} | yes | 200, 404 | handlers.go:782 |
| DELETE | /policies/{name} | yes | 204, 400, 403, 404 | handlers.go:843 |

### Execution Plans (5 endpoints)
| Method | Path | Auth | Status Codes | Handler File |
|--------|------|------|-------------|-------------|
| POST | /projects/{id}/plans | yes | 201, 400, 404 | handlers_orchestration.go:18 |
| GET | /projects/{id}/plans | yes | 200, 404 | handlers_orchestration.go:36 |
| GET | /plans/{id} | yes | 200, 404 | handlers_orchestration.go:47 |
| POST | /plans/{id}/start | yes | 200, 404 | handlers_orchestration.go:58 |
| POST | /plans/{id}/cancel | yes | 200, 404 | handlers_orchestration.go:69 |
| GET | /plans/{id}/graph | yes | 200, 404 | handlers_orchestration.go:128 |
| POST | /plans/{id}/steps/{stepId}/evaluate | yes | 200, 404, 500, 503 | handlers_orchestration.go:80 |

### Modes (6 endpoints)
| Method | Path | Auth | Status Codes | Handler File |
|--------|------|------|-------------|-------------|
| GET | /modes | yes | 200 | handlers_orchestration.go:279 |
| GET | /modes/{id} | yes | 200, 404 | handlers_orchestration.go:285 |
| POST | /modes | yes | 201, 400, 404 | handlers_orchestration.go:311 |
| PUT | /modes/{id} | yes | 200, 400, 404 | handlers_orchestration.go:324 |
| DELETE | /modes/{id} | yes | 204, 404 | handlers_orchestration.go:339 |
| GET | /modes/scenarios | yes | 200 | handlers_orchestration.go:296 |
| GET | /modes/tools | yes | 200 | handlers_orchestration.go:301 |
| GET | /modes/artifact-types | yes | 200 | handlers_orchestration.go:306 |

### Pipelines (4 endpoints)
| Method | Path | Auth | Status Codes | Handler File |
|--------|------|------|-------------|-------------|
| GET | /pipelines | yes | 200 | handlers_orchestration.go:351 |
| POST | /pipelines | yes | 201, 400, 404 | handlers_orchestration.go:368 |
| GET | /pipelines/{id} | yes | 200, 404 | handlers_orchestration.go:357 |
| POST | /pipelines/{id}/instantiate | yes | 200, 400, 404 | handlers_orchestration.go:381 |

### Feature Decomposition (2 endpoints)
| Method | Path | Auth | Status Codes | Handler File |
|--------|------|------|-------------|-------------|
| POST | /projects/{id}/decompose | yes | 201, 400, 404 | handlers_orchestration.go:202 |
| POST | /projects/{id}/plan-feature | yes | 201, 400, 404 | handlers_orchestration.go:222 |

### Retrieval & Search (7 endpoints)
| Method | Path | Auth | Status Codes | Handler File |
|--------|------|------|-------------|-------------|
| POST | /projects/{id}/search | yes | 200, 400, 504 | handlers_retrieval.go:74 |
| POST | /projects/{id}/search/agent | yes | 200, 400, 504 | handlers_retrieval.go:115 |
| POST | /projects/{id}/index | yes | 202, 404 | handlers_retrieval.go:45 |
| GET | /projects/{id}/index | yes | 200, 404 | handlers_retrieval.go:63 |
| GET | /projects/{id}/repomap | yes | 200, 404 | handlers_retrieval.go:14 |
| POST | /projects/{id}/repomap | yes | 202, 404 | handlers_retrieval.go:25 |
| POST | /search | yes | 200, 400, 500 | handlers_search.go:80 |
| POST | /search/conversations | yes | 200, 400, 500 | handlers_search.go:23 |

### GraphRAG (3 endpoints)
| Method | Path | Auth | Status Codes | Handler File |
|--------|------|------|-------------|-------------|
| POST | /projects/{id}/graph/build | yes | 202, 404, 500 | handlers_retrieval.go:178 |
| GET | /projects/{id}/graph/status | yes | 200, 404 | handlers_retrieval.go:194 |
| POST | /projects/{id}/graph/search | yes | 200, 400, 504 | handlers_retrieval.go:205 |

### Scopes (9 endpoints)
| Method | Path | Auth | Status Codes | Handler File |
|--------|------|------|-------------|-------------|
| POST | /scopes | yes | 201, 400, 404 | handlers_scope.go:14 |
| GET | /scopes | yes | 200, 404 | handlers_scope.go:29 |
| GET | /scopes/{id} | yes | 200, 404 | handlers_scope.go:39 |
| PUT | /scopes/{id} | yes | 200, 400, 404 | handlers_scope.go:49 |
| DELETE | /scopes/{id} | yes | 204, 404 | handlers_scope.go:67 |
| POST | /scopes/{id}/projects | yes | 204, 400, 404 | handlers_scope.go:77 |
| DELETE | /scopes/{id}/projects/{pid} | yes | 204, 404 | handlers_scope.go:99 |
| POST | /scopes/{id}/search | yes | 200, 400, 504 | handlers_scope.go:111 |
| POST | /scopes/{id}/graph/search | yes | 200, 400, 504 | handlers_scope.go:151 |

### Knowledge Bases (8 endpoints)
| Method | Path | Auth | Status Codes | Handler File |
|--------|------|------|-------------|-------------|
| GET | /knowledge-bases | yes | 200, 500 | handlers_knowledgebase.go:12 |
| POST | /knowledge-bases | yes | 201, 400 | handlers_knowledgebase.go:22 |
| GET | /knowledge-bases/{id} | yes | 200, 404 | handlers_knowledgebase.go:17 |
| PUT | /knowledge-bases/{id} | yes | 200, 400, 404 | handlers_knowledgebase.go:27 |
| DELETE | /knowledge-bases/{id} | yes | 204, 404 | handlers_knowledgebase.go:32 |
| POST | /knowledge-bases/{id}/index | yes | 202, 404 | handlers_knowledgebase.go:37 |
| POST | /scopes/{id}/knowledge-bases | yes | 204, 400, 404 | handlers_knowledgebase.go:47 |
| DELETE | /scopes/{id}/knowledge-bases/{kbid} | yes | 204, 404 | handlers_knowledgebase.go:68 |
| GET | /scopes/{id}/knowledge-bases | yes | 200, 404 | handlers_knowledgebase.go:80 |

### Cost (7 endpoints)
| Method | Path | Auth | Status Codes | Handler File |
|--------|------|------|-------------|-------------|
| GET | /costs | yes | 200, 500 | handlers_cost.go:12 |
| GET | /projects/{id}/costs | yes | 200, 404 | handlers_cost.go:22 |
| GET | /projects/{id}/costs/by-model | yes | 200, 404 | handlers_cost.go:33 |
| GET | /projects/{id}/costs/by-tool | yes | 200, 404 | handlers_cost.go:68 |
| GET | /projects/{id}/costs/daily | yes | 200, 404 | handlers_cost.go:44 |
| GET | /projects/{id}/costs/runs | yes | 200, 404 | handlers_cost.go:56 |
| GET | /runs/{id}/costs/by-tool | yes | 200, 404 | handlers_cost.go:79 |

### Dashboard (7 endpoints)
| Method | Path | Auth | Status Codes | Handler File |
|--------|------|------|-------------|-------------|
| GET | /dashboard/stats | yes | 200, 500 | handlers_dashboard.go:12 |
| GET | /dashboard/charts/cost-trend | yes | 200, 500 | handlers_dashboard.go:86 |
| GET | /dashboard/charts/run-outcomes | yes | 200, 500 | handlers_dashboard.go:33 |
| GET | /dashboard/charts/agent-performance | yes | 200, 500 | handlers_dashboard.go:47 |
| GET | /dashboard/charts/model-usage | yes | 200, 500 | handlers_dashboard.go:60 |
| GET | /dashboard/charts/cost-by-project | yes | 200, 500 | handlers_dashboard.go:73 |
| GET | /projects/{id}/health | yes | 200, 404 | handlers_dashboard.go:22 |

### Roadmap (10 endpoints)
| Method | Path | Auth | Status Codes | Handler File |
|--------|------|------|-------------|-------------|
| GET | /projects/{id}/roadmap | yes | 200, 404 | handlers_roadmap.go:22 |
| POST | /projects/{id}/roadmap | yes | 201, 400, 404 | handlers_roadmap.go:33 |
| PUT | /projects/{id}/roadmap | yes | 200, 400, 404 | handlers_roadmap.go:56 |
| DELETE | /projects/{id}/roadmap | yes | 204, 404 | handlers_roadmap.go:96 |
| GET | /projects/{id}/roadmap/ai | yes | 200, 404 | handlers_roadmap.go:113 |
| POST | /projects/{id}/roadmap/detect | yes | 200, 404 | handlers_roadmap.go:129 |
| POST | /projects/{id}/roadmap/import | yes | 200, 404 | handlers_roadmap.go:319 |
| POST | /projects/{id}/roadmap/import/pm | yes | 200, 400, 404 | handlers_roadmap.go:331 |
| POST | /projects/{id}/roadmap/milestones | yes | 201, 400, 404 | handlers_roadmap.go:141 |
| POST | /projects/{id}/roadmap/sync | yes | 200, 400, 404 | handlers_settings.go:129 |
| POST | /projects/{id}/roadmap/sync-to-file | yes | 200, 404 | handlers_settings.go:359 |

### Milestones & Features (5 endpoints)
| Method | Path | Auth | Status Codes | Handler File |
|--------|------|------|-------------|-------------|
| GET | /milestones/{id} | yes | 200, 404 | handlers_roadmap.go:491 |
| PUT | /milestones/{id} | yes | 200, 400, 404 | handlers_roadmap.go:170 |
| DELETE | /milestones/{id} | yes | 204, 404 | handlers_roadmap.go:214 |
| POST | /milestones/{id}/features | yes | 201, 400, 404 | handlers_roadmap.go:224 |
| GET | /features/{id} | yes | 200, 404 | handlers_roadmap.go:502 |
| PUT | /features/{id} | yes | 200, 400, 404 | handlers_roadmap.go:247 |
| DELETE | /features/{id} | yes | 204, 404 | handlers_roadmap.go:307 |

### Auth (13 endpoints)
| Method | Path | Auth | Status Codes | Handler File |
|--------|------|------|-------------|-------------|
| POST | /auth/login | no | 200, 400, 401 | handlers_auth.go:29 |
| POST | /auth/refresh | no | 200, 401 | handlers_auth.go:58 |
| POST | /auth/logout | yes | 200, 401, 500 | handlers_auth.go:96 |
| GET | /auth/me | yes | 200, 401 | handlers_auth.go:158 |
| POST | /auth/change-password | yes | 200, 400, 401 | handlers_auth.go:137 |
| POST | /auth/api-keys | yes | 201, 400, 401 | handlers_auth.go:168 |
| GET | /auth/api-keys | yes | 200, 401, 500 | handlers_auth.go:190 |
| DELETE | /auth/api-keys/{id} | yes | 204, 401, 404 | handlers_auth.go:211 |
| GET | /auth/setup-status | no | 200, 500 | handlers_auth.go:284 |
| POST | /auth/setup | no | 201, 400, 409, 500 | handlers_auth.go:303 |
| POST | /auth/forgot-password | no | 200 | handlers_auth.go:372 |
| POST | /auth/reset-password | no | 200, 400, 404 | handlers_auth.go:402 |
| GET | /auth/github | no | 307, 501 | handlers_github_oauth.go:9 |
| GET | /auth/github/callback | no | 200, 400, 501 | handlers_github_oauth.go:26 |

### Subscription Providers (4 endpoints)
| Method | Path | Auth | Status Codes | Handler File |
|--------|------|------|-------------|-------------|
| GET | /auth/providers | yes | 200, 501 | handlers_subscription.go:10 |
| POST | /auth/providers/{provider}/connect | yes | 200, 400, 501 | handlers_subscription.go:21 |
| GET | /auth/providers/{provider}/status | yes | 200, 400, 501 | handlers_subscription.go:43 |
| DELETE | /auth/providers/{provider}/disconnect | yes | 200, 400, 501 | handlers_subscription.go:59 |

### Users (5 endpoints)
| Method | Path | Auth | Status Codes | Handler File |
|--------|------|------|-------------|-------------|
| GET | /users | admin | 200, 500 | handlers_auth.go:226 |
| POST | /users | admin | 201, 400 | handlers_auth.go:238 |
| PUT | /users/{id} | admin | 200, 400, 404 | handlers_auth.go:257 |
| DELETE | /users/{id} | admin | 204, 404 | handlers_auth.go:274 |
| POST | /users/{id}/force-password-change | admin | 200, 400, 404, 500 | handlers_auth.go:427 |

### Tenants (4 endpoints)
| Method | Path | Auth | Status Codes | Handler File |
|--------|------|------|-------------|-------------|
| GET | /tenants | admin | 200, 500 | handlers_settings.go:21 |
| POST | /tenants | admin | 201, 400 | handlers_settings.go:31 |
| GET | /tenants/{id} | admin | 200, 404 | handlers_settings.go:46 |
| PUT | /tenants/{id} | admin | 200, 400, 404 | handlers_settings.go:57 |

### Branch Protection (5 endpoints)
| Method | Path | Auth | Status Codes | Handler File |
|--------|------|------|-------------|-------------|
| POST | /projects/{id}/branch-rules | yes | 201, 400, 404 | handlers_session.go:28 |
| GET | /projects/{id}/branch-rules | yes | 200, 404 | handlers_session.go:17 |
| GET | /branch-rules/{id} | yes | 200, 404 | handlers_session.go:46 |
| PUT | /branch-rules/{id} | yes | 200, 400, 404 | handlers_session.go:57 |
| DELETE | /branch-rules/{id} | yes | 200, 404 | handlers_session.go:74 |

### Audit Trail (2 endpoints)
| Method | Path | Auth | Status Codes | Handler File |
|--------|------|------|-------------|-------------|
| GET | /audit | yes | 200, 500 | handlers_session.go:114 |
| GET | /projects/{id}/audit | yes | 200, 404 | handlers_session.go:130 |

### Review System (7 endpoints)
| Method | Path | Auth | Status Codes | Handler File |
|--------|------|------|-------------|-------------|
| GET | /projects/{id}/review-policies | yes | 200, 404 | handlers_settings.go:223 |
| POST | /projects/{id}/review-policies | yes | 201, 400, 404 | handlers_settings.go:234 |
| GET | /review-policies/{id} | yes | 200, 404 | handlers_settings.go:251 |
| PUT | /review-policies/{id} | yes | 200, 400, 404 | handlers_settings.go:262 |
| DELETE | /review-policies/{id} | yes | 204, 404 | handlers_settings.go:278 |
| POST | /review-policies/{id}/trigger | yes | 201, 404 | handlers_settings.go:288 |
| GET | /projects/{id}/reviews | yes | 200, 404 | handlers_settings.go:299 |
| GET | /reviews/{id} | yes | 200, 404 | handlers_settings.go:310 |

### Boundaries & Review-Refactor (3 endpoints)
| Method | Path | Auth | Status Codes | Handler File |
|--------|------|------|-------------|-------------|
| GET | /projects/{id}/boundaries | yes | 200, 404 | handlers_review.go:13 |
| PUT | /projects/{id}/boundaries | editor+ | 200, 400, 404 | handlers_review.go:24 |
| POST | /projects/{id}/boundaries/analyze | editor+ | 200, 500, 503 | handlers_review.go:39 |
| POST | /projects/{id}/review-refactor | editor+ | 200, 500, 503 | handlers_review.go:54 |

### MCP Servers (8 endpoints)
| Method | Path | Auth | Status Codes | Handler File |
|--------|------|------|-------------|-------------|
| GET | /mcp/servers | yes | 200, 500 | handlers_mcp.go:14 |
| POST | /mcp/servers | yes | 201, 400, 404 | handlers_mcp.go:35 |
| POST | /mcp/servers/test | yes | 200, 400, 404 | handlers_mcp.go:100 |
| GET | /mcp/servers/{id} | yes | 200, 404 | handlers_mcp.go:24 |
| PUT | /mcp/servers/{id} | yes | 200, 400, 404 | handlers_mcp.go:49 |
| DELETE | /mcp/servers/{id} | yes | 204, 404 | handlers_mcp.go:64 |
| POST | /mcp/servers/{id}/test | yes | 200, 404 | handlers_mcp.go:76 |
| GET | /mcp/servers/{id}/tools | yes | 200, 404 | handlers_mcp.go:114 |

### MCP Project Assignment (3 endpoints)
| Method | Path | Auth | Status Codes | Handler File |
|--------|------|------|-------------|-------------|
| GET | /projects/{id}/mcp-servers | yes | 200, 404 | handlers_mcp.go:125 |
| POST | /projects/{id}/mcp-servers | yes | 204, 400, 404 | handlers_mcp.go:141 |
| DELETE | /projects/{id}/mcp-servers/{serverId} | yes | 204, 404 | handlers_mcp.go:159 |

### Memory & Experience (4 endpoints)
| Method | Path | Auth | Status Codes | Handler File |
|--------|------|------|-------------|-------------|
| GET | /projects/{id}/memories | yes | 200, 500 | handlers_agent_features.go:266 |
| POST | /projects/{id}/memories | yes | 202, 400, 404 | handlers_agent_features.go:277 |
| POST | /projects/{id}/memories/recall | yes | 200, 400, 404 | handlers_agent_features.go:292 |
| GET | /projects/{id}/experience | yes | 200, 500 | handlers_agent_features.go:310 |
| DELETE | /experience/{id} | yes | 204, 500 | handlers_agent_features.go:321 |

### Microagents, Skills, Feedback (10 endpoints)
| Method | Path | Auth | Status Codes | Handler File |
|--------|------|------|-------------|-------------|
| GET | /projects/{id}/microagents | yes | 200, 500 | handlers_agent_features.go:333 |
| POST | /projects/{id}/microagents | yes | 201, 400 | handlers_agent_features.go:344 |
| GET | /microagents/{id} | yes | 200, 500 | handlers_agent_features.go:360 |
| PUT | /microagents/{id} | yes | 200, 400, 500 | handlers_agent_features.go:371 |
| DELETE | /microagents/{id} | yes | 204, 500 | handlers_agent_features.go:386 |
| GET | /projects/{id}/skills | yes | 200, 500 | handlers_agent_features.go:398 |
| POST | /projects/{id}/skills | yes | 201, 400 | handlers_agent_features.go:409 |
| GET | /skills/{id} | yes | 200, 500 | handlers_agent_features.go:425 |
| PUT | /skills/{id} | yes | 200, 400, 500 | handlers_agent_features.go:436 |
| DELETE | /skills/{id} | yes | 204, 500 | handlers_agent_features.go:451 |
| POST | /skills/import | editor+ | 201, 400, 422, 502, 500 | handlers_skill_import.go:29 |
| POST | /feedback/{run_id}/{call_id} | editor+ | 200, 400, 404 | handlers_agent_features.go:462 |
| GET | /runs/{id}/feedback | yes | 200, 500 | handlers_agent_features.go:490 |

### A2A (10 endpoints)
| Method | Path | Auth | Status Codes | Handler File |
|--------|------|------|-------------|-------------|
| POST | /a2a/agents | yes | 201, 400, 404 | handlers_a2a.go:24 |
| GET | /a2a/agents | yes | 200, 500 | handlers_a2a.go:42 |
| DELETE | /a2a/agents/{id} | yes | 204, 404 | handlers_a2a.go:52 |
| POST | /a2a/agents/{id}/discover | yes | 200, 404 | handlers_a2a.go:62 |
| POST | /a2a/agents/{id}/send | yes | 201, 400, 404 | handlers_a2a.go:73 |
| GET | /a2a/tasks | yes | 200, 500 | handlers_a2a.go:92 |
| GET | /a2a/tasks/{id} | yes | 200, 404 | handlers_a2a.go:106 |
| POST | /a2a/tasks/{id}/cancel | yes | 200, 404 | handlers_a2a.go:117 |
| POST | /a2a/tasks/{id}/push-config | yes | 201, 400, 404 | handlers_a2a.go:134 |
| GET | /a2a/tasks/{id}/push-config | yes | 200, 500 | handlers_a2a.go:153 |
| DELETE | /a2a/push-config/{id} | yes | 204, 404 | handlers_a2a.go:164 |
| GET | /a2a/tasks/{id}/subscribe | yes | 200 (SSE), 404, 500 | handlers_a2a.go:174 |

### Goals (6 endpoints)
| Method | Path | Auth | Status Codes | Handler File |
|--------|------|------|-------------|-------------|
| GET | /projects/{id}/goals | yes | 200, 500 | handlers_goals.go:18 |
| POST | /projects/{id}/goals | yes | 201, 400 | handlers_goals.go:29 |
| POST | /projects/{id}/goals/detect | yes | 200, 400, 500 | handlers_goals.go:44 |
| POST | /projects/{id}/goals/ai-discover | yes | 200, 400, 500, 503 | handlers_goals.go:104 |
| GET | /goals/{id} | yes | 200, 500 | handlers_goals.go:66 |
| PUT | /goals/{id} | yes | 200, 400 | handlers_goals.go:77 |
| DELETE | /goals/{id} | yes | 204, 500 | handlers_goals.go:92 |

### Routing (4 endpoints)
| Method | Path | Auth | Status Codes | Handler File |
|--------|------|------|-------------|-------------|
| GET | /routing/stats | yes | 200, 500 | handlers_routing.go:10 |
| POST | /routing/stats/refresh | yes | 200, 500 | handlers_routing.go:24 |
| GET | /routing/outcomes | yes | 200, 500 | handlers_routing.go:34 |
| POST | /routing/outcomes | yes | 201, 500 | handlers_routing.go:47 |
| POST | /routing/seed-from-benchmarks | yes | 200, 500 | handlers_routing.go:62 |

### Auto-Agent (3 endpoints)
| Method | Path | Auth | Status Codes | Handler File |
|--------|------|------|-------------|-------------|
| POST | /projects/{id}/auto-agent/start | yes | 200, 404, 503 | handlers_autoagent.go:10 |
| POST | /projects/{id}/auto-agent/stop | yes | 200, 404, 503 | handlers_autoagent.go:29 |
| GET | /projects/{id}/auto-agent/status | yes | 200, 404, 503 | handlers_autoagent.go:47 |

### Benchmarks (14 endpoints, dev-mode only)
| Method | Path | Auth | Status Codes | Handler File |
|--------|------|------|-------------|-------------|
| GET | /benchmarks/suites | dev | 200, 500 | handlers_benchmark.go:17 |
| POST | /benchmarks/suites | dev | 201, 400 | handlers_benchmark.go:22 |
| GET | /benchmarks/suites/{id} | dev | 200, 404 | handlers_benchmark.go:36 |
| PUT | /benchmarks/suites/{id} | dev | 200, 400, 404 | handlers_benchmark.go:197 |
| DELETE | /benchmarks/suites/{id} | dev | 204, 404 | handlers_benchmark.go:41 |
| GET | /benchmarks/runs | dev | 200, 500 | handlers_benchmark.go:48 |
| POST | /benchmarks/runs | dev | 201, 400 | handlers_benchmark.go:81 |
| GET | /benchmarks/runs/{id} | dev | 200, 404 | handlers_benchmark.go:75 |
| PATCH | /benchmarks/runs/{id} | dev | 200, 400, 404 | handlers_benchmark.go:101 |
| DELETE | /benchmarks/runs/{id} | dev | 204, 404 | handlers_benchmark.go:95 |
| GET | /benchmarks/runs/{id}/results | dev | 200, 404 | handlers_benchmark.go:127 |
| GET | /benchmarks/runs/{id}/export/results | dev | 200, 400, 404 | handlers_benchmark.go:161 |
| GET | /benchmarks/runs/{id}/export/training | dev | 200, 400, 404 | handlers_benchmark.go:315 |
| GET | /benchmarks/runs/{id}/export/rlvr | dev | 200, 400 | handlers_benchmark.go:287 |
| GET | /benchmarks/runs/{id}/cost-analysis | dev | 200, 400, 404 | handlers_benchmark.go:259 |
| POST | /benchmarks/compare | dev | 200, 400, 404 | handlers_benchmark.go:132 |
| POST | /benchmarks/compare-multi | dev | 200, 400, 404 | handlers_benchmark.go:241 |
| GET | /benchmarks/leaderboard | dev | 200, 404 | handlers_benchmark.go:274 |
| GET | /benchmarks/datasets | dev | 200, 500 | handlers_benchmark.go:150 |
| POST | /benchmarks/runs/{id}/analyze | dev | 200, 400, 404, 500 | handlers_benchmark_analyze.go:11 |

### Channels (8 endpoints)
| Method | Path | Auth | Status Codes | Handler File |
|--------|------|------|-------------|-------------|
| GET | /channels | yes | 200, 404 | handlers_channel.go:12 |
| POST | /channels | yes | 201, 400 | handlers_channel.go:23 |
| GET | /channels/{id} | yes | 200, 404 | handlers_channel.go:37 |
| DELETE | /channels/{id} | yes | 204, 404 | handlers_channel.go:48 |
| GET | /channels/{id}/messages | yes | 200, 404 | handlers_channel.go:58 |
| POST | /channels/{id}/messages | yes | 201, 400 | handlers_channel.go:72 |
| POST | /channels/{id}/messages/{mid}/thread | yes | 201, 400 | handlers_channel.go:89 |
| PUT | /channels/{id}/members/{uid} | yes | 200, 400, 404 | handlers_channel.go:109 |
| POST | /channels/{id}/webhook | yes | 201, 400, 401 | handlers_channel.go:130 |

### Quarantine (5 endpoints)
| Method | Path | Auth | Status Codes | Handler File |
|--------|------|------|-------------|-------------|
| GET | /quarantine | admin | 200, 400, 500, 503 | handlers_quarantine.go:13 |
| GET | /quarantine/stats | admin | 200, 400, 500, 503 | handlers_quarantine.go:99 |
| GET | /quarantine/{id} | admin | 200, 404, 503 | handlers_quarantine.go:38 |
| POST | /quarantine/{id}/approve | admin | 200, 400, 404, 503 | handlers_quarantine.go:54 |
| POST | /quarantine/{id}/reject | admin | 200, 400, 404, 503 | handlers_quarantine.go:77 |

### Remaining (misc)
| Method | Path | Auth | Status Codes | Handler File |
|--------|------|------|-------------|-------------|
| GET | / (version) | yes | 200 | routes.go:40 |
| POST | /parse-repo-url | yes | 200, 400 | handlers.go:177 |
| GET | /repos/info | yes | 200, 400, 502 | handlers.go:198 |
| GET | /providers/git | yes | 200 | handlers.go:757 |
| GET | /providers/agent | yes | 200 | handlers.go:764 |
| GET | /providers/spec | yes | 200 | handlers_roadmap.go:370 |
| GET | /providers/pm | yes | 200 | handlers_roadmap.go:392 |
| GET | /backends/health | yes | 200, 503, 504 | handlers_backend_health.go:11 |
| GET | /settings | yes | 200, 500 | handlers_settings.go:323 |
| PUT | /settings | admin | 200, 400, 500 | handlers_settings.go:339 |
| GET | /vcs-accounts | yes | 200, 500 | handlers_settings.go:358 |
| POST | /vcs-accounts | yes | 201, 400 | handlers_settings.go:368 |
| DELETE | /vcs-accounts/{id} | yes | 204, 404 | handlers_settings.go:384 |
| POST | /vcs-accounts/{id}/test | yes | 200, 404 | handlers_settings.go:394 |
| GET | /commands | yes | 200 | handlers_commands.go:9 |
| GET | /prompt-sections | yes | 200, 404 | handlers_prompt_section.go:14 |
| PUT | /prompt-sections | yes | 200, 400, 404 | handlers_prompt_section.go:28 |
| DELETE | /prompt-sections/{id} | yes | 204, 404 | handlers_prompt_section.go:51 |
| POST | /prompt-sections/preview | yes | 200, 400 | handlers_prompt_section.go:74 |
| POST | /dev/benchmark | dev | 200, 400, 403, 502 | handlers_agent_features.go:25 |
| GET | /projects/{id}/active-work | yes | 200, 404 | handlers.go:1041 |
| GET | /prompt-evolution/status | yes | 200, 503 | handlers_prompt_evolution.go:12 |
| POST | /prompt-evolution/revert/{modeId} | editor+ | 200, 400, 500, 503 | handlers_prompt_evolution.go:23 |
| POST | /prompt-evolution/promote/{variantId} | editor+ | 200, 400, 404, 503 | handlers_prompt_evolution.go:45 |

### Webhooks (outside auth)
| Method | Path | Auth | Status Codes | Handler File |
|--------|------|------|-------------|-------------|
| POST | /webhooks/vcs/github | HMAC | 200, 400, 404 | handlers_settings.go:76 |
| POST | /webhooks/vcs/gitlab | token | 200, 400, 404 | handlers_settings.go:105 |
| POST | /webhooks/pm/github | HMAC | 200, 400, 404 | handlers_settings.go:161 |
| POST | /webhooks/pm/gitlab | token | 200, 400, 404 | handlers_settings.go:183 |
| POST | /webhooks/pm/plane | HMAC | 200, 400, 404 | handlers_settings.go:205 |

**Total: ~230+ endpoints across 37 handler files**

---

## Architecture Review

### URL Pattern Consistency
The API follows a generally consistent pattern: `/api/v1/{resource}` for top-level resources and `/api/v1/projects/{id}/{sub-resource}` for nested resources. Plural nouns are used consistently. There are a few deviations documented in the findings below.

### Error Response Consistency
A single `errorResponse{Error string}` struct is used across almost all handlers. The `writeDomainError()` function provides centralized mapping from domain errors to HTTP status codes. Two notable exceptions break this pattern.

### Pagination
The API uses **cursor-based pagination** for trajectory, audit trail, and channel messages. Other list endpoints (projects, agents, tasks, conversations, etc.) have **no pagination at all** -- they return all records.

### JSON Field Naming
JSON tags use **snake_case** consistently across all request/response structs (e.g., `project_id`, `task_id`, `run_id`, `call_id`, `model_name`).

---

## Code Review Findings

### CRITICAL-001: Internal Error Message Leakage in Search Endpoints -- **FIXED**
- **File:** `internal/adapter/http/handlers_search.go:43,101`
- **Description:** The `SearchConversations` and `GlobalSearch` handlers expose internal error messages to clients by concatenating `err.Error()` into the response: `"search failed: " + err.Error()`. This can leak database connection strings, table names, or other internal details.
- **Impact:** Information disclosure vulnerability. Internal errors may reveal PostgreSQL query details, table names, or other system internals to external callers.
- **Recommendation:** Replace with `writeInternalError(w, err)` which logs the error server-side and returns a generic "internal server error" message. Or use `writeError(w, http.StatusInternalServerError, "search failed")` without the error details.

### HIGH-001: Mixed PathValue vs URLParam for Route Parameter Extraction -- **FIXED**
- **File:** `internal/adapter/http/handlers_benchmark.go:102,162,198,260,288,316` and `handlers_benchmark_analyze.go:12`
- **Description:** Benchmark handlers use `r.PathValue("id")` (Go 1.22+ net/http), while all other handlers use `chi.URLParam(r, "id")`. When using chi router, `PathValue` may not be populated because chi uses its own context key for URL parameters. This means these handlers may receive an empty `id` when routed through chi.
- **Impact:** All benchmark `PathValue("id")` calls could silently return empty strings, causing "run id is required" errors for valid requests, or worse, operating on empty IDs. The `handleGet`/`handleDelete` calls from crud.go use `chi.URLParam` correctly, masking this for some endpoints. But `CancelBenchmarkRun`, `ExportBenchmarkResults`, `UpdateBenchmarkSuite`, `BenchmarkCostAnalysis`, `ExportRLVRData`, `ExportTrainingData`, and `AnalyzeBenchmarkRun` are all affected.
- **Recommendation:** Replace all `r.PathValue("id")` with `chi.URLParam(r, "id")` (or the local `urlParam(r, "id")` alias) for consistency and correctness with the chi router.

### HIGH-002: Inconsistent Error Response Format in LLM Keys Handler -- **FIXED**
- **File:** `internal/adapter/http/handlers_llm_keys.go:16,32,53`
- **Description:** Three handlers use `http.Error(w, "unauthorized", http.StatusUnauthorized)` instead of `writeError(w, http.StatusUnauthorized, "not authenticated")`. `http.Error()` sets Content-Type to `text/plain` and writes a plain text body, while all other handlers return `application/json` with `{"error": "..."}`. This breaks the API contract for any client expecting consistent JSON error responses.
- **Impact:** Clients parsing JSON error responses will get parse errors when these endpoints return plain text. The inconsistent Content-Type header (`text/plain` vs `application/json`) can confuse HTTP client libraries.
- **Recommendation:** Replace all three `http.Error()` calls with `writeError(w, http.StatusUnauthorized, "not authenticated")` to match the rest of the API.

### HIGH-003: Inconsistent Delete Response Pattern -- **FIXED**
- **File:** `internal/adapter/http/handlers_session.go:80` (DeleteBranchProtectionRule)
- **Description:** `DeleteBranchProtectionRule` returns `200 {"status":"deleted"}` while every other delete handler returns `204 No Content`. This is the only delete endpoint that returns a JSON body.
- **Impact:** Clients implementing generic delete logic will fail or behave unexpectedly on this one endpoint. The 200+body pattern violates the established convention.
- **Recommendation:** Change to `w.WriteHeader(http.StatusNoContent)` with no response body, matching all other delete handlers.

### HIGH-004: No Pagination on Most List Endpoints -- **FIXED**
- **File:** Multiple (handlers.go, handlers_conversation.go, handlers_agent_features.go, etc.)
- **Description:** Approximately 40+ list endpoints return all records without pagination. Only trajectory (`GetTrajectory`), audit trail (`GlobalAuditTrail`, `ProjectAuditTrail`), and channel messages (`ListChannelMessages`) implement cursor-based pagination with `cursor` and `limit` parameters. All other list endpoints (projects, agents, tasks, conversations, runs, modes, skills, microagents, etc.) return unbounded result sets.
- **Impact:** As the system grows, these endpoints will cause performance degradation, memory pressure, and potentially timeout errors. A project with thousands of conversations, for example, will return all of them in a single response.
- **Recommendation:** Implement cursor-based or offset pagination consistently across all list endpoints. At minimum, add `limit` with a sensible default (e.g., 50) and `offset`/`cursor` parameters.

### MEDIUM-001: Verb-in-URL Violations -- **FIXED**
- **File:** `internal/adapter/http/routes.go` (multiple lines)
- **Description:** Several endpoints use verbs in URLs, violating RESTful naming conventions:
  - `POST /llm/models/delete` -- should be `DELETE /llm/models/{id}`
  - `POST /projects/{id}/detect-stack` and `POST /detect-stack` -- verb "detect" in URL
  - `POST /projects/{id}/decompose` -- verb "decompose" in URL
  - `POST /projects/{id}/plan-feature` -- verb "plan" in URL
  - `POST /parse-repo-url` -- verb "parse" in URL
  - `POST /routing/seed-from-benchmarks` -- verb "seed" in URL
- **Impact:** Inconsistency with RESTful conventions. `POST /llm/models/delete` is particularly problematic since it uses POST instead of DELETE and has a verb in the URL.
- **Recommendation:** For `POST /llm/models/delete`, consider proxying through a proper `DELETE /llm/models/{id}` endpoint. For action endpoints like decompose/detect, these are acceptable as RPC-style actions under REST (they are not CRUD operations), but should be documented as such.

### MEDIUM-002: Duplicate Query Parameter Parser Functions -- **FIXED**
- **File:** `internal/adapter/http/helpers.go:42` and `internal/adapter/http/handlers_quarantine.go:137`
- **Description:** Two nearly identical functions exist for parsing integer query parameters: `queryParamInt()` in helpers.go (enforces `n > 0`) and `queryInt()` in handlers_quarantine.go (allows zero/negative values). This creates maintenance burden and subtle behavior differences.
- **Impact:** Inconsistent handling of edge cases. `queryParamInt` rejects zero values (returns default), while `queryInt` allows them. Quarantine's offset=0 works correctly only because it uses `queryInt`.
- **Recommendation:** Remove `queryInt` from handlers_quarantine.go. Modify `queryParamInt` to accept a `minValue` parameter, or create a `queryParamIntRange(min, max)` variant that handles the offset=0 case.

### MEDIUM-003: Missing Pagination Envelope Consistency -- **FIXED**
- **File:** Multiple handler files
- **Description:** List responses use three different formats: (1) bare arrays `[...]` (most endpoints), (2) paginated objects `{"events":[], "cursor":"...", "has_more":true, "total":N}` (trajectory), (3) wrapped objects `{"results":[], "count":N}` (scope search). There is no consistent response envelope.
- **Impact:** Clients cannot implement generic list handling. Adding pagination later requires a breaking change for bare-array responses.
- **Recommendation:** Define a standard list envelope: `{"data": [], "total": N, "cursor": "", "has_more": false}`. Apply it consistently to all list endpoints. Bare arrays are acceptable for small, bounded enumerations (providers, modes).

### MEDIUM-004: No 422 Unprocessable Entity Usage for Validation Errors -- **FIXED**
- **File:** `internal/adapter/http/helpers.go:104` (writeError for validation)
- **Description:** All validation errors return `400 Bad Request`. The API does not use `422 Unprocessable Entity` for semantic validation failures (e.g., valid JSON but invalid field values). The only 422 usage is in `handlers_skill_import.go:50` for safety rejection.
- **Impact:** Clients cannot distinguish between malformed requests (true 400) and semantically invalid requests (should be 422). RFC 4918 defines 422 specifically for this purpose.
- **Recommendation:** Use 400 for malformed JSON / missing fields, and 422 for business rule validation failures (invalid enum values, constraint violations, etc.).

### MEDIUM-005: Error Detail Leakage in EvaluateStep -- **FIXED**
- **File:** `internal/adapter/http/handlers_orchestration.go:119`
- **Description:** The `EvaluateStep` handler exposes internal error details: `fmt.Sprintf("review evaluation failed: %v", err)`. This can leak internal service details.
- **Impact:** Internal error messages from the review router service could be exposed to API clients.
- **Recommendation:** Use `writeInternalError(w, err)` to log the error server-side and return a generic message.

### LOW-001: Quarantine Handlers Use Lowercase Method Names
- **File:** `internal/adapter/http/handlers_quarantine.go:13,38,54,77,99`
- **Description:** Quarantine handlers use unexported (lowercase) method names: `listQuarantinedMessages`, `getQuarantinedMessage`, `approveQuarantinedMessage`, `rejectQuarantinedMessage`, `quarantineStats`. All other handlers use exported (uppercase) names like `ListProjects`, `GetProject`, etc.
- **Impact:** Minor inconsistency. The handlers work correctly because they are referenced by method value in routes.go. However, this diverges from the codebase convention and makes code navigation less predictable.
- **Recommendation:** Rename to `ListQuarantinedMessages`, `GetQuarantinedMessage`, etc. for consistency.

### LOW-002: Batch Operations Use POST for DELETE Semantics
- **File:** `internal/adapter/http/routes.go:51` and `internal/adapter/http/handlers_batch.go:32`
- **Description:** `POST /projects/batch/delete` uses POST for a destructive delete operation. While this is a pragmatic choice (DELETE requests with JSON bodies are not universally supported), it diverges from RESTful conventions.
- **Impact:** Minor deviation from REST principles. The POST method is acceptable for batch operations where the body contains the list of IDs, but it should be documented.
- **Recommendation:** This is an acceptable pattern for batch operations. Document it explicitly in API docs as a batch RPC endpoint.

### LOW-003: Inconsistent Status Response Messages
- **File:** Multiple handler files
- **Description:** Ad-hoc status response messages vary across handlers: `"ok"`, `"cancelled"`, `"canceled"` (note different spelling in A2A), `"dispatched"`, `"stopped"`, `"updated"`, `"read"`, `"generating"`, `"building"`, `"bypassed"`, etc. The `CancelRun` handler returns `"cancelled"` (British), while `CancelA2ATask` returns `"canceled"` (American).
- **Impact:** Clients that match on status strings may break. The spelling inconsistency (`cancelled` vs `canceled`) is particularly problematic.
- **Recommendation:** Standardize on one spelling (prefer `"cancelled"` to match Go's convention) and define an enum of valid status values.

### LOW-004: Missing PATCH Usage for Partial Updates
- **File:** `internal/adapter/http/routes.go` (most PUT endpoints)
- **Description:** The API uses `PUT` for partial updates (e.g., `UpdateProjectRoadmap` only updates non-empty fields), but `PUT` semantically means full replacement. The only PATCH usage is for file rename and benchmark run cancellation. Partial update endpoints should use PATCH.
- **Impact:** Clients following HTTP semantics strictly would send full resource representations for PUT requests, potentially overwriting fields with empty values.
- **Recommendation:** For endpoints that accept partial updates (roadmap, milestones, features), consider switching to PATCH or documenting that PUT accepts partial bodies.

---

## Documentation vs Reality

### Documented but Missing from Code
- No endpoints found for `SyncConfig` direction handling -- the `SyncRoadmap` handler exists but the bidirectional sync mechanisms are partially implemented (delegates to service layer).

### In Code but Not Explicitly Documented in Feature Docs
The following endpoints exist in code but are not mentioned in any `docs/features/*.md`:
- `/prompt-evolution/*` endpoints (3 endpoints)
- `/routing/*` endpoints (5 endpoints)
- `/a2a/push-config/{id}` DELETE endpoint (orphaned from task context)
- `/dev/benchmark` endpoint
- `/projects/{id}/active-work` endpoint

### Documentation Alignment
- `docs/features/05-chat-enhancements.md` documents the channel endpoints (9 endpoints) -- **matches code**
- `docs/features/06-visual-design-canvas.md` documents message images -- **no dedicated API endpoint, handled via existing message flow**
- CLAUDE.md documents `POST /policies/allow-always` -- **matches code**
- CLAUDE.md documents `POST /conversations/{id}/bypass-approvals` -- **matches code**
- CLAUDE.md documents `POST /projects/{id}/adopt` -- **matches code**

---

## Summary & Recommendations

### Priority 1 (Fix Immediately)
1. **CRITICAL-001:** Remove internal error messages from search endpoint responses
2. **HIGH-001:** Replace all `r.PathValue("id")` with `chi.URLParam(r, "id")` in benchmark handlers (7 occurrences)
3. **HIGH-002:** Replace `http.Error()` with `writeError()` in LLM keys handlers (3 occurrences)

### Priority 2 (Fix Soon)
4. **HIGH-003:** Standardize `DeleteBranchProtectionRule` to return 204 No Content
5. **MEDIUM-001:** Replace `POST /llm/models/delete` with proper `DELETE /llm/models/{id}` proxy
6. **MEDIUM-002:** Consolidate duplicate query parameter parser functions
7. **MEDIUM-005:** Remove internal error leakage in `EvaluateStep`

### Priority 3 (Plan for Next Release)
8. **HIGH-004:** Implement pagination across all list endpoints
9. **MEDIUM-003:** Define a standard list response envelope
10. **MEDIUM-004:** Introduce 422 status code for semantic validation errors
11. **LOW-003:** Standardize status message spelling (`cancelled` vs `canceled`)

### Priority 4 (Nice to Have)
12. **LOW-001:** Rename quarantine handlers to exported names
13. **LOW-002:** Document batch POST-for-delete pattern
14. **LOW-004:** Consider PATCH for partial update endpoints

### Scoring Breakdown
| Finding | Severity | Deduction |
|---------|----------|-----------|
| CRITICAL-001 | CRITICAL | -15 |
| HIGH-001 | HIGH | -5 |
| HIGH-002 | HIGH | -5 |
| HIGH-003 | HIGH | -5 |
| HIGH-004 | HIGH | -5 |
| MEDIUM-001 | MEDIUM | -2 |
| MEDIUM-002 | MEDIUM | -2 |
| MEDIUM-003 | MEDIUM | -2 |
| MEDIUM-004 | MEDIUM | -2 |
| MEDIUM-005 | MEDIUM | -2 |
| LOW-001 | LOW | -1 |
| LOW-002 | LOW | -1 |
| LOW-003 | LOW | -1 |
| LOW-004 | LOW | -1 |
| | Subtotal deductions | -49 |
| | Positive bonus (strong foundations) | +21 |
| | **Final Score** | **72/100** |

Bonus points awarded for: consistent error struct (+3), generic CRUD factories (+3), null-safe lists (+2), proper HTTP semantics for most CRUD (+3), body size limiting (+2), security headers (+2), role-based access control (+3), snake_case consistency (+3).

---

## Fix Status

| Severity | Total | Fixed | Unfixed |
|----------|------:|------:|--------:|
| CRITICAL | 1     | 1     | 0       |
| HIGH     | 4     | 4     | 0       |
| MEDIUM   | 5     | 5     | 0       |
| LOW      | 4     | 0     | 4       |
| **Total**| **14**| **10**| **4**   |

**Post-fix score:** 100 - (0 CRITICAL x 15) - (0 HIGH x 5) - (0 MEDIUM x 2) - (4 LOW x 1) = **96/100 -- Grade: A**

**Remaining unfixed findings:**
- LOW-001: Quarantine handlers use lowercase method names
- LOW-002: Batch operations use POST for DELETE semantics
- LOW-003: Inconsistent status message spelling (cancelled/canceled)
- LOW-004: Missing PATCH usage for partial updates
