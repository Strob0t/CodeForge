# Full Service Interactive QA Test Plan — Claude Code + Playwright-MCP

**Date:** 2026-03-18
**Type:** Interactive AI QA Test (Claude Code drives browser via playwright-mcp)
**Scope:** Complete CodeForge service — all frontend routes, API endpoints, WebSocket events
**Coverage:** 42 phases, 12 groups, ~318+ API endpoints, 10 frontend routes

---

## How to Use This Document

This is a **runbook for Claude Code sessions** using playwright-mcp tools. It is NOT a static test script — Claude Code follows the phases interactively, adapts to runtime conditions, and uses Decision Trees when steps fail.

**Prerequisites:**
- CodeForge full stack running in development mode
- `docker compose up -d postgres nats litellm`
- `APP_ENV=development go run ./cmd/codeforge/`
- `cd frontend && npm run dev`
- At least one LLM model configured (any provider)

**Execution:**
- Follow phases sequentially within each group
- Groups can run in parallel where marked
- Each phase: execute Steps, check Validation, follow Decision Tree on failure
- Record results as PASS / PARTIAL / SKIP / FAIL per phase

---

## Phase Dependency Graph

```
FOUNDATION (sequential, gate for everything)
├─ Phase 0:  Environment Discovery & Readiness
├─ Phase 1:  Auth & Login
└─ Phase 2:  Project Setup

DASHBOARD & ANALYTICS (parallel, need Phase 0-1)
├─ Phase 3:  Dashboard KPIs & Charts
└─ Phase 4:  Cost Dashboard

PROJECT WORKSPACE (sequential, need Phase 0-2)
├─ Phase 5:  File Operations
└─ Phase 6:  Git Operations

ROADMAP & PLANNING (sequential, need Phase 0-2)
├─ Phase 7:  Roadmap & Milestones
├─ Phase 8:  Feature Map (Kanban)
└─ Phase 9:  Goals

LLM MANAGEMENT (sequential, need Phase 0-1)
├─ Phase 10: Model Management
└─ Phase 11: LLM Key Management

CHAT — CORE (sequential, need Phase 0-2)
├─ Phase 12: Chat UI Navigation
├─ Phase 13: Simple Message & Response
├─ Phase 14: Streaming Observation
├─ Phase 15: Agentic Tool-Use
├─ Phase 16: HITL Permissions & Diff Preview
└─ Phase 17: Full Project Creation

CHAT — FEATURES (parallel, need Phase 12-13)
├─ Phase 18: Cost Tracking (per-message)
├─ Phase 19: Slash Commands
├─ Phase 20: Conversation Search
├─ Phase 21: Conversation Management (rewind, fork, sessions)
├─ Phase 22: Smart References
├─ Phase 23: Autonomy Controls
└─ Phase 24: Canvas Integration

AGENT ORCHESTRATION (parallel, need Phase 0-2)
├─ Phase 25: Mode Management
├─ Phase 26: Execution Plans
├─ Phase 27: War Room
├─ Phase 28: Sessions & Trajectory
└─ Phase 29: Agent Identity & Inbox

INFRASTRUCTURE (parallel, need Phase 0-1)
├─ Phase 30: MCP Server Management
├─ Phase 31: Knowledge Base & Retrieval
├─ Phase 32: Channels & Threads
├─ Phase 33: Policy Management
└─ Phase 34: Prompt Editor

NOTIFICATIONS
└─ Phase 35: Notifications

ADMIN (parallel, need Phase 0-1, admin role)
├─ Phase 36: Settings & Preferences
├─ Phase 37: Quarantine
├─ Phase 38: Boundaries & Contract Review
└─ Phase 39: Audit Trail

DEV-MODE
└─ Phase 40: Benchmarks

REPORT
└─ Phase 41: Report, Screenshots & Cleanup
```

---

## FOUNDATION

### Phase 0: Environment Discovery & Readiness

**Goal:** Verify all services running, discover models, classify capabilities.

**Steps:**
1. `browser_navigate` -> `http://localhost:3000` — frontend reachable?
2. `browser_snapshot` -> login page visible?
3. API check via `browser_evaluate`: `fetch('/api/v1/health').then(r=>r.json())` -> `{ status: "ok", dev_mode: true }`
4. Model discovery: `fetch('/api/v1/llm/discover', {headers:{'Authorization':'Bearer '+token}})` -> available models
5. Model classification: tool-capable? streaming? vision?
6. Store: `ENV = { models, bestModel, toolCapable, visionCapable, devMode }`

**Validation:**
- Frontend renders login page
- Backend responds with `dev_mode: true`
- At least 1 model available

**Decision Tree:**
```
Frontend unreachable?
├─ Timeout -> ABORT: "Frontend dev server not started (npm run dev)"
├─ Error page -> screenshot + ABORT: "Frontend error"
└─ Blank page -> browser_console_messages for JS errors

Backend /health failed?
├─ 502/503 -> ABORT: "Go backend not started"
├─ dev_mode: false -> WARN: "APP_ENV!=development, some features missing"
└─ Connection error -> ABORT: "Backend unreachable on port 8080"

No models found?
├─ LiteLLM? -> fetch('http://localhost:4000/health')
│  ├─ Down -> ABORT: "LiteLLM proxy not started"
│  └─ Up but 0 models -> WARN: "No API keys configured"
├─ Worker? -> check /health worker_status
│  └─ Offline -> ABORT: "Python worker not started"
└─ Models but none tool-capable ->
   FLAG: "Simple chat only, Phase 15-17 will be skipped"
```

---

### Phase 1: Auth & Login

**Goal:** Login with seeded admin credentials, verify auth flow.

**Steps:**
1. `browser_snapshot` -> login form visible (email + password fields)
2. `browser_fill_form` -> email: `admin@localhost`, password: `Changeme123`
3. `browser_click` -> submit/login button
4. `browser_wait_for` -> dashboard or redirect
5. `browser_snapshot` -> logged-in state (sidebar, avatar, project list)
6. Store auth token for API calls: `browser_evaluate`: extract from localStorage/cookie

**Validation:**
- Login form accepts credentials
- Redirect to dashboard after login
- Sidebar navigation visible (Projects, AI, MCP, Knowledge, Costs, etc.)
- Auth token available for subsequent API calls

**Decision Tree:**
```
Login form not visible?
├─ Already logged in (session cookie) -> proceed, skip login
├─ Setup page shown instead -> first-run setup needed
│  └─ Complete setup: create admin user, then login
└─ Different page entirely -> screenshot, check route

Login rejected (401)?
├─ admin@localhost/Changeme123 not working ->
│  POST /api/v1/auth/login via browser_evaluate to confirm
│  ├─ 401 -> credentials changed or seed not run -> ABORT
│  └─ 200 -> UI form broken, use token from API response
├─ CSRF/validation error -> check form fields, hidden inputs
└─ Network error -> backend down, back to Phase 0

Post-login redirect fails?
├─ Stuck on login page -> cookie not set? browser_evaluate: document.cookie
├─ Blank page after login -> JavaScript error, browser_console_messages
└─ 403 forbidden -> role/permission issue, check admin status
```

---

### Phase 2: Project Setup

**Goal:** Create a new test project with workspace.

**Steps:**
1. Navigate to dashboard -> `browser_snapshot` -> project list visible
2. "New Project" or "+" button -> `browser_click`
3. Fill project form:
   - Name: "QA-Test-Project"
   - Type: "local" (or "init new workspace")
   - Path: `/tmp/qa-test-project` (or auto-assigned)
4. Submit -> `browser_click`
5. `browser_wait_for` -> project created, detail page or list
6. `browser_snapshot` -> project visible
7. Open project detail page -> `browser_click` on project
8. `browser_snapshot` -> project detail with tabs (Files, Goals, Roadmap, etc.)

**Validation:**
- Project created successfully
- Project detail page shows tabs
- workspace_path is set (needed for chat)

**Decision Tree:**
```
"New Project" button not found?
├─ UI layout changed -> browser_snapshot, search all clickable elements
├─ Button in dropdown/menu -> try header area, sidebar
└─ API fallback: browser_evaluate: fetch('/api/v1/projects', {method:'POST', ...})

Project creation failed?
├─ Form validation error -> screenshot, read error
├─ API 409 (exists) -> use existing project
├─ API 500 -> browser_console_messages, ABORT if DB issue
└─ No workspace_path -> try "init-workspace" or "adopt" endpoint
   └─ Still no workspace -> WARN: "Chat features may not work"

Project detail page not loading?
├─ Route /projects/:id 404 -> project ID wrong?
├─ Tabs not rendering -> JavaScript error
└─ Permission denied -> admin role issue
```

---

## DASHBOARD & ANALYTICS

### Phase 3: Dashboard KPIs & Charts

**Goal:** Verify main dashboard shows statistics and charts.

**API Endpoints Tested:**
- `GET /api/v1/dashboard/stats`
- `GET /api/v1/dashboard/charts/cost-trend`
- `GET /api/v1/dashboard/charts/run-outcomes`
- `GET /api/v1/dashboard/charts/agent-performance`
- `GET /api/v1/dashboard/charts/model-usage`
- `GET /api/v1/dashboard/charts/cost-by-project`

**Steps:**
1. Navigate to dashboard (home route `/`) -> `browser_click` on logo/home
2. `browser_snapshot` -> KPI strip visible? (total projects, runs, cost, agents)
3. Scroll down -> `browser_evaluate`: `window.scrollTo(0, document.body.scrollHeight)`
4. `browser_snapshot` -> charts visible? (cost trend, run outcomes, model usage)
5. Verify KPI values are numeric and plausible
6. Click on a chart section (if interactive) -> `browser_click`
7. `browser_snapshot` -> detail view or tooltip

**Validation:**
- KPI strip shows at least: project count, run count, total cost
- At least 2 charts render (SVG or canvas elements)
- No "loading forever" spinners
- Data reflects actual state (project count matches created projects)

**Decision Tree:**
```
Dashboard shows no data?
├─ No projects/runs yet -> expected for fresh install
│  └─ PARTIAL PASS: "Dashboard renders but empty (no data yet)"
├─ API returns 500 -> database issue?
│  └─ browser_evaluate: fetch('/api/v1/dashboard/stats') check response
└─ Charts don't render -> JavaScript charting error
   └─ browser_console_messages, screenshot

KPI values clearly wrong?
├─ All zeros despite data -> aggregation query issue, WARN
├─ NaN / undefined shown -> formatting bug, screenshot
└─ Values from wrong tenant -> tenant isolation issue, WARN
```

---

### Phase 4: Cost Dashboard

**Goal:** Verify dedicated cost tracking page with time-series and breakdowns.

**API Endpoints Tested:**
- `GET /api/v1/costs`
- `GET /api/v1/projects/{id}/costs`
- `GET /api/v1/projects/{id}/costs/by-model`
- `GET /api/v1/projects/{id}/costs/by-tool`
- `GET /api/v1/projects/{id}/costs/daily`

**Steps:**
1. Navigate to `/costs` -> `browser_click` on "Costs" in sidebar
2. `browser_snapshot` -> cost dashboard visible
3. Time-range selector present? -> `browser_snapshot`
4. Select different time range -> `browser_click`
5. `browser_snapshot` -> chart updates
6. By-model breakdown visible? -> scroll/tab
7. By-tool breakdown visible? -> scroll/tab
8. Click on project filter (if available) -> `browser_click`

**Validation:**
- Cost page renders with chart(s)
- Global cost summary shows total
- Time-series chart shows data points (or empty state for new install)
- Breakdown tabs/sections (by model, by tool) are navigable

**Decision Tree:**
```
/costs route 404?
├─ Route not registered -> check sidebar links
│  └─ Try alternative routes: /dashboard/costs, /analytics
└─ Route exists but blank -> component not mounted, JS error

No cost data despite LLM usage?
├─ LiteLLM cost tracking off -> WARN: "LiteLLM not reporting costs"
├─ Cost aggregation lag -> data may appear after next run
└─ Wrong time range selected -> try "all time"

Charts don't render?
├─ Canvas/SVG missing -> charting library error
├─ Data format mismatch -> API returns data but chart can't parse
└─ CORS or auth issue on fetch -> browser_console_messages
```

---

## PROJECT WORKSPACE

### Phase 5: File Operations

**Goal:** Test file tree, read, write, rename, delete operations in project.

**API Endpoints Tested:**
- `GET /api/v1/projects/{id}/files`
- `GET /api/v1/projects/{id}/files/tree`
- `GET /api/v1/projects/{id}/files/content`
- `PUT /api/v1/projects/{id}/files/content`
- `PATCH /api/v1/projects/{id}/files/rename`
- `DELETE /api/v1/projects/{id}/files`

**Steps:**
1. Open project detail -> Files tab/panel
2. `browser_snapshot` -> file tree visible?
3. If empty: create a file via API first
   `browser_evaluate`: `fetch('/api/v1/projects/{id}/files/content', {method:'PUT', body: JSON.stringify({path:'test.txt', content:'hello'})})`
4. Refresh file tree -> `browser_click` on refresh or navigate away/back
5. `browser_snapshot` -> test.txt in tree?
6. Click on file -> `browser_click`
7. `browser_snapshot` -> file content visible?
8. Edit file content (if editor exists) -> change content
9. Rename file (if context menu) -> right-click or menu
10. Delete file -> context menu or button

**Validation:**
- File tree renders with directory structure
- Files are clickable and show content
- File CRUD operations work through UI
- Tree updates after operations

**Decision Tree:**
```
File tree empty despite workspace having files?
├─ workspace_path not set -> back to Phase 2
├─ Permission error on filesystem -> WARN: "File access denied"
└─ API 500 -> workspace path doesn't exist on disk

File content not displayed?
├─ Binary file selected -> expected, should show placeholder
├─ Large file -> truncation expected
├─ Encoding issue -> WARN

File operations not available in UI?
├─ Read-only mode -> check project permissions
├─ Context menu missing -> feature not in UI, test via API only
│  └─ PARTIAL PASS: "File read works, write/rename/delete API-only"
└─ UI exists but operations fail -> API error, console logs
```

---

### Phase 6: Git Operations

**Goal:** Test git status, branches, pull, checkout through UI.

**API Endpoints Tested:**
- `GET /api/v1/projects/{id}/git/status`
- `GET /api/v1/projects/{id}/git/branches`
- `POST /api/v1/projects/{id}/git/pull`
- `POST /api/v1/projects/{id}/git/checkout`

**Steps:**
1. Open project detail -> look for git status indicator
2. `browser_snapshot` -> branch name, clean/dirty status visible?
3. Click on git/branch UI element -> `browser_click`
4. `browser_snapshot` -> branch list or git panel?
5. If branch list: select a different branch -> `browser_click`
6. `browser_snapshot` -> branch switched?
7. Look for "Pull" button -> `browser_click` if available
8. `browser_snapshot` -> pull result

**Validation:**
- Current branch name displayed
- Git status (clean/dirty) shown
- Branch list accessible
- Pull operation executes

**Decision Tree:**
```
No git status visible?
├─ Project is not a git repo -> SKIP phase (local/non-git project)
├─ Git status in different location -> search project detail page
└─ API returns error -> git binary not available in container?

Branch switching fails?
├─ Uncommitted changes -> WARN: "Dirty workspace, checkout blocked"
├─ Branch doesn't exist -> list branches first, pick valid one
└─ Permission error -> WARN

Pull fails?
├─ No remote configured -> expected for local-only project
├─ Auth required -> VCS credentials not set up
└─ Network error -> WARN
```

---

## ROADMAP & PLANNING

### Phase 7: Roadmap & Milestones

**Goal:** Test roadmap creation, milestone CRUD, auto-detection, import.

**API Endpoints Tested:**
- `GET/POST/PUT/DELETE /api/v1/projects/{id}/roadmap`
- `POST /api/v1/projects/{id}/roadmap/detect`
- `GET /api/v1/projects/{id}/roadmap/ai`
- `POST /api/v1/projects/{id}/roadmap/milestones`
- `GET/PUT/DELETE /api/v1/milestones/{id}`

**Steps:**
1. Open project detail -> Roadmap tab -> `browser_click`
2. `browser_snapshot` -> roadmap view (empty or populated)
3. Create milestone: find "Add Milestone" -> `browser_click`
4. Fill form: Title="v1.0", Description="First release" -> `browser_fill_form`
5. Submit -> `browser_click`
6. `browser_snapshot` -> milestone visible?
7. "Auto-Detect" button (if present) -> `browser_click`
8. `browser_snapshot` -> detected specs/features?
9. "AI Suggestions" (if present) -> `browser_click`
10. `browser_snapshot` -> AI-generated roadmap suggestions?

**Validation:**
- Roadmap panel renders
- Milestone can be created and appears in view
- Auto-detect finds project spec files (if any)
- AI suggestions return useful content (or empty state)

**Decision Tree:**
```
Roadmap tab not visible?
├─ Tab renamed or in different position -> search panel tabs
├─ Feature disabled -> API check: GET /projects/{id}/roadmap
│  └─ 404 -> feature not enabled for project, SKIP
└─ Loading spinner forever -> API timeout

Milestone creation fails?
├─ Form validation -> required fields missing
├─ API 500 -> database error
└─ UI doesn't update -> refresh, check if created via API

Auto-detect returns nothing?
├─ No spec files in project -> expected for empty project
│  └─ PARTIAL PASS: "No specs to detect"
└─ Detector error -> WARN
```

---

### Phase 8: Feature Map (Kanban)

**Goal:** Test Kanban-style feature map with drag-drop.

**API Endpoints Tested:**
- `POST /api/v1/milestones/{id}/features`
- `GET/PUT/DELETE /api/v1/features/{id}`

**Steps:**
1. Open Feature Map tab/view -> `browser_click`
2. `browser_snapshot` -> Kanban columns visible? (MilestoneColumns)
3. Create feature: "Add Feature" -> `browser_click`
4. Fill: Title="User Auth", Status="planned" -> `browser_fill_form`
5. Submit -> `browser_click`
6. `browser_snapshot` -> feature card in Kanban column?
7. Drag feature to different column (if drag-drop works) -> `browser_drag`
8. `browser_snapshot` -> feature moved?
9. Edit feature -> `browser_click` on card
10. Delete feature -> delete button or menu

**Validation:**
- Kanban board renders with columns
- Feature creation works
- Cards appear in correct columns
- Drag-drop moves cards (or PARTIAL if not interactive)

**Decision Tree:**
```
Kanban not rendering?
├─ No milestones exist -> create one first (back to Phase 7)
├─ FeatureMapPanel component error -> console logs
└─ Empty board (no features) -> expected, create one

Drag-drop fails?
├─ browser_drag coordinates wrong -> calculate from snapshot
├─ Drag not implemented -> PARTIAL PASS: "Kanban read-only"
└─ Drop rejected -> status transition invalid?
```

---

### Phase 9: Goals

**Goal:** Test goal CRUD and AI discovery.

**API Endpoints Tested:**
- `GET/POST /api/v1/projects/{id}/goals`
- `POST /api/v1/projects/{id}/goals/detect`
- `POST /api/v1/projects/{id}/goals/ai-discover`
- `GET/PUT/DELETE /api/v1/goals/{id}`

**Steps:**
1. Open Goals tab -> `browser_click`
2. `browser_snapshot` -> goals list (empty or populated)
3. Create goal: "Add Goal" -> `browser_click`
4. Fill: Title="Implement Auth", Description="OAuth2 support" -> `browser_fill_form`
5. Submit -> `browser_click`
6. `browser_snapshot` -> goal in list?
7. "Auto-Detect" button -> `browser_click`
8. `browser_snapshot` -> detected goals from codebase?
9. "AI Discover" button -> `browser_click`
10. `browser_wait_for` -> AI-generated goals?
11. `browser_snapshot` -> suggestions list?

**Validation:**
- Goal CRUD works
- Auto-detect scans codebase for TODOs/FIXMEs
- AI discovery returns meaningful suggestions
- Goals link to conversations (if feature exists)

**Decision Tree:**
```
Goals tab missing?
├─ Conditional feature (GoalDiscovery service nil) -> SKIP
├─ Tab in different position -> search
└─ API 404 -> feature not compiled in, SKIP

AI discover fails?
├─ No LLM available -> SKIP AI features
├─ LLM timeout -> WARN: "AI discovery slow"
└─ Returns empty -> PARTIAL (codebase too small for meaningful goals)
```

---

## LLM MANAGEMENT

### Phase 10: Model Management

**Goal:** Test model listing, discovery, add/remove on /ai page.

**API Endpoints Tested:**
- `GET /api/v1/llm/models`
- `GET /api/v1/llm/discover`
- `GET /api/v1/llm/available`
- `POST /api/v1/llm/models`
- `POST /api/v1/llm/models/delete`
- `GET /api/v1/llm/health`

**Steps:**
1. Navigate to `/ai` -> `browser_click` on "AI" in sidebar
2. `browser_snapshot` -> model list visible?
3. Models listed with provider, name, capabilities?
4. "Discover" / "Refresh" button -> `browser_click`
5. `browser_wait_for` -> discovery completes
6. `browser_snapshot` -> new models found? (Ollama, LM Studio)
7. "Add Model" (if available) -> `browser_click`
8. Fill custom model form -> `browser_fill_form`
9. Health indicator for each model -> green/red/yellow?
10. "Remove" a model (if safe) -> `browser_click`

**Validation:**
- Model list shows at least 1 model
- Each model shows: provider, name, capabilities (tool, vision, streaming)
- Discovery finds local models (if Ollama/LM Studio running)
- Health status displayed

**Decision Tree:**
```
/ai page blank?
├─ Route not registered -> try /models, /settings/models
├─ Component error -> console logs
└─ Auth issue -> re-login

No models in list?
├─ LiteLLM not configured -> WARN: "No models, configure litellm-config.yaml"
├─ API returns empty array -> no providers set up
└─ Models exist but not shown -> rendering bug

Discovery finds nothing?
├─ No local model servers running -> expected
│  └─ PARTIAL: "Discovery works but no local servers found"
├─ Discovery endpoint timeout -> Ollama/LM Studio slow to respond
└─ Error during discovery -> API response check
```

---

### Phase 11: LLM Key Management

**Goal:** Test API key management for LLM providers.

**API Endpoints Tested:**
- `GET /api/v1/llm-keys`
- `POST /api/v1/llm-keys`
- `DELETE /api/v1/llm-keys/{id}`

**Steps:**
1. On /ai page (or Settings): find "API Keys" section
2. `browser_snapshot` -> key list visible (masked keys)
3. "Add Key" -> `browser_click`
4. Fill: Provider="test", Key="sk-test-1234" -> `browser_fill_form`
5. Submit -> `browser_click`
6. `browser_snapshot` -> key added (masked)?
7. Delete test key -> `browser_click` on delete
8. `browser_snapshot` -> key removed

**Validation:**
- Keys are listed but masked (not shown in full)
- Add key works
- Delete key works
- No plaintext key display

**Decision Tree:**
```
Key management not in UI?
├─ Feature in different location -> search settings pages
├─ API-only feature -> test via browser_evaluate
│  └─ PARTIAL: "Key CRUD works via API, no UI"
└─ Feature not implemented -> SKIP

Key shown in plaintext?
├─ SECURITY BUG -> screenshot, FAIL with security note
└─ Only first/last 4 chars shown -> correct masking, PASS
```

---

## CHAT — CORE

### Phase 12: Chat UI Navigation

**Goal:** Open chat panel in project and verify UI elements.

**Steps:**
1. Open project -> `browser_click` on project in list
2. Find chat panel -> `browser_snapshot` (bottom panel, sidebar, or tab)
3. Open chat -> `browser_click`
4. Verify elements: input field, send button, conversation list
5. `browser_snapshot` -> screenshot

**Validation:**
- Chat input visible and focusable
- Send button present
- Conversation list visible

**Decision Tree:**
```
Chat panel not visible?
├─ No workspace_path -> chat needs workspace, back to Phase 2
├─ Chat in different location -> search all panels/tabs
└─ WebSocket not connected -> wait for reconnect (3s)

Input not interactive?
├─ Modal/overlay blocking -> close it
├─ JS error -> browser_console_messages
└─ Disabled state -> check WebSocket connection
```

---

### Phase 13: Simple Message & Response

**Goal:** Send a message and receive LLM response.

**Steps:**
1. Click input -> `browser_click`
2. Type: "Hello, what model are you?" -> `browser_type`
3. Send -> `browser_click` or `browser_press_key("Enter")`
4. `browser_wait_for` -> assistant message appears (up to 30s)
5. `browser_snapshot` -> response visible
6. Read response content

**Validation:**
- User message appears in chat
- Assistant response with text
- No error state

**Decision Tree:**
```
Message not sent?
├─ Button disabled -> WebSocket? Empty input?
├─ Nothing happens -> browser_console_messages
└─ Try Enter key instead of button click

No response after 30s?
├─ Streaming indicator but no text -> worker issue
│  └─ Try different model
├─ Error message in chat -> read error
│  ├─ "Model not found" -> redo discovery
│  ├─ "Rate limit" -> wait 10s, retry
│  └─ "Worker unavailable" -> ABORT
├─ No reaction -> WebSocket dead, reload
└─ Empty response -> model problem, switch model
```

---

### Phase 14: Streaming Observation

**Goal:** Verify streaming — text appears progressively.

**Steps:**
1. Send: "Explain in 3 paragraphs what a compiler does"
2. Immediately `browser_snapshot` -> typing indicator?
3. Wait 2s -> `browser_snapshot` -> text growing?
4. Wait 5s -> `browser_snapshot` -> more text?
5. `browser_wait_for` -> run finished
6. `browser_console_messages` -> check AG-UI events
7. Final `browser_snapshot`

**Validation:**
- Typing indicator during generation
- Text grows progressively (snapshot comparison)
- AG-UI events: run_started, text_message, run_finished
- Indicator disappears after completion

**Decision Tree:**
```
No streaming (text as block)?
├─ Non-streaming model -> PARTIAL PASS
├─ WebSocket buffering -> check console for events
└─ SSE instead of WS -> architecture check

Indicator stuck?
├─ run_finished never received -> worker crash? NATS issue?
├─ UI bug -> reload, check if response in DB
└─ Slow model -> wait up to 120s
```

---

### Phase 15: Agentic Tool-Use

**Goal:** LLM uses tools — ToolCallCards appear.

**Prerequisite:** Tool-capable model from Phase 0. Otherwise SKIP.

**Steps:**
1. Send: "Create a file called hello.py with a simple Hello World script"
2. `browser_wait_for` -> ToolCallCard appears
3. `browser_snapshot` -> tool name visible
4. Tool arguments and result visible?
5. Wait for completion
6. Verify file: "Read the file hello.py"
7. `browser_snapshot`

**Validation:**
- ToolCallCard with tool name + arguments
- Tool result shows success
- File actually created

**Decision Tree:**
```
Model doesn't call tools?
├─ Not tool-capable -> try different model, or SKIP Phase 15-17
├─ Prompt too vague -> "Use the write_file tool to create hello.py"
├─ Model explains instead of doing -> "Do NOT explain. Create the file."
└─ Hallucinated tool use -> verify with Read, WARN if missing

ToolCallCard not visible?
├─ Tool call happens but no UI -> check agui.tool_call event
├─ HITL blocking -> continue to Phase 16
└─ Tool error -> read result, check workspace path

Run stuck >60s?
├─ Permission request off-screen -> scroll
├─ Worker stall -> console logs
└─ Max iterations -> WARN
```

---

### Phase 16: HITL Permissions & Diff Preview

**Goal:** Test approve/deny/allow-always + inline diff preview.

**Prerequisite:** Phase 15 passed.

**Steps:**
1. Set autonomy to "supervised" via `/mode` or CompactSettingsPopover
2. Send: "Run `echo hello` in the terminal"
3. `browser_wait_for` -> PermissionRequestCard
4. `browser_snapshot` -> tool, command, countdown visible
5. **Deny** -> `browser_click`
6. `browser_snapshot` -> agent reacts to denial
7. Send: "Run `echo test`"
8. `browser_wait_for` -> PermissionRequestCard
9. **Approve** -> `browser_click`
10. `browser_wait_for` -> tool executes
11. Send: "Create a file called diff-test.py with print('hi')"
12. `browser_wait_for` -> PermissionRequestCard with **DiffPreview**?
13. `browser_snapshot` -> inline diff (old/new code)?
14. Approve -> `browser_click`
15. Send: "Run `echo again`"
16. `browser_wait_for` -> PermissionRequestCard
17. **Allow-Always** -> `browser_click`
18. Send: "Run `echo final`" -> should run WITHOUT permission

**Validation:**
- PermissionRequestCard shows tool + command + countdown
- Deny works, Approve works, Allow-Always works
- DiffPreview shows code diff for write/edit operations
- Allow-Always persists (subsequent same-tool calls auto-approved)

**Decision Tree:**
```
No PermissionRequestCard?
├─ Autonomy not "supervised" -> set via API
├─ Policy preset allows all -> switch to "supervised-ask-all"
├─ Tool call doesn't happen -> back to Phase 15
└─ Permission off-screen -> scroll

DiffPreview not shown?
├─ Only for write/edit tools, not for bash -> expected
├─ Component not rendering -> check for DiffModal/DiffView
└─ Old content unknown (new file) -> diff shows "new file" mode

Allow-Always doesn't persist?
├─ POST /policies/allow-always failed -> console logs
├─ Policy cache -> reload + retest
└─ Tool name mismatch -> case sensitivity issue
```

---

### Phase 17: Full Project Creation

**Goal:** LLM creates complete project — multiple files, tests, structure.

**Prerequisite:** Phase 15+16 passed.

**Steps:**
1. Set autonomy to `auto-edit` or allow-always for all tools
2. Send comprehensive prompt:
   ```
   Create a complete Python CLI todo app with:
   - main.py with argparse
   - todo.py with TodoList class
   - test_todo.py with pytest tests
   - requirements.txt
   - README.md
   ```
3. `browser_snapshot` every 10s -> document progress
4. Count tool calls, observe step indicators
5. Wait up to 180s -> `browser_wait_for`
6. Verify: "List all files you created"
7. Optional: "Run the tests"
8. Final `browser_snapshot`

**Validation:**
- At least 3 files created
- No error states
- Assistant summarizes at end
- Cost badge shows > $0

**Decision Tree:**
```
Stops after 1-2 tool calls?
├─ Max iterations too low -> WARN
├─ Context window full -> smaller task
├─ Model gives up -> stronger model
└─ Budget limit -> WARN

Files missing?
├─ Write tool failed -> check result
├─ Model forgot -> follow-up prompt
└─ Path problem -> workspace config

Run >180s?
├─ Still active -> wait to 300s
├─ Stall >30s -> permission blocking? Worker stall?
└─ Infinite loop -> WARN "Agent loop"
```

---

## CHAT — FEATURES

### Phase 18: Cost Tracking (per-message)

**Goal:** Verify MessageBadge and CostBreakdown on messages.

**Steps:**
1. Open conversation with responses
2. `browser_snapshot` -> MessageBadge on assistant messages?
3. Click badge -> `browser_click` -> CostBreakdown expands?
4. `browser_snapshot` -> tokens, model, cost visible?
5. Send cheap message: "What is 2+2?"
6. `browser_wait_for` -> response
7. `browser_snapshot` -> new badge with cost

**Validation:**
- MessageBadge shows model + tokens + cost
- CostBreakdown expandable
- Cost > $0.00 (unless local model)

**Decision Tree:**
```
No badge? -> state_delta events missing, check console
Cost = $0? -> local model without pricing, WARN
Badge but no expand? -> click handler missing
```

---

### Phase 19: Slash Commands

**Goal:** Test all 8 built-in commands.

**Steps:**
1. `/help` -> Enter -> help text appears?
2. `/model` -> Enter -> current model shown?
3. `/mode` -> Enter -> current mode shown?
4. `/cost` -> Enter -> session cost summary?
5. `/diff` -> Enter -> file changes?
6. Send several messages for history
7. `/compact` -> Enter -> compaction?
8. `/clear` -> Enter -> chat cleared?
9. `/rewind` -> Enter -> checkpoint picker? (if available)

**Validation:**
- Each command produces expected output
- Invalid command shows error
- /clear actually clears messages

**Decision Tree:**
```
Command sent as message?
├─ CommandRegistry not loaded -> console
├─ "/" not intercepted -> input handler
└─ Server-side command -> check API response

/compact fails? -> API 500, worker issue
/rewind unavailable? -> no checkpoints, SKIP
```

---

### Phase 20: Conversation Search

**Goal:** Test PostgreSQL FTS across conversations.

**Steps:**
1. Ensure 2+ conversations with known content
2. Find search UI -> `browser_snapshot`
3. Search "compiler" -> `browser_type`
4. `browser_snapshot` -> relevant results?
5. Search "xyznonexistent" -> no results?
6. Click result -> opens conversation?

**Validation:**
- Search finds relevant conversations
- Filters irrelevant ones
- Results clickable

**Decision Tree:**
```
Search field missing? -> API test: POST /search/conversations
Finds nothing? -> FTS index not built, WARN
No filter? -> ranking issue, WARN
```

---

### Phase 21: Conversation Management

**Goal:** Test rewind, fork, sessions panel.

**Steps:**
1. Conversation with checkpoints (from Phase 15+)
2. Find rewind UI -> `browser_snapshot`
3. Open timeline -> `browser_click`
4. Click earlier checkpoint -> DiffSummaryModal?
5. Confirm rewind -> messages reset?
6. Create new conversation -> tab switch
7. Session panel -> past sessions listed?

**Validation:**
- Rewind with diff preview works
- Fork creates independent conversation
- Sessions panel shows history

**Decision Tree:**
```
No checkpoints? -> only after agentic runs, SKIP rewind
Rewind no effect? -> API check, reload
Session panel empty? -> no completed runs yet
```

---

### Phase 22: Smart References

**Goal:** Test @files, #conversations, //commands autocomplete.

**Steps:**
1. Type `@` -> AutocompletePopover appears?
2. Type `#` -> conversation autocomplete?
3. Type `//` -> command autocomplete?
4. Select entry -> TokenBadge inserted?
5. Typing filters results?
6. Escape closes popover?

**Validation:**
- Three trigger types work
- Fuzzy filtering
- TokenBadge rendering

**Decision Tree:**
```
No popover? -> component error, console
Empty popover? -> no files/conversations yet
No TokenBadge? -> plain text inserted instead, WARN
```

---

### Phase 23: Autonomy Controls

**Goal:** Test CompactSettingsPopover for autonomy level, mode, policy.

**Steps:**
1. Find settings gear/popover in chat -> `browser_snapshot`
2. Open popover -> `browser_click`
3. `browser_snapshot` -> autonomy selector, mode selector, policy preset
4. Change autonomy level -> `browser_click` on different level
5. `browser_snapshot` -> level changed?
6. Change policy preset -> `browser_click`
7. `browser_snapshot` -> preset changed?

**Validation:**
- Popover shows autonomy levels (1-5)
- Mode selector works
- Policy preset selector works
- Changes persist for conversation

**Decision Tree:**
```
Popover not found?
├─ Settings in different location -> search chat header
├─ Feature not in UI -> use /mode slash command instead
└─ SKIP if no UI control exists

Changes don't persist?
├─ API call failed -> console
├─ Optimistic UI reverted -> actual error
└─ Per-conversation vs global -> check scope
```

---

### Phase 24: Canvas Integration

**Goal:** Test design canvas — draw, export, multimodal chat.

**Steps:**
1. Find canvas button/tab -> `browser_snapshot`
2. Open canvas -> `browser_click`
3. `browser_snapshot` -> 7 tools visible?
4. Select rect tool -> draw rectangle via `browser_drag`
5. `browser_snapshot` -> rectangle visible?
6. Select text tool -> type "Hello"
7. Export panel -> `browser_click`
8. `browser_snapshot` -> PNG/ASCII/JSON options?
9. "Send to Chat" -> `browser_click`
10. `browser_snapshot` -> canvas in chat input?
11. Send message: "Describe this design"
12. `browser_wait_for` -> LLM response

**Validation:**
- Canvas renders with tools
- Drawing works
- Export produces output
- Canvas-to-chat pipeline functions
- LLM responds to canvas input

**Decision Tree:**
```
Canvas not found? -> feature not deployed, SKIP
Drawing fails? -> browser_drag coords, canvas bounds
Export empty? -> offscreen canvas issue
LLM ignores canvas? -> non-vision model, WARN (JSON/ASCII fallback)
```

---

## AGENT ORCHESTRATION

### Phase 25: Mode Management

**Goal:** Test agent mode CRUD, scenarios, tools listing.

**API Endpoints Tested:**
- `GET /api/v1/modes`
- `POST /api/v1/modes`
- `GET /api/v1/modes/{id}`
- `PUT /api/v1/modes/{id}`
- `DELETE /api/v1/modes/{id}`
- `GET /api/v1/modes/scenarios`
- `GET /api/v1/modes/tools`
- `GET /api/v1/modes/artifact-types`

**Steps:**
1. Find mode management UI (Settings or dedicated page)
2. `browser_snapshot` -> mode list (built-in modes: coder, architect, reviewer, etc.)
3. Built-in modes listed with description, tools, scenario?
4. "Create Mode" -> `browser_click`
5. Fill: Name="test-mode", Description="QA Test", Scenario="default" -> `browser_fill_form`
6. Submit -> `browser_click`
7. `browser_snapshot` -> custom mode in list?
8. Edit custom mode -> change description
9. Delete custom mode -> `browser_click`
10. Verify built-in modes cannot be deleted

**Validation:**
- Built-in modes listed (coder, architect, reviewer, debugger, etc.)
- Custom mode CRUD works
- Built-in modes protected from deletion
- Scenarios and tools endpoints return data

**Decision Tree:**
```
Mode UI not found?
├─ Mode management via API only ->
│  browser_evaluate: fetch('/api/v1/modes')
│  └─ PARTIAL: "Modes work via API, no dedicated UI"
├─ Mode selection only in chat (via /mode) -> test there
└─ /modes route exists -> navigate directly

Custom mode creation fails?
├─ Validation error -> required fields
├─ Duplicate name -> use unique name
└─ API 500 -> database schema issue
```

---

### Phase 26: Execution Plans

**Goal:** Test plan creation, visualization, execution.

**API Endpoints Tested:**
- `POST /api/v1/projects/{id}/plans`
- `GET /api/v1/projects/{id}/plans`
- `GET /api/v1/plans/{id}`
- `POST /api/v1/plans/{id}/start`
- `GET /api/v1/plans/{id}/graph`

**Steps:**
1. Find execution plans UI -> `browser_snapshot`
2. Create plan (or via decompose): `browser_click` on "Create Plan"
3. `browser_snapshot` -> plan form?
4. Fill plan details or use AI decomposition
5. `browser_snapshot` -> plan with steps visible?
6. Plan graph visualization -> `browser_click` on graph tab
7. `browser_snapshot` -> AgentFlowGraph renders?
8. Start plan execution (if safe) -> `browser_click`
9. Monitor execution status

**Validation:**
- Plan creation works
- Steps/graph visualization renders
- Plan can be started
- Status updates flow via WebSocket

**Decision Tree:**
```
Plans UI not found?
├─ Feature in project detail -> check all tabs
├─ API-only -> test via browser_evaluate
└─ SKIP if no UI

Plan creation requires decomposition first?
├─ POST /projects/{id}/decompose -> trigger first
├─ Manual plan creation -> fill steps manually
└─ No tasks/features to plan -> create goals first (Phase 9)

Plan execution fails?
├─ No agents configured -> create agent first
├─ Worker not running -> ABORT
└─ Budget exceeded -> WARN
```

---

### Phase 27: War Room

**Goal:** Test multi-agent collaboration view.

**Steps:**
1. Open project detail -> War Room tab -> `browser_click`
2. `browser_snapshot` -> War Room view (AgentLanes, FlowGraph, SharedContext)
3. If agents are active: observe agent lanes with status
4. If no agents: verify empty state message
5. AgentFlowGraph -> dependency arrows between agents?
6. MessageFlow -> handoff visualization?
7. SharedContextPanel -> shared context visible?

**Validation:**
- War Room renders without errors
- Agent lanes show agent names + status
- Flow graph shows dependencies
- Real-time updates if agents active

**Decision Tree:**
```
War Room blank?
├─ No active agents -> expected, empty state
│  └─ PARTIAL: "War Room renders, no active agents to display"
├─ Component error -> console logs
└─ Tab not found -> search project panels

No real-time updates?
├─ WebSocket events not wired -> check agent.status events
├─ No active runs -> start an agentic conversation
└─ Events arrive but UI doesn't update -> rendering bug
```

---

### Phase 28: Sessions & Trajectory

**Goal:** Test session history and trajectory replay.

**API Endpoints Tested:**
- `GET /api/v1/projects/{id}/sessions`
- `GET /api/v1/sessions/{id}`
- `GET /api/v1/runs/{id}/trajectory`
- `GET /api/v1/runs/{id}/trajectory/export`
- `GET /api/v1/runs/{id}/checkpoints`

**Steps:**
1. Open Sessions tab/panel -> `browser_click`
2. `browser_snapshot` -> session list (past conversations/runs)
3. Click on a session -> `browser_click`
4. `browser_snapshot` -> session detail (status, cost, model, steps)
5. Open Trajectory tab -> `browser_click`
6. `browser_snapshot` -> step-by-step execution trace
7. Each step: tool call, result, timing
8. Export trajectory -> `browser_click` on export button
9. `browser_snapshot` -> download initiated or export view

**Validation:**
- Sessions list shows past runs
- Session detail includes metadata
- Trajectory shows execution steps
- Export produces JSON/CSV

**Decision Tree:**
```
No sessions?
├─ No completed runs yet -> run a conversation first (Phase 13+)
│  └─ SKIP if no runs available
├─ Sessions endpoint returns empty -> check API
└─ Tab missing -> search project panels

Trajectory empty?
├─ Simple (non-agentic) runs have no trajectory -> expected
│  └─ Only agentic runs have steps
├─ Trajectory recording disabled -> WARN
└─ Data exists but not rendering -> component error
```

---

### Phase 29: Agent Identity & Inbox

**Goal:** Test persistent agent profiles and messaging.

**API Endpoints Tested:**
- `GET /api/v1/agents/{id}/state`
- `PUT /api/v1/agents/{id}/state`
- `GET /api/v1/agents/{id}/inbox`
- `POST /api/v1/agents/{id}/inbox`

**Steps:**
1. Find agent list or agent detail view
2. `browser_snapshot` -> agent with fingerprint, stats?
3. Agent inbox -> `browser_click`
4. `browser_snapshot` -> inbox messages (empty or populated)
5. Send message to agent inbox -> `browser_click` on compose
6. `browser_snapshot` -> message sent?
7. Agent state -> accumulated stats (runs, cost, tokens)?

**Validation:**
- Agent identity persists across runs
- Inbox receives messages
- Stats accumulate

**Decision Tree:**
```
Agent identity not in UI?
├─ Feature API-only -> test via browser_evaluate
│  └─ PARTIAL: "Agent identity API works, no dedicated UI"
├─ Agent detail in War Room -> check there
└─ No agents created -> create one first

Inbox feature missing?
├─ API exists but no UI -> PARTIAL
└─ API 404 -> feature not enabled, SKIP
```

---

## INFRASTRUCTURE

### Phase 30: MCP Server Management

**Goal:** Test MCP server CRUD, connection test, tool discovery.

**API Endpoints Tested:**
- `GET/POST /api/v1/mcp/servers`
- `GET/PUT/DELETE /api/v1/mcp/servers/{id}`
- `POST /api/v1/mcp/servers/{id}/test`
- `GET /api/v1/mcp/servers/{id}/tools`

**Steps:**
1. Navigate to `/mcp` -> `browser_click` on "MCP" in sidebar
2. `browser_snapshot` -> MCP server list
3. "Add Server" -> `browser_click`
4. Fill: Name="test-server", URL="http://localhost:3001", Type="sse" -> `browser_fill_form`
5. Submit -> `browser_click`
6. `browser_snapshot` -> server in list?
7. "Test Connection" -> `browser_click`
8. `browser_wait_for` -> test result (success/failure)
9. `browser_snapshot` -> connection status
10. "Discover Tools" -> `browser_click`
11. `browser_snapshot` -> tool list for server?
12. Delete test server -> `browser_click`

**Validation:**
- MCP server CRUD works
- Connection test executes
- Tool discovery lists available tools
- Server assignment to projects works

**Decision Tree:**
```
/mcp page not found?
├─ Route different -> try /settings/mcp
├─ MCP disabled (config) -> SKIP
└─ Component error -> console

Connection test fails?
├─ No MCP server actually running -> expected
│  └─ PARTIAL: "CRUD works, no server to test against"
├─ Network error -> port/URL wrong
└─ Auth issue -> check server config

Tool discovery empty?
├─ Server not running -> expected
├─ Server has no tools -> unusual but valid
└─ Protocol mismatch -> WARN
```

---

### Phase 31: Knowledge Base & Retrieval

**Goal:** Test knowledge base CRUD, indexing, and search.

**API Endpoints Tested:**
- `GET/POST /api/v1/knowledge-bases`
- `GET/PUT/DELETE /api/v1/knowledge-bases/{id}`
- `POST /api/v1/knowledge-bases/{id}/index`
- `POST /api/v1/projects/{id}/search`

**Steps:**
1. Navigate to `/knowledge` -> `browser_click`
2. `browser_snapshot` -> knowledge base list
3. "Create KB" -> `browser_click`
4. Fill: Name="test-kb", Source="project files" -> `browser_fill_form`
5. Submit -> `browser_click`
6. `browser_snapshot` -> KB in list?
7. "Index" -> `browser_click` (trigger indexing)
8. `browser_wait_for` -> indexing completes
9. Search via KB -> test query
10. `browser_snapshot` -> search results?
11. Delete test KB

**Validation:**
- KB CRUD works
- Indexing triggers and completes
- Search returns results from indexed content

**Decision Tree:**
```
/knowledge page not found? -> try /settings/knowledge, SKIP
Indexing never completes? -> worker issue, check NATS
Search returns nothing? -> index not built, or content too small
```

---

### Phase 32: Channels & Threads

**Goal:** Test real-time channel messaging and threading.

**API Endpoints Tested:**
- `GET/POST /api/v1/channels`
- `GET/DELETE /api/v1/channels/{id}`
- `GET/POST /api/v1/channels/{id}/messages`
- `POST /api/v1/channels/{id}/messages/{mid}/thread`

**Steps:**
1. Find channels in sidebar -> `browser_snapshot`
2. ChannelList visible with existing channels?
3. Create channel: `browser_click` on "+"
4. Fill: Name="qa-test-channel" -> `browser_fill_form`
5. Submit -> `browser_click`
6. `browser_snapshot` -> channel created, ChannelView opens?
7. Send message in channel -> `browser_type` + send
8. `browser_snapshot` -> message appears?
9. Reply to message (thread) -> `browser_click` on reply icon
10. `browser_snapshot` -> ThreadPanel opens?
11. Send thread reply -> `browser_type` + send
12. `browser_snapshot` -> thread reply visible?

**Validation:**
- Channel CRUD works
- Messages send and appear in real-time
- Threading works (side panel)
- WebSocket events for live updates

**Decision Tree:**
```
Channels not in UI?
├─ Sidebar section collapsed -> expand
├─ Route /channels/:id exists -> navigate directly
├─ Feature not deployed -> API check, SKIP
└─ No channels yet -> create one

Thread panel doesn't open?
├─ Reply button not found -> UI layout
├─ ThreadPanel component error -> console
└─ Feature not implemented -> PARTIAL (messages work, threads don't)
```

---

### Phase 33: Policy Management

**Goal:** Test policy presets CRUD and evaluation.

**API Endpoints Tested:**
- `GET /api/v1/policies`
- `POST /api/v1/policies`
- `GET/DELETE /api/v1/policies/{name}`
- `POST /api/v1/policies/{name}/evaluate`

**Steps:**
1. Find policy management UI
2. `browser_snapshot` -> policy list (4 built-in presets)
3. View built-in preset details -> `browser_click`
4. `browser_snapshot` -> rules listed?
5. Create custom policy -> `browser_click`
6. Fill: Name="qa-test-policy", rules -> `browser_fill_form`
7. Submit -> `browser_click`
8. Test evaluation: simulate tool call against policy
9. Delete custom policy

**Validation:**
- 4 built-in presets visible (supervised-ask-all, etc.)
- Custom policy CRUD works
- Evaluation returns allow/deny/ask
- Built-in presets protected

**Decision Tree:**
```
Policy UI not found?
├─ Settings page -> check /settings
├─ API-only -> test via browser_evaluate
│  └─ PARTIAL: "Policy CRUD works via API"
└─ SKIP if no UI at all

Evaluation doesn't work?
├─ POST /policies/{name}/evaluate 404 -> endpoint missing
├─ Returns unexpected result -> rule parsing issue
└─ WARN and continue
```

---

### Phase 34: Prompt Editor

**Goal:** Test custom prompt section management.

**API Endpoints Tested:**
- `GET /api/v1/prompt-sections`
- `PUT /api/v1/prompt-sections`
- `DELETE /api/v1/prompt-sections/{id}`
- `POST /api/v1/prompt-sections/preview`

**Steps:**
1. Navigate to `/prompts` -> `browser_click`
2. `browser_snapshot` -> prompt section list
3. Edit a section -> `browser_click`
4. Modify content -> `browser_type`
5. Preview -> `browser_click`
6. `browser_snapshot` -> rendered preview?
7. Save changes -> `browser_click`
8. `browser_snapshot` -> saved?

**Validation:**
- Prompt sections listed
- Editing works
- Preview shows rendered template
- Changes persist

**Decision Tree:**
```
/prompts page not found? -> SKIP
Empty list? -> no custom sections yet, try creating one
Preview fails? -> template syntax error, WARN
```

---

## NOTIFICATIONS

### Phase 35: Notifications

**Goal:** Test notification bell, in-app center, tab badge, sounds.

**Steps:**
1. Find notification bell in header -> `browser_snapshot`
2. Click bell -> `browser_click` -> NotificationCenter opens?
3. `browser_snapshot` -> notification list (All/Unread/Archived tabs)
4. Generate notification: run agentic conversation (Phase 15+ already done)
5. Check bell badge updates -> `browser_snapshot`
6. Click notification -> navigates to source?
7. Archive notification -> `browser_click`
8. Tab badge: switch tabs, come back -> unread count in title?

**Validation:**
- Bell icon visible in header
- NotificationCenter opens with tabs
- New events create notifications
- Archive and mark-read work
- Tab badge shows unread count

**Decision Tree:**
```
Bell not visible? -> search header, sidebar, SKIP if missing
No notifications? -> AG-UI events not wired, or no recent activity
Tab badge not updating? -> document.title not modified, WARN
Audio not playing? -> autoplay policy, WARN (expected in test)
```

---

## ADMIN

### Phase 36: Settings & Preferences

**Goal:** Test settings page with keyboard shortcuts and preferences.

**Steps:**
1. Navigate to `/settings` -> `browser_click`
2. `browser_snapshot` -> settings page content
3. Keyboard shortcuts section visible?
4. `browser_snapshot` -> shortcut list
5. Toggle a preference (if available) -> `browser_click`
6. `browser_snapshot` -> preference saved?

**Validation:**
- Settings page renders
- Keyboard shortcuts listed
- Preferences toggleable

**Decision Tree:**
```
/settings blank? -> component error, console
No shortcuts? -> ShortcutsSection not rendering
Preferences don't save? -> API 500, check console
```

---

### Phase 37: Quarantine (Admin)

**Goal:** Test message quarantine review system.

**API Endpoints Tested:**
- `GET /api/v1/quarantine`
- `GET /api/v1/quarantine/stats`
- `GET /api/v1/quarantine/{id}`
- `POST /api/v1/quarantine/{id}/approve`
- `POST /api/v1/quarantine/{id}/reject`

**Steps:**
1. Find quarantine UI (admin panel) -> `browser_snapshot`
2. `browser_snapshot` -> quarantine list + stats
3. If items present: view item detail -> `browser_click`
4. `browser_snapshot` -> risk score, message content, sender
5. Approve item -> `browser_click`
6. `browser_snapshot` -> item resolved?
7. Stats update?

**Validation:**
- Quarantine list accessible (admin only)
- Stats show counts
- Approve/reject actions work
- Items move from pending to resolved

**Decision Tree:**
```
Quarantine not in UI?
├─ Admin-only feature -> verify admin role
├─ API test: GET /quarantine -> works? PARTIAL
└─ No quarantined items -> expected for clean system
   └─ PARTIAL: "Quarantine UI renders, no items to test"
```

---

### Phase 38: Boundaries & Contract Review

**Goal:** Test Phase 31 boundary detection and refactor approval.

**API Endpoints Tested:**
- `GET/PUT /api/v1/projects/{id}/boundaries`
- `POST /api/v1/projects/{id}/boundaries/analyze`
- `POST /api/v1/projects/{id}/review-refactor`

**Steps:**
1. Open project -> Boundaries tab -> `browser_click`
2. `browser_snapshot` -> BoundariesPanel visible?
3. "Analyze Boundaries" -> `browser_click`
4. `browser_wait_for` -> analysis completes
5. `browser_snapshot` -> detected boundaries (API, data, inter-service)?
6. Trigger review-refactor pipeline -> `browser_click`
7. RefactorApproval overlay appears? -> `browser_snapshot`
8. Approve/reject refactoring suggestion

**Validation:**
- Boundary analysis detects service boundaries
- Review-refactor pipeline triggers
- HITL approval overlay for refactoring

**Decision Tree:**
```
Boundaries tab missing? -> Phase 31 not deployed, SKIP
Analysis returns nothing? -> project too small, PARTIAL
RefactorApproval not showing? -> pipeline didn't trigger HITL step
Pipeline timeout? -> LLM issue, WARN
```

---

### Phase 39: Audit Trail

**Goal:** Test global and project audit log.

**API Endpoints Tested:**
- `GET /api/v1/audit`
- `GET /api/v1/projects/{id}/audit`

**Steps:**
1. Navigate to `/activity` -> `browser_click`
2. `browser_snapshot` -> audit table with entries
3. Filter by action type -> `browser_click` on filter
4. `browser_snapshot` -> filtered results
5. Open project audit -> Audit tab in project detail
6. `browser_snapshot` -> project-specific actions
7. Verify entries from earlier phases (project creation, conversations, etc.)

**Validation:**
- Audit trail shows recent actions
- Timestamps, actors, resources visible
- Filtering works
- Project audit scoped to project

**Decision Tree:**
```
/activity page blank?
├─ No actions recorded yet -> run earlier phases first
├─ AuditTable component error -> console
└─ Route different -> search navigation

No entries?
├─ Audit recording disabled -> WARN
├─ Entries exist but pagination -> scroll/paginate
└─ Tenant isolation -> correct tenant?
```

---

## DEV-MODE

### Phase 40: Benchmarks

**Goal:** Test benchmark system (dev mode only).

**Prerequisite:** `APP_ENV=development` (checked in Phase 0).

**API Endpoints Tested:**
- `GET/POST /api/v1/benchmarks/suites`
- `GET/POST /api/v1/benchmarks/runs`
- `GET /api/v1/benchmarks/runs/{id}`
- `GET /api/v1/benchmarks/datasets`
- `GET /api/v1/benchmarks/leaderboard`

**Steps:**
1. Navigate to `/benchmarks` -> `browser_click`
2. `browser_snapshot` -> benchmark UI visible?
3. Suite list -> existing suites?
4. "Create Suite" -> `browser_click`
5. Fill: Name="qa-test", Dataset="basic-coding" -> `browser_fill_form`
6. Submit -> `browser_click`
7. `browser_snapshot` -> suite created?
8. "Run" -> `browser_click`
9. `browser_wait_for` -> run starts, progress visible
10. `browser_snapshot` -> running state?
11. Leaderboard tab -> `browser_click`
12. `browser_snapshot` -> leaderboard with past results?

**Validation:**
- Benchmark UI accessible in dev mode
- Suite CRUD works
- Run can be started
- Progress visible via WebSocket
- Leaderboard shows results

**Decision Tree:**
```
/benchmarks 403?
├─ APP_ENV != development -> ABORT: "Dev mode required"
├─ devMode was false in Phase 0 -> already flagged
└─ SKIP phase

No datasets?
├─ GET /benchmarks/datasets returns empty -> no built-in datasets
├─ Dataset dir not found -> path resolution issue
└─ WARN

Run doesn't start?
├─ Worker not running -> ABORT
├─ No model available -> can't run without LLM
└─ Budget/timeout -> WARN
```

---

## REPORT

### Phase 41: Report, Screenshots & Cleanup

**Goal:** Collect results, generate report, clean up.

**Steps:**
1. Collect all phase results: PASS / PARTIAL / SKIP / FAIL
2. `browser_take_screenshot` -> final dashboard state
3. `browser_navigate` to each main route, `browser_take_screenshot` for each
4. Generate summary report
5. Delete test project (optional): `browser_evaluate` or via UI
6. Clean up test conversations, channels, etc.

**Report Format:**
```
## QA Test Report — [Date]

### Environment
- Frontend: http://localhost:3000
- Backend: http://localhost:8080
- LiteLLM: http://localhost:4000
- Models: [list]
- Dev Mode: true/false

### Results Summary
| # | Phase | Group | Status | Notes |
|---|-------|-------|--------|-------|
| 0 | Environment Discovery | Foundation | PASS | 3 models, gpt-4o tool-capable |
| 1 | Auth & Login | Foundation | PASS | admin@localhost |
| ... | ... | ... | ... | ... |

### Statistics
- Total Phases: 42
- PASS: X
- PARTIAL: X
- SKIP: X
- FAIL: X
- Coverage: X%

### Decision Tree Activations
- Phase 13: "No response 30s" -> model timeout -> switched model -> RESOLVED
- Phase 15: "Model doesn't call tools" -> explicit prompt -> RESOLVED

### Bugs Found
- [BUG-001] Description + screenshot
- [WARN-001] Description

### Feature Gaps
- Features in code but no UI: [list]
- Features in docs but not implemented: [list]

### Total LLM Cost: $X.XX
```

---

## API Endpoint Coverage Map

### Fully Tested via UI (interactive)
- Auth (login, logout, me, api-keys)
- Projects (CRUD, clone, workspace)
- Conversations (CRUD, send, stop, compact, clear, mode, model, fork, rewind)
- Files (tree, content, write, rename, delete)
- Git (status, branches, pull, checkout)
- LLM (models, discover, health, keys)
- Modes (CRUD, scenarios, tools)
- Policies (CRUD, evaluate, allow-always)
- MCP (servers CRUD, test, tools)
- Channels (CRUD, messages, threads)
- Knowledge (CRUD, index)
- Goals (CRUD, detect, AI discover)
- Roadmap (CRUD, milestones, features, detect, import)
- Benchmarks (suites, runs, datasets, leaderboard)
- Dashboard (stats, charts)
- Costs (global, project, by-model, by-tool, daily)
- Runs (approve, revert, trajectory, checkpoints)
- Search (conversations, code)
- Sessions (list, detail)
- Notifications (bell, center)
- Settings (read, update)
- Quarantine (list, stats, approve, reject)
- Boundaries (CRUD, analyze)
- Audit (global, project)
- Prompt sections (CRUD, preview)

### API-Only (no UI interaction in plan, tested via browser_evaluate if needed)
- Webhooks (VCS, PM)
- A2A protocol
- Copilot token exchange
- Memories & experience pool
- Microagents
- Skills
- Auto-agent
- Routing stats
- Review policies
- Prompt evolution
- Retrieval scopes
- LSP
- Human feedback
- Feature decomposition/planning via API

---

## Execution Notes

- **Estimated Duration:** 2-4 hours for full 42-phase run (depends on LLM speed)
- **Minimum Viable Run:** Phase 0-3 (Foundation) + Phase 12-13 (Chat basics) = ~15 min
- **Cost Estimate:** $1-5 depending on model and number of agentic interactions
- **Parallelization:** Groups can run in parallel if multiple Claude Code sessions available
- **Retry Policy:** Each phase can be retried independently. Decision trees guide recovery.
