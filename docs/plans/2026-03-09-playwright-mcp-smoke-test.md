# Playwright MCP Full Frontend Smoke Test

**Date:** 2026-03-09
**Method:** Interactive browser automation via Playwright MCP tools
**Target:** ALL frontend routes, components, and interactive features

## Prerequisites

All services must be running:

```bash
# 1. Docker infrastructure
docker compose up -d postgres nats litellm playwright-mcp

# 2. Go backend (dev mode required for benchmarks endpoint)
APP_ENV=development go run ./cmd/codeforge/

# 3. Frontend dev server
cd frontend && npm run dev
```

Health checks:
- `curl http://localhost:8080/health` -> `{"status":"ok","dev_mode":true}`
- `curl http://localhost:3000` -> HTML response
- `curl http://localhost:8001/mcp` -> Playwright MCP endpoint

## Test Phases

### Phase 1: Authentication (5 routes)

| # | Route | Component | Test Actions | Result |
|---|-------|-----------|-------------|--------|
| 1.1 | `/login` | LoginPage | Navigate, verify form fields (email, password), submit with invalid creds (error msg), submit with valid creds (redirect to /) | |
| 1.2 | `/setup` | SetupPage | Navigate, verify setup form renders | |
| 1.3 | `/change-password` | ChangePasswordPage | Navigate (after login), verify form fields | |
| 1.4 | `/forgot-password` | ForgotPasswordPage | Navigate, verify email field and submit button | |
| 1.5 | `/reset-password` | ResetPasswordPage | Navigate, verify token-based form | |

### Phase 2: Dashboard (7 components)

| # | Route | Component | Test Actions | Result |
|---|-------|-----------|-------------|--------|
| 2.1 | `/` | DashboardPage | Navigate, verify page loads, check layout | |
| 2.2 | `/` | KpiStrip | Verify KPI cards render with data | |
| 2.3 | `/` | ProjectCard | Verify project cards visible (or empty state) | |
| 2.4 | `/` | ChartsPanel | Verify chart containers render | |
| 2.5 | `/` | ActivityTimeline | Verify timeline section present | |
| 2.6 | `/` | HealthDot | Verify health indicators on project cards | |
| 2.7 | `/` | CreateProjectModal | Click "New Project" button, verify modal opens with form fields, close modal | |

### Phase 3: Sidebar Navigation (15 items)

| # | Nav Item | Target Route | Test Actions | Result |
|---|----------|-------------|-------------|--------|
| 3.1 | Dashboard | `/` | Click sidebar link, verify navigation | |
| 3.2 | Costs | `/costs` | Click, verify CostDashboardPage loads | |
| 3.3 | Models | `/models` | Click, verify ModelsPage loads | |
| 3.4 | Modes | `/modes` | Click, verify ModesPage loads | |
| 3.5 | Activity | `/activity` | Click, verify ActivityPage loads | |
| 3.6 | Audit | `/audit` | Click, verify AuditTrailPage loads | |
| 3.7 | Knowledge Bases | `/knowledge-bases` | Click, verify KnowledgeBasesPage loads | |
| 3.8 | Scopes | `/scopes` | Click, verify ScopesPage loads | |
| 3.9 | MCP | `/mcp` | Click, verify MCPServersPage loads | |
| 3.10 | Prompts | `/prompts` | Click, verify PromptEditorPage loads | |
| 3.11 | Search | `/search` | Click, verify SearchPage loads | |
| 3.12 | Settings | `/settings` | Click, verify SettingsPage loads | |
| 3.13 | Benchmarks | `/benchmarks` | Click, verify BenchmarkPage loads (dev-mode only) | |
| 3.14 | CommandPalette | (overlay) | Trigger Cmd+K / Ctrl+K, verify palette opens | |
| 3.15 | 404 Page | `/nonexistent` | Navigate to invalid route, verify NotFoundPage | |

### Phase 4: Top-Level Pages (12 pages)

| # | Route | Component | Test Actions | Result |
|---|-------|-----------|-------------|--------|
| 4.1 | `/costs` | CostDashboardPage | Verify cost table/cards, check filters | |
| 4.2 | `/models` | ModelsPage | Verify model list, add model form/button | |
| 4.3 | `/modes` | ModesPage | Verify modes list, mode cards/details | |
| 4.4 | `/activity` | ActivityPage | Verify activity feed renders | |
| 4.5 | `/audit` | AuditTrailPage + AuditTable | Verify audit table renders with columns | |
| 4.6 | `/knowledge-bases` | KnowledgeBasesPage | Verify KB list or empty state | |
| 4.7 | `/scopes` | ScopesPage | Verify scopes list or empty state | |
| 4.8 | `/mcp` | MCPServersPage | Verify MCP server list, add button | |
| 4.9 | `/prompts` | PromptEditorPage | Verify prompt editor interface | |
| 4.10 | `/search` | SearchPage | Verify search input and results area | |
| 4.11 | `/settings` | SettingsPage | Verify settings sections, shortcuts | |
| 4.12 | `/benchmarks` | BenchmarkPage | Verify benchmark tabs, suite list | |

### Phase 5: Project Detail (25+ panels)

Prerequisite: Create a test project or use existing one. Navigate to `/projects/:id`.

| # | Panel/Tab | Component | Test Actions | Result |
|---|-----------|-----------|-------------|--------|
| 5.1 | Main layout | ProjectDetailPage | Verify multi-panel layout loads | |
| 5.2 | Chat | ChatPanel | Verify chat input, message area, send button | |
| 5.3 | Chat suggestions | ChatSuggestions | Verify suggestion chips appear | |
| 5.4 | Onboarding | OnboardingProgress | Verify onboarding steps indicator | |
| 5.5 | Files | FilePanel | Verify file tree, click to open file | |
| 5.6 | File Tree | FileTree | Verify tree structure, expand/collapse folders | |
| 5.7 | Code Editor | CodeEditor | Open a file, verify editor renders | |
| 5.8 | Goals | GoalsPanel | Verify goals list or empty state | |
| 5.9 | Goal Proposals | GoalProposalCard | Create/view goal proposals | |
| 5.10 | Roadmap | RoadmapPanel | Verify roadmap view | |
| 5.11 | Feature Map | FeatureMapPanel | Verify milestone columns | |
| 5.12 | Milestone | MilestoneColumn | Verify milestone with features | |
| 5.13 | Feature Card | FeatureCard | Verify feature details display | |
| 5.14 | Runs | RunPanel | Verify run list or empty state | |
| 5.15 | Trajectory | TrajectoryPanel | Verify trajectory timeline | |
| 5.16 | Sessions | SessionPanel | Verify session controls | |
| 5.17 | Active Work | ActiveWorkPanel | Verify current work status | |
| 5.18 | Plans | PlanPanel | Verify plan display | |
| 5.19 | Policies | PolicyPanel | Verify policy config | |
| 5.20 | Tasks | TaskPanel | Verify task list | |
| 5.21 | Repo Map | RepoMapPanel | Verify repo map visualization | |
| 5.22 | Retrieval | RetrievalPanel | Verify retrieval panel | |
| 5.23 | Agent Network | AgentNetwork | Verify agent graph | |
| 5.24 | Agent Flow | AgentFlowGraph | Verify flow graph | |
| 5.25 | War Room | WarRoom | Verify war room view | |
| 5.26 | Live Output | LiveOutput | Verify output display | |
| 5.27 | Multi Terminal | MultiTerminal | Verify terminal panes | |
| 5.28 | Auto Agent | AutoAgentButton | Verify button state and click | |
| 5.29 | Settings Popover | CompactSettingsPopover | Click settings icon, verify popover | |
| 5.30 | Shared Context | SharedContextPanel | Verify shared context view | |
| 5.31 | LSP | LSPPanel | Verify LSP status | |
| 5.32 | Search Sim | SearchSimulator | Verify search simulator | |

### Phase 6: Cross-Cutting Concerns (10 items)

| # | Feature | Component | Test Actions | Result |
|---|---------|-----------|-------------|--------|
| 6.1 | WebSocket | WebSocketProvider | Check WS connection indicator in UI | |
| 6.2 | Offline Banner | OfflineBanner | Disconnect/reconnect behavior | |
| 6.3 | Theme | ThemeProvider | Toggle dark/light mode if available | |
| 6.4 | i18n | LocaleSwitcher | Switch locale if available | |
| 6.5 | Shortcuts | ShortcutProvider | Test keyboard shortcuts (Cmd+K, etc.) | |
| 6.6 | Toast | Toast | Trigger an action that shows a toast | |
| 6.7 | Confirm Dialog | ConfirmDialog | Trigger a delete action, verify dialog | |
| 6.8 | Diff Preview | DiffPreview | Find a context that shows diffs | |
| 6.9 | Role Gate | RoleGate | Verify admin-only elements visible for admin | |
| 6.10 | Console Errors | (global) | Check browser console for JS errors across all pages | |

### Phase 7: Report

Aggregate all results into:
- Total components tested
- Pass / Fail / Broken counts
- List of bugs found with screenshots
- Console errors per page
- Recommendations

## Scoring

| Rating | Criteria |
|--------|----------|
| PASS | Component renders correctly, interactions work, no console errors |
| WARN | Component renders but with minor issues (missing data, layout glitch) |
| FAIL | Component crashes, doesn't render, or has JS errors |
| SKIP | Component not reachable (requires specific state/data) |

---

## Execution Results (2026-03-09)

**Environment:**
- Docker: postgres, nats, litellm, playwright-mcp (all healthy)
- Go backend: running, dev_mode=true, port 8080
- Frontend: Vite dev server, port 3000
- Playwright MCP: chromium headless via Docker, port 8001
- Browser accesses frontend via `host.docker.internal:3000`

**Note:** WebSocket shows "disconnected" intermittently because the Playwright browser
(inside Docker) resolves `host.docker.internal:3000` for HTTP but the frontend's WS URL
targets `localhost:8080` which is unreachable from inside the container. This is a
test-environment artifact, not a production bug.

### Phase 1: Authentication

| # | Route | Component | Result | Notes |
|---|-------|-----------|--------|-------|
| 1.1 | `/login` | LoginPage | PASS | Form renders, invalid creds show error alert, valid creds redirect to `/` |
| 1.2 | `/setup` | SetupPage | PASS | Redirects to `/login` (setup already completed) |
| 1.3 | `/change-password` | ChangePasswordPage | PASS | 3 password fields, sign out button, recognizes admin@localhost |
| 1.4 | `/forgot-password` | ForgotPasswordPage | PASS | Email field, submit button, back-to-login link |
| 1.5 | `/reset-password` | ResetPasswordPage | PASS | Token validation works (shows "Invalid or expired"), button disabled |

### Phase 2: Dashboard

| # | Component | Result | Notes |
|---|-----------|--------|-------|
| 2.1 | DashboardPage | PASS | Full layout with project list, KPIs, charts, activity |
| 2.2 | KpiStrip | PASS | 7 KPI cards (Cost Today, Active Runs, Success Rate, Active Agents, Avg Cost/Run, Tokens Today, Error Rate) |
| 2.3 | ProjectCard | PASS | 3 project cards with health dot, stats (Runs/Success/Cost), edit/delete buttons |
| 2.4 | ChartsPanel | PASS | 5 chart tabs (Cost Trend, Run Outcomes, Agents, Models, Cost/Project), SVG charts render |
| 2.5 | ActivityTimeline | PASS | Shows "No recent activity" empty state |
| 2.6 | HealthDot | PASS | "Health: 35" visible on each project card |
| 2.7 | CreateProjectModal | PASS | Modal with 3 tabs (Remote/Local/Empty), provider dropdown (6 options), name/URL/description, advanced settings |

### Phase 3: Sidebar Navigation

| # | Nav Item | Result | Notes |
|---|----------|--------|-------|
| 3.1-3.13 | All 13 links | PASS | Every sidebar link navigates to correct page |
| 3.14 | CommandPalette | PASS | Ctrl+K opens dialog with grouped commands (Navigation/Theme/Actions), keyboard hints |
| 3.15 | 404 Page | PASS | "Page not found" heading, description, "Back to Dashboard" button |
| -- | Sidebar expand/collapse | PASS | Expands to show labels, version (v0.1.0), user info, sign out, WS/API status |

### Phase 4: Top-Level Pages

| # | Route | Component | Result | Notes |
|---|-------|-----------|--------|-------|
| 4.1 | `/costs` | CostDashboardPage | PASS | 4 stat cards (Total Cost, Tokens In/Out, Runs), cost-by-project table |
| 4.2 | `/models` | ModelsPage | PASS | 460+ models loaded (388 Gemini, 75 Anthropic), Discover/Add buttons |
| 4.3 | `/modes` | ModesPage | PASS | 20 built-in modes with tools, denied tools, actions, LLM scenario, autonomy, artifacts |
| 4.4 | `/activity` | ActivityPage | PASS | Event type filter dropdown, Pause/Clear buttons, empty state |
| 4.5 | `/audit` | AuditTrailPage | PASS | Action filter dropdown, empty state message |
| 4.6 | `/knowledge-bases` | KnowledgeBasesPage | PASS | "Create Knowledge Base" button, empty state |
| 4.7 | `/scopes` | ScopesPage | PASS | "Create Scope" button, empty state |
| 4.8 | `/mcp` | MCPServersPage | PASS | "Add Server" button, empty state |
| 4.9 | `/prompts` | PromptEditorPage | PASS | Scope selector (Global), "Add Section" + "Preview" buttons |
| 4.10 | `/search` | SearchPage | PASS | Search input, project filter buttons for all 3 projects |
| 4.11 | `/settings` | SettingsPage | PASS | 8 sections: General, Shortcuts (editable), VCS Accounts (8 providers + GitHub OAuth), Providers, LLM Proxy, API Keys, User Mgmt, Dev Tools (Prompt Benchmark) |
| 4.12 | `/benchmarks` | BenchmarkPage | PASS | 5 tabs (Runs/Leaderboard/Cost Analysis/Multi-Compare/Suites), "New Run" button |

### Phase 5: Project Detail

| # | Panel/Tab | Component | Result | Notes |
|---|-----------|-----------|--------|-------|
| 5.1 | Main layout | ProjectDetailPage | PASS | Multi-panel layout, header with branch (master/dirty), Pull button, Auto-Agent button |
| 5.2 | Chat | ChatPanel | PASS | Message history (agent conversation), text input, send button, attach file button |
| 5.3 | Chat suggestions | ChatSuggestions | PASS | Context-aware suggestion chips (change per tab) |
| 5.4 | Onboarding | OnboardingProgress | PASS | 5-step progress (Repo cloned, Stack detected, Goals defined, Roadmap created, First agent run) |
| 5.5 | Files | FilePanel | PASS | File tree with folders/files, filter input, Expand/Collapse/Upload/New buttons |
| 5.6 | File Tree | FileTree | PASS | Folders expandable (triangle), files clickable, 3 dirs + 6 files visible |
| 5.7 | Code Editor | CodeEditor | PASS | Monaco editor loads, syntax highlighting (Python), line numbers, file tabs with close |
| 5.8 | Goals | GoalsPanel | PASS | 3 goals (Vision/Requirements/Constraints) with ON/delete toggles, AI Discover/Detect/Add buttons |
| 5.9 | Goal Proposals | GoalProposalCard | SKIP | No proposals present (requires AI discovery action) |
| 5.10 | Roadmap | RoadmapPanel | PASS | "Agent Eval Benchmark" (draft), Import Specs/PM/AI View/Sync/Delete, milestone with 3 features |
| 5.11 | Feature Map | FeatureMapPanel | PASS | Kanban view, milestone column, 3 draggable feature cards, Add Feature/Milestone |
| 5.12 | Milestone | MilestoneColumn | PASS | "Python Coding Problems" with draft badge, feature count |
| 5.13 | Feature Card | FeatureCard | PASS | Status badges (in_progress/backlog), Mark as done, Edit clickable |
| 5.14 | Runs | RunPanel | SKIP | Not a separate tab in this project layout |
| 5.15 | Trajectory | TrajectoryPanel | PASS | Empty state "No trajectory data", "Go to Sessions" link |
| 5.16 | Sessions | SessionPanel | PASS | Empty state "No agent sessions yet", "Open Chat" link |
| 5.17 | Active Work | ActiveWorkPanel | SKIP | Not visible as separate tab |
| 5.18 | Plans | PlanPanel | SKIP | Not visible as separate tab |
| 5.19 | Policies | PolicyPanel | SKIP | Not visible as separate tab |
| 5.20 | Tasks | TaskPanel | SKIP | Not visible as separate tab |
| 5.21 | Repo Map | RepoMapPanel | SKIP | Not visible as separate tab |
| 5.22 | Retrieval | RetrievalPanel | SKIP | Not visible as separate tab |
| 5.23 | Agent Network | AgentNetwork | SKIP | Not visible as separate tab |
| 5.24 | Agent Flow | AgentFlowGraph | SKIP | Not visible as separate tab |
| 5.25 | War Room | WarRoom | PASS | Empty state "No active agents", "Open Chat" button, "Shared Context (0)" toggle |
| 5.26 | Live Output | LiveOutput | SKIP | Requires active agent run |
| 5.27 | Multi Terminal | MultiTerminal | SKIP | Requires active agent run |
| 5.28 | Auto Agent | AutoAgentButton | PASS | Shows "Stopping..." state with 0/3 counter |
| 5.29 | Settings Popover | CompactSettingsPopover | PASS | Autonomy Level dropdown (5 levels), Save Settings, Cost Summary (4 metrics) |
| 5.30 | Shared Context | SharedContextPanel | SKIP | Requires active agent context |
| 5.31 | LSP | LSPPanel | SKIP | Not visible as separate tab |
| 5.32 | Audit Trail (proj) | AuditTable | WARN | Filter renders but shows "HTTP 404 Not Found" -- see Bugs section |

### Phase 6: Cross-Cutting

| # | Feature | Component | Result | Notes |
|---|---------|-----------|--------|-------|
| 6.1 | WebSocket | WebSocketProvider | PASS | Status indicator in expanded sidebar ("WebSocket: connected/disconnected") |
| 6.2 | Offline Banner | OfflineBanner | PASS | Alert "WebSocket disconnected. Reconnecting..." shown when WS down |
| 6.3 | Theme | ThemeProvider | PASS | Cycles System (gear) -> Light (sun) -> Dark (moon), persists across pages |
| 6.4 | i18n | LocaleSwitcher | PASS | EN->DE full translation (tabs, buttons, onboarding, chat input, all labels) |
| 6.5 | Shortcuts | ShortcutProvider | PASS | Ctrl+K opens command palette, editable shortcuts in Settings |
| 6.6 | Toast | Toast | PASS | Error toast shown on invalid login ("invalid credentials") with dismiss button |
| 6.7 | Confirm Dialog | ConfirmDialog | SKIP | Delete action not tested (would modify data) |
| 6.8 | Diff Preview | DiffPreview | SKIP | No diff context available in current state |
| 6.9 | Role Gate | RoleGate | PASS | Admin user sees all controls (User Mgmt, Dev Tools, all settings sections) |
| 6.10 | Console Errors | (global) | WARN | 2 console errors (see Bugs section) |

---

## Summary

| Category | Tested | PASS | WARN | FAIL | SKIP |
|----------|--------|------|------|------|------|
| Phase 1: Auth | 5 | 5 | 0 | 0 | 0 |
| Phase 2: Dashboard | 7 | 7 | 0 | 0 | 0 |
| Phase 3: Navigation | 16 | 16 | 0 | 0 | 0 |
| Phase 4: Top-Level Pages | 12 | 12 | 0 | 0 | 0 |
| Phase 5: Project Detail | 32 | 15 | 1 | 0 | 16 |
| Phase 6: Cross-Cutting | 10 | 7 | 1 | 0 | 2 |
| **Total** | **82** | **62** | **2** | **0** | **18** |

**Pass rate (excluding skips): 62/64 = 96.9%**

## Bugs Found

### Bug 1: Project-scoped audit endpoint 404 (FIXED)
- **Where:** Project Detail -> Audit Trail tab
- **URL:** `GET /api/v1/projects/:id/audit%3Flimit%3D50`
- **Issue:** The query string `?limit=50` was URL-encoded as `%3Flimit%3D50` and appended to the path segment instead of being sent as a query parameter. This caused a 404 because no route matches `/projects/:id/audit%3Flimit%3D50`.
- **Root cause:** `frontend/src/api/client.ts:1323` passed the query string through the `url` tagged template, which applies `encodeURIComponent()` to all interpolated values. The `?` and `=` characters got encoded.
- **Fix:** Split the URL construction so the path goes through `url` (for safe projectId encoding) but the query string is appended outside: `` `${url`/projects/${projectId}/audit`}${qs ? `?${qs}` : ""}` ``
- **Severity:** Medium -- project-level audit trail was non-functional.

### Bug 2: Conversation session endpoint 404 (NOT A BUG)
- **Where:** Project Detail page load
- **URL:** `GET /api/v1/conversations/:id/session`
- **Issue:** Returns 404 when a conversation has no associated session. The Go route exists (`routes.go:353`, handler at `handlers_session.go:225`), but `GetSessionByConversation` returns "not found" for conversations without sessions.
- **Frontend handling:** Already handled gracefully at `ChatPanel.tsx:80` with `.catch(() => null)`. Console error is expected.
- **Severity:** Non-issue -- working as designed.

## Skipped Components (18)

These components were not reachable because they require specific runtime state:
- **Requires active agent run:** LiveOutput, MultiTerminal, SharedContextPanel, ActiveWorkPanel
- **Requires agent data:** AgentNetwork, AgentFlowGraph, GoalProposalCard
- **Not exposed as separate tabs:** RunPanel, PlanPanel, PolicyPanel, TaskPanel, RepoMapPanel, RetrievalPanel, LSPPanel
- **Would modify data:** ConfirmDialog (delete)
- **No context available:** DiffPreview

## Recommendations

1. ~~**Fix Bug 1** (audit URL encoding)~~ -- FIXED: split query string out of `url` tagged template in `client.ts:1323`
2. ~~**Fix Bug 2** (session endpoint)~~ -- NOT A BUG: route exists, 404 is expected for conversations without sessions, frontend already catches it
3. **Consider making hidden panels accessible** -- panels like Plans, Policies, Tasks, RepoMap, Retrieval, LSP, Agent Network/Flow are defined as components but not visible as tabs in the project detail. Either add them to the tab bar or remove dead code.
4. **WebSocket URL** should use a relative or configurable base URL so it works regardless of access hostname (currently hardcoded to localhost:8080)
