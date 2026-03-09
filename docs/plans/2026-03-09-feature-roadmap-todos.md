# Feature Roadmap — Atomic Agent TODOs

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 8 feature areas broken into atomic, independently executable tasks for AI agents.

**Architecture:** Hexagonal (Ports & Adapters) with self-registering providers via `init()`. Go Core + Python Workers via NATS. SolidJS frontend.

**Tech Stack:** Go 1.25 (chi, pgx, coder/websocket), Python 3.12 (Poetry, Pydantic), TypeScript (SolidJS, Tailwind)

---

## Feature 1: GitHub OAuth Adapter

> Currently only Copilot token exchange exists (`internal/adapter/copilot/`). The `gitlocal` provider registers itself as "github" but only wraps local `git` CLI. This feature adds a real GitHub REST API adapter with OAuth App flow for repo listing, PR creation, and authenticated cloning.

### 1.1 — GitHub OAuth domain model

**Files:**
- Create: `internal/domain/vcsaccount/oauth.go`

**What to do:**
- Add `OAuthState` struct: `State string`, `Provider string`, `RedirectURL string`, `CreatedAt time.Time`, `ExpiresAt time.Time`
- Add `OAuthToken` struct: `AccessToken string`, `RefreshToken string`, `TokenType string`, `Scope string`, `ExpiresAt time.Time`
- Add `func NewOAuthState(provider, redirectURL string) *OAuthState` — generates crypto-random state, 10min expiry

**Test:** `internal/domain/vcsaccount/oauth_test.go`
- Test state generation produces 32-char hex string
- Test expiry is ~10 minutes from now
- Test NewOAuthState sets provider and redirect correctly

**Run:** `go test ./internal/domain/vcsaccount/ -v -run TestOAuth`

---

### 1.2 — OAuth state store methods

**Files:**
- Modify: `internal/port/database/store.go` — add 3 methods to `Store` interface
- Modify: `internal/adapter/postgres/store.go` — implement methods
- Create: `internal/adapter/postgres/migrations/070_oauth_state.sql`

**What to do:**
- Add to `Store` interface:
  ```go
  StoreOAuthState(ctx context.Context, state *vcsaccount.OAuthState) error
  GetOAuthState(ctx context.Context, stateStr string) (*vcsaccount.OAuthState, error)
  DeleteOAuthState(ctx context.Context, stateStr string) error
  ```
- Migration: `CREATE TABLE IF NOT EXISTS oauth_states (state TEXT PRIMARY KEY, tenant_id TEXT NOT NULL, provider TEXT NOT NULL, redirect_url TEXT NOT NULL, created_at TIMESTAMPTZ DEFAULT now(), expires_at TIMESTAMPTZ NOT NULL)`
- Implementation must include `AND tenant_id = $N` in queries
- `GetOAuthState` must check `expires_at > now()`, return error if expired

**Test:** `internal/adapter/postgres/store_oauth_test.go` (integration test with `//go:build integration`)
- Store then Get roundtrip
- Get expired state returns error
- Get non-existent state returns error
- Delete removes state

**Run:** `go test -tags=integration ./internal/adapter/postgres/ -v -run TestOAuthState`

---

### 1.3 — GitHub OAuth service

**Files:**
- Create: `internal/service/github_oauth.go`

**What to do:**
- `GitHubOAuthService` struct with fields: `store database.Store`, `clientID string`, `clientSecret string`, `callbackURL string`
- `NewGitHubOAuthService(store, clientID, clientSecret, callbackURL)` — reads from config/env
- `AuthorizeURL(ctx) (url string, state string, err error)` — stores state in DB, returns `https://github.com/login/oauth/authorize?client_id=...&state=...&scope=repo,read:org`
- `HandleCallback(ctx, code, state string) (*vcsaccount.VCSAccount, error)` — validates state from DB, exchanges code for token via `POST https://github.com/login/oauth/access_token`, creates VCSAccount with encrypted token, deletes state
- Token exchange uses `net/http` client with 10s timeout, `Accept: application/json` header

**Test:** `internal/service/github_oauth_test.go`
- Test AuthorizeURL returns valid URL with client_id and state params
- Test HandleCallback with invalid state returns error
- Test HandleCallback with expired state returns error
- Test HandleCallback success creates VCSAccount (mock HTTP server for token exchange)
- Test HandleCallback encrypts token before storage

**Run:** `go test ./internal/service/ -v -run TestGitHubOAuth`

---

### 1.4 — GitHub OAuth HTTP handlers

**Files:**
- Create: `internal/adapter/http/handlers_github_oauth.go`
- Modify: `internal/adapter/http/routes.go` — add 2 routes

**What to do:**
- `StartGitHubOAuth(w, r)` — GET `/api/v1/auth/github` — calls service.AuthorizeURL, returns JSON `{ "url": "...", "state": "..." }`
- `GitHubOAuthCallback(w, r)` — GET `/api/v1/auth/github/callback` — reads `code` + `state` from query params, calls service.HandleCallback, returns created VCSAccount as JSON 201
- Routes: add inside authenticated router group in `routes.go`

**Test:** `internal/adapter/http/handlers_github_oauth_test.go`
- Test StartGitHubOAuth returns 200 with url field
- Test Callback with missing code returns 400
- Test Callback with invalid state returns 400
- Test Callback success returns 201 with VCSAccount

**Run:** `go test ./internal/adapter/http/ -v -run TestGitHubOAuth`

---

### 1.5 — GitHub API git provider

**Files:**
- Create: `internal/adapter/github/provider.go`
- Create: `internal/adapter/github/register.go`

**What to do:**
- `Provider` struct with `token string`, `baseURL string` (default `https://api.github.com`)
- Implements `gitprovider.Provider` interface
- `Name()` returns `"github-api"`
- `Capabilities()` returns `Clone: true, Push: true, PullRequest: true, Webhook: true, Issues: true`
- `ListRepos(ctx)` — GET `/user/repos?per_page=100&sort=updated`, paginate via Link header, return repo full_names
- `CloneURL(ctx, repo)` — returns `https://x-access-token:{token}@github.com/{repo}.git`
- `Clone(ctx, url, dest)` — delegates to `git clone` via `os/exec` (same pattern as gitlocal)
- `Status/Pull/ListBranches/Checkout` — delegate to local git CLI (same as gitlocal, repos are local after clone)
- `register.go`: `init()` registers `"github-api"` with factory reading `token` and `base_url` from config map
- Import in `cmd/codeforge/main.go`

**Test:** `internal/adapter/github/provider_test.go`
- Test Name returns "github-api"
- Test Capabilities returns all true
- Test CloneURL formats token into URL
- Test ListRepos parses JSON response (httptest.Server)
- Test ListRepos handles pagination
- Test CloneURL with custom baseURL

**Run:** `go test ./internal/adapter/github/ -v`

---

### 1.6 — Frontend OAuth connect button

**Files:**
- Modify: `frontend/src/api/client.ts` — add `auth.githubOAuth()` method
- Modify: `frontend/src/features/settings/SettingsPage.tsx` — add "Connect GitHub" button in VCS accounts section

**What to do:**
- API client: `auth: { githubOAuth: () => request<{ url: string }>("/auth/github") }`
- Settings page: Add a "Connect GitHub" button next to the VCS account form
- On click: call `api.auth.githubOAuth()`, then `window.location.href = response.url`
- After OAuth redirect back, the callback handler creates the VCSAccount — the settings page list refreshes automatically

**Test:** Manual browser test — verify button appears, click initiates OAuth flow

---

### 1.7 — Update docs and todo

**Files:**
- Modify: `docs/todo.md` — mark GitHub OAuth task `[x]`
- Modify: `docs/features/01-project-dashboard.md` — add GitHub OAuth section
- Modify: `docs/dev-setup.md` — add `GITHUB_CLIENT_ID` and `GITHUB_CLIENT_SECRET` env vars

**Commit:** `feat(github): add GitHub OAuth adapter with API-based repo listing`

---

## Feature 2: Forgejo/Codeberg Compatibility

> The Gitea PM adapter (`internal/adapter/gitea/`) uses REST API. Forgejo/Codeberg are Gitea forks with mostly compatible APIs. Need to verify and fix any differences.

### 2.1 — Gitea adapter base_url test coverage

**Files:**
- Read: `internal/adapter/gitea/provider.go`
- Create or modify: `internal/adapter/gitea/provider_test.go`

**What to do:**
- Write tests that verify the Gitea adapter correctly uses `base_url` from config for all API calls
- Test with `base_url = "https://codeberg.org"` and `base_url = "https://forgejo.example.com"`
- Test that API paths are appended correctly (no double slashes, trailing slash handling)
- Test ListItems, GetItem, CreateItem, UpdateItem all use base_url

**Run:** `go test ./internal/adapter/gitea/ -v`

---

### 2.2 — Forgejo API compatibility layer

**Files:**
- Modify: `internal/adapter/gitea/provider.go` — handle Forgejo-specific API differences

**What to do:**
- Forgejo has `/api/forgejo/v1/version` endpoint — add `detectForgejo(ctx) bool` method
- Some Forgejo responses use `full_name` instead of `name` in certain endpoints — add fallback parsing
- Codeberg enforces stricter rate limits — respect `X-RateLimit-Remaining` header and add backoff
- Add `provider_variant` field to config: `"gitea"` (default), `"forgejo"`, `"codeberg"`

**Test:** `internal/adapter/gitea/provider_test.go`
- Test Forgejo detection via version endpoint
- Test field name fallback (full_name vs name)
- Test rate limit header parsing
- Test each variant string is accepted

**Run:** `go test ./internal/adapter/gitea/ -v -run TestForgejo`

---

### 2.3 — Register forgejo/codeberg as provider aliases

**Files:**
- Modify: `internal/adapter/gitea/register.go`

**What to do:**
- Register the Gitea factory under 3 names: `"gitea"`, `"forgejo"`, `"codeberg"`
- Factory reads `variant` from config map to set provider_variant
- Update `validProviders` in `internal/service/vcsaccount.go` to include `"forgejo"` and `"codeberg"`

**Test:**
- Verify `gitprovider.Available()` includes all 3 names
- Verify creating VCSAccount with provider="forgejo" succeeds

**Run:** `go test ./internal/adapter/gitea/ -v && go test ./internal/service/ -v -run TestVCSAccount`

---

### 2.4 — Frontend provider dropdown update

**Files:**
- Modify: `frontend/src/api/types.ts` — extend `VCSProvider` type
- Modify: `frontend/src/features/settings/SettingsPage.tsx` — update provider selector

**What to do:**
- Add `"forgejo" | "codeberg"` to `VCSProvider` union type
- Settings page provider dropdown: add "Forgejo" and "Codeberg" options
- When "Codeberg" selected, auto-fill server_url with `https://codeberg.org`

**Commit:** `feat(gitea): add Forgejo/Codeberg compatibility with provider aliases`

---

## Feature 3: Batch Operations

> No batch operation framework exists. All project operations are single-item. This adds multi-select UI and batch API endpoints.

### 3.1 — Batch project API endpoints (Go)

**Files:**
- Create: `internal/adapter/http/handlers_batch.go`
- Modify: `internal/adapter/http/routes.go` — add batch routes
- Modify: `internal/service/project.go` — add batch methods

**What to do:**
- Service methods:
  ```go
  func (s *ProjectService) BatchDelete(ctx context.Context, ids []string) (deleted int, errors []BatchError, err error)
  func (s *ProjectService) BatchPull(ctx context.Context, ids []string) (results []BatchResult, err error)
  func (s *ProjectService) BatchStatus(ctx context.Context, ids []string) (statuses []BatchStatus, err error)
  ```
- `BatchError` struct: `ID string`, `Error string`
- `BatchResult` struct: `ID string`, `Status string`, `Error string`
- `BatchStatus` struct: `ID string`, `GitStatus *project.GitStatus`, `Error string`
- Handler: `BatchDeleteProjects` — POST `/api/v1/projects/batch/delete` — body: `{ "ids": ["..."] }`, max 50 IDs
- Handler: `BatchPullProjects` — POST `/api/v1/projects/batch/pull` — body: `{ "ids": ["..."] }`
- Handler: `BatchStatusProjects` — POST `/api/v1/projects/batch/status` — body: `{ "ids": ["..."] }`
- All handlers validate max 50 IDs, return partial results (some may fail)

**Test:** `internal/adapter/http/handlers_batch_test.go`
- Test batch delete with valid IDs returns count
- Test batch delete with mix of valid/invalid IDs returns partial errors
- Test batch delete with >50 IDs returns 400
- Test batch delete with empty list returns 400
- Test batch pull returns results per project
- Test batch status returns git status per project

**Run:** `go test ./internal/adapter/http/ -v -run TestBatch`

---

### 3.2 — Batch store methods

**Files:**
- Modify: `internal/port/database/store.go` — add `BatchDeleteProjects(ctx, ids []string) (int, error)`
- Modify: `internal/adapter/postgres/store.go` — implement with single DELETE ... WHERE id = ANY($1)

**What to do:**
- Use `DELETE FROM projects WHERE id = ANY($1) AND tenant_id = $2 RETURNING id`
- Return count of actually deleted rows
- Single query, not loop

**Test:** `internal/adapter/postgres/store_test.go` (extend existing)
- Create 3 projects, batch delete 2, verify 1 remains
- Batch delete with non-existent IDs returns 0 count
- Batch delete respects tenant isolation

**Run:** `go test -tags=integration ./internal/adapter/postgres/ -v -run TestBatchDelete`

---

### 3.3 — Frontend batch selection UI

**Files:**
- Modify: `frontend/src/features/dashboard/DashboardPage.tsx` — add multi-select mode
- Modify: `frontend/src/api/client.ts` — add batch API methods

**What to do:**
- API client:
  ```typescript
  batch: {
    deleteProjects: (ids: string[]) =>
      request<{ deleted: number; errors: BatchError[] }>(
        "/projects/batch/delete",
        { method: "POST", body: JSON.stringify({ ids }) },
      ),
    pullProjects: (ids: string[]) =>
      request<BatchResult[]>(
        "/projects/batch/pull",
        { method: "POST", body: JSON.stringify({ ids }) },
      ),
    statusProjects: (ids: string[]) =>
      request<BatchStatus[]>(
        "/projects/batch/status",
        { method: "POST", body: JSON.stringify({ ids }) },
      ),
  }
  ```
- Add `selectedIds` signal (`createSignal<Set<string>>`)
- Add checkbox on each ProjectCard (visible in batch mode)
- Add "Select All" / "Deselect All" toggle in header
- Add batch action bar (appears when selection > 0): "Delete Selected", "Pull Selected", "Check Status"
- Confirmation modal for batch delete with count

**Test:** Manual browser test — select multiple projects, execute batch action

---

### 3.4 — Add i18n keys and docs

**Files:**
- Modify: `frontend/src/i18n/en.ts` — add `batch.*` keys
- Modify: `frontend/src/i18n/de.ts` — add `batch.*` keys
- Modify: `docs/todo.md` — mark batch operations `[x]`

**Commit:** `feat(batch): add batch operations for projects (delete, pull, status)`

---

## Feature 4: Cross-Repo Search

> Scope-based cross-project search already exists (`SearchScope` in `internal/service/retrieval.go`). This feature adds a dedicated UI for cross-repo code search and improves the aggregation.

### 4.1 — Global search HTTP endpoint

**Files:**
- Create: `internal/adapter/http/handlers_search.go`
- Modify: `internal/adapter/http/routes.go`

**What to do:**
- `GlobalSearch` handler — POST `/api/v1/search` — body: `{ "query": "...", "project_ids": ["..."] (optional), "types": ["code", "issues"] (optional), "limit": 20 }`
- If `project_ids` is empty, search all projects the tenant has access to
- Delegates to existing `retrieval.Search()` per project, merges and ranks results by score
- Response: `{ "results": [{ "project_id", "project_name", "file", "line", "snippet", "score" }], "total": N }`

**Test:** `internal/adapter/http/handlers_search_test.go`
- Test with query returns results
- Test with project_ids filter limits scope
- Test with empty query returns 400
- Test results are sorted by score descending
- Test limit is respected (max 100)

**Run:** `go test ./internal/adapter/http/ -v -run TestGlobalSearch`

---

### 4.2 — Search results aggregation service

**Files:**
- Modify: `internal/service/retrieval.go` — add `GlobalSearch` method

**What to do:**
- `GlobalSearch(ctx, query string, projectIDs []string, limit int) ([]GlobalSearchResult, error)`
- If projectIDs empty, load all tenant projects via `store.ListProjects(ctx)`
- Fan out search to each project concurrently (max 10 goroutines via semaphore)
- Merge results, sort by score, truncate to limit
- `GlobalSearchResult`: `ProjectID string`, `ProjectName string`, `File string`, `Line int`, `Snippet string`, `Score float64`

**Test:** `internal/service/retrieval_test.go`
- Test fan-out to multiple projects
- Test results merged and sorted
- Test concurrent limit respected
- Test empty project list searches all

**Run:** `go test ./internal/service/ -v -run TestGlobalSearch`

---

### 4.3 — Frontend global search page

**Files:**
- Create: `frontend/src/features/search/SearchPage.tsx`
- Modify: `frontend/src/App.tsx` — add route `/search`
- Modify: `frontend/src/components/layout/Sidebar.tsx` — add search nav link
- Modify: `frontend/src/api/client.ts` — add `search.global()` method

**What to do:**
- Search input with debounce (300ms)
- Optional project filter dropdown (multi-select from loaded projects)
- Results list: project badge + file path + line number + code snippet (syntax highlighted via `<pre>`)
- Click result navigates to project detail page
- Loading state, empty state ("No results"), error state
- API: `search: { global: (query, projectIds?, limit?) => request<GlobalSearchResponse>("/search", ...) }`

**Test:** Manual browser test

---

### 4.4 — Add i18n keys and docs

**Files:**
- Modify: `frontend/src/i18n/en.ts` — add `search.*` keys
- Modify: `frontend/src/i18n/de.ts` — add `search.*` keys
- Modify: `docs/todo.md` — mark cross-repo search `[x]`

**Commit:** `feat(search): add cross-repo global search with aggregated results`

---

## Feature 5: Enhanced Agent Backend Wrappers

> All 5 backends (Aider, Goose, OpenCode, OpenHands, Plandex) have basic CLI wrappers in `workers/codeforge/backends/`. This enhances them with real-time WebSocket streaming, config passthrough, and health checks.

### 5.1 — Streaming output via NATS bridge

**Files:**
- Modify: `workers/codeforge/backends/_base.py` — add `StreamingOutputCallback` protocol
- Modify: `workers/codeforge/backends/router.py` — accept NATS publish callback

**What to do:**
- Add `StreamingOutputCallback` protocol: `async def __call__(self, line: str, stream: str = "stdout") -> None`
- In `BackendRouter.execute()`, wrap the `on_output` callback to also publish each line to NATS subject `agent.output.{task_id}`
- Each output message: `{ "task_id": "...", "line": "...", "stream": "stdout|stderr", "timestamp": "..." }`
- Go Core subscribes to `agent.output.*` and forwards to WebSocket clients

**Test:** `workers/tests/test_backend_streaming.py`
- Test output callback receives all lines
- Test NATS publish is called for each line
- Test stream type (stdout/stderr) is preserved

**Run:** `cd workers && poetry run pytest tests/test_backend_streaming.py -v`

---

### 5.2 — NATS subject and Go-side WebSocket forwarding

**Files:**
- Modify: `internal/port/messagequeue/subjects.go` — add `SubjectAgentOutput = "agent.output.*"`
- Modify: `internal/port/messagequeue/jetstream.go` — add `agent.>` to stream config if not present
- Modify: `internal/service/agent.go` — subscribe to agent output, forward to WebSocket

**What to do:**
- Add NATS subscription for `agent.output.*` in agent service
- On each message, extract task_id from subject, forward payload to WebSocket hub for that task's project
- WebSocket event type: `"agent_output"` with `{ task_id, line, stream, timestamp }`

**Test:** `internal/service/agent_test.go`
- Test output message is forwarded to WebSocket
- Test malformed message is logged and skipped

**Run:** `go test ./internal/service/ -v -run TestAgentOutput`

---

### 5.3 — Config passthrough for backends

**Files:**
- Modify: `workers/codeforge/backends/aider.py` — add `extra_env` config field
- Modify: `workers/codeforge/backends/goose.py` — same
- Modify: `workers/codeforge/backends/opencode.py` — same
- Modify: `workers/codeforge/backends/plandex.py` — same

**What to do:**
- Each backend's `ConfigField` schema: add `extra_env` field (type=dict, optional) — passed as environment variables to subprocess
- Each backend's `ConfigField` schema: add `working_dir_override` field (type=str, optional) — overrides workspace_path
- In `execute()`, merge `extra_env` into subprocess env: `env = {**os.environ, **config.get("extra_env", {})}`
- This allows users to pass `OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, etc. directly to backends

**Test:** `workers/tests/test_backend_config_passthrough.py`
- Test extra_env is merged into subprocess environment
- Test working_dir_override changes cwd
- Test missing extra_env uses default environment

**Run:** `cd workers && poetry run pytest tests/test_backend_config_passthrough.py -v`

---

### 5.4 — Backend health check endpoint

**Files:**
- Create: `internal/adapter/http/handlers_backend_health.go`
- Modify: `internal/adapter/http/routes.go`

**What to do:**
- `CheckBackendHealth` — GET `/api/v1/backends/health` — for each registered backend, check if CLI is available
- Returns `{ "backends": { "aider": { "available": true, "version": "0.82.0" }, "goose": { "available": false, "error": "not found" } } }`
- Dispatches to Python worker via NATS `backend.health.request` then `backend.health.result`

**Test:** `internal/adapter/http/handlers_backend_health_test.go`
- Test returns all registered backends
- Test available backend shows version
- Test unavailable backend shows error

**Run:** `go test ./internal/adapter/http/ -v -run TestBackendHealth`

**Commit:** `feat(backends): add streaming output, config passthrough, and health checks`

---

## Feature 6: Trajectory Replay UI Integration

> TrajectoryPanel.tsx and SessionPanel.tsx already exist with browse, replay, fork, and rewind. However, detailed tool-level event recording from the Python agent loop is partial, and the panels need integration as a project tab.

### 6.1 — Detailed tool-level event recording in agent loop

**Files:**
- Modify: `workers/codeforge/agent_loop.py` — emit granular events to NATS

**What to do:**
- After each tool call completes, publish event to NATS `agent.event.{run_id}`:
  ```python
  {
    "type": "agent.tool_called",
    "run_id": run_id,
    "tool_name": tool_name,
    "input": truncated_input(500),
    "output": truncated_output(1000),
    "tokens_in": tokens_in,
    "tokens_out": tokens_out,
    "cost_usd": cost,
    "duration_ms": elapsed_ms,
    "model": model_name
  }
  ```
- After each LLM call (not just tool calls), publish `agent.step_done` event
- On loop completion, publish `agent.finished` with total metrics

**Test:** `workers/tests/test_agent_loop_events.py`
- Test tool call emits event with correct type
- Test input/output are truncated at limits
- Test loop completion emits finished event
- Test metrics (tokens, cost) are populated

**Run:** `cd workers && poetry run pytest tests/test_agent_loop_events.py -v`

---

### 6.2 — Go-side event ingestion from NATS to event store

**Files:**
- Modify: `internal/service/agent.go` — subscribe to `agent.event.*`, persist to event store

**What to do:**
- Add NATS subscription for `agent.event.>` (wildcard)
- Parse each message as `AgentEvent`, call `eventStore.Append(ctx, &ev)`
- Extract `run_id` from subject suffix for routing
- Handle `tenant_id` from NATS headers (set by Python worker)
- Log and skip malformed events (don't crash)

**Test:** `internal/service/agent_test.go`
- Test event from NATS is persisted to event store
- Test malformed event is logged and skipped
- Test tenant_id is extracted correctly

**Run:** `go test ./internal/service/ -v -run TestEventIngestion`

---

### 6.3 — Trajectory tab in ProjectDetailPage

**Files:**
- Modify: `frontend/src/features/project/ProjectDetailPage.tsx` — add "Trajectory" tab

**What to do:**
- Add "Trajectory" as a new sub-tab in the project detail page (alongside existing tabs)
- The tab shows a run selector dropdown (load runs for project via existing conversation API)
- When a run is selected, render `<TrajectoryPanel runId={selectedRunId} />`
- If no runs exist, show empty state message
- The panel already supports browse, replay, fork, rewind — no changes needed there

**Test:** Manual browser test — navigate to project, select Trajectory tab, pick a run

---

### 6.4 — Trajectory export as downloadable JSON

**Files:**
- Modify: `frontend/src/features/project/TrajectoryPanel.tsx` — add export button

**What to do:**
- Add "Export" button in TrajectoryPanel header (next to replay controls)
- On click: fetch `/api/v1/runs/{id}/trajectory/export` (endpoint already exists)
- Trigger browser download as `trajectory-{runId}.json`
- Use `Blob` + `URL.createObjectURL` + hidden `<a>` click pattern

**Test:** Manual browser test — click export, verify JSON file downloads

**Commit:** `feat(trajectory): add detailed event recording, project tab, and JSON export`

---

## Feature 7: Session Resume/Fork/Rewind Integration

> Session domain model, service, HTTP handlers, and frontend panels all exist. Missing: integration with ChatPanel so users can trigger Resume/Fork/Rewind from the conversation UI.

### 7.1 — Session controls in ChatPanel

**Files:**
- Modify: `frontend/src/features/project/ChatPanel.tsx` — add session action buttons

**What to do:**
- Add a session toolbar below the chat header (visible when a conversation has a completed run)
- "Resume" button: calls `api.runs.resume(currentRunId)`, shows toast, reloads conversation
- "Fork from here" button: calls `api.runs.fork(currentRunId, { from_event_id: lastEventId })`, shows toast
- "Session History" toggle: expands inline `<SessionPanel projectId={projectId} />` below toolbar
- Buttons disabled when no run is active or run is still in progress
- Use existing `api.runs` client methods (already defined in `client.ts`)

**Test:** Manual browser test — open chat with completed run, test Resume and Fork buttons

---

### 7.2 — Session indicator in conversation list

**Files:**
- Modify: `frontend/src/features/project/ChatPanel.tsx` (conversation list section)

**What to do:**
- In the conversation sidebar list, show a small session badge next to conversations that have active sessions
- Fetch session for each conversation via `api.conversations.session(convId)` (endpoint exists)
- Badge shows session status: green dot for active, yellow for paused, gray for completed
- On badge click, show session metadata tooltip (parent run, created date)

**Test:** Manual browser test — verify badges appear on conversations with sessions

---

### 7.3 — Rewind with event picker

**Files:**
- Modify: `frontend/src/features/project/TrajectoryPanel.tsx` — improve rewind UX

**What to do:**
- Current rewind button exists but sends `to_event_id` from clicked event
- Add a "Rewind to here" context action on each event in the timeline (hover button)
- Show confirmation dialog: "Rewind to event #N? This will create a new session from this point."
- After rewind, show toast with session ID and offer to navigate to the new session

**Test:** Manual browser test — open trajectory, hover event, select rewind

**Commit:** `feat(session): integrate Resume/Fork/Rewind into ChatPanel and trajectory UI`

---

## Feature 8: CI Polish

> CI has Go, Python, Frontend, Contract, Smoke jobs. Missing: status badge, verification matrix as artifact, required PR checks.

### 8.1 — Add CI status badge to README

**Files:**
- Modify: `README.md` — add badge at top

**What to do:**
- Add GitHub Actions CI badge: `![CI](https://github.com/OWNER/CodeForge/actions/workflows/ci.yml/badge.svg?branch=staging)`
- Add contract tests badge: same pattern for contract job
- Place badges right after the project title/description

**Test:** Push to staging, verify badge renders on GitHub

---

### 8.2 — Upload verification matrix as CI artifact

**Files:**
- Modify: `.github/workflows/ci.yml` — add artifact upload step

**What to do:**
- After the smoke test job, add step:
  ```yaml
  - name: Generate verification matrix
    run: bash scripts/verify-features.sh || true
  - name: Upload verification matrix
    uses: actions/upload-artifact@v4
    with:
      name: verification-matrix
      path: /tmp/verification-summary.json
      retention-days: 30
  ```
- The `verify-features.sh` script already exists and generates `/tmp/verification-summary.json`

**Test:** Push to staging, verify artifact appears in Actions tab

---

### 8.3 — Stabilize job names for branch protection

**Files:**
- Modify: `.github/workflows/ci.yml` — ensure job names are stable

**What to do:**
- Ensure these job names are stable (don't change them):
  - `test-go` — Go unit tests
  - `test-python` — Python tests
  - `test-frontend` — Frontend lint + build
  - `contract` — NATS contract validation
- Add concurrency group to prevent duplicate runs:
  ```yaml
  concurrency:
    group: ci-${{ github.ref }}
    cancel-in-progress: true
  ```
- Document in `docs/dev-setup.md`: which checks to enable as required in GitHub branch protection settings

**Test:** Open PR against main, verify all 4 checks run

---

### 8.4 — Verification gate job

**Files:**
- Modify: `.github/workflows/ci.yml` — add `verify` job

**What to do:**
- Add `verify` job that runs after `test-go`, `test-python`, `contract`:
  ```yaml
  verify:
    needs: [test-go, test-python, contract]
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Run verification
        run: bash scripts/verify-features.sh
  ```
- The script already exits 1 if critical features (1-10, 22-23) fail
- This job can be set as required check in branch protection

**Test:** Verify job passes in CI when all tests pass

**Commit:** `ci: add status badges, verification artifact, and required check gates`

---

## Cross-Reference: docs/todo.md Updates

After completing each feature, mark the corresponding item in `docs/todo.md`:

| Feature | TODO Item to mark `[x]` |
|---------|-------------------------|
| 1 | `Implement GitHub adapter with OAuth flow` |
| 2 | `Verify GitHub adapter compatibility with Forgejo/Codeberg` |
| 3 | `Batch operations across selected repos` |
| 4 | `Cross-repo search (code, issues)` |
| 5 | `Enhance CLI wrappers for Goose, OpenHands, OpenCode, Plandex` |
| 6 | `Trajectory replay UI and audit trail` |
| 7 | `Session events as source of truth (Resume/Fork/Rewind)` |
| 8 | `Upload verification matrix as CI artifact` + `Add status badge to README` + `Add verification reporter as required CI check` |
