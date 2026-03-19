# Autonomous Goal-to-Program Test Plan — Claude Code + Playwright-MCP

**Date:** 2026-03-19
**Type:** Interactive AI QA Test (Claude Code drives browser via playwright-mcp)
**Scope:** Core autonomous workflow: Chat -> Goals -> Roadmap -> Execution -> Working Program
**Coverage:** 9 phases, sequential, real LLM calls (no mocks)

---

## How to Use This Document

This is a **runbook for Claude Code sessions** using playwright-mcp tools. Claude Code drives the CodeForge frontend interactively, observes results via `browser_snapshot`, and adapts via Decision Trees when steps fail.

**Prerequisites:**
- Playwright-MCP connected (`/mcp` shows playwright-mcp)
- At least one LLM API key configured (ANTHROPIC_API_KEY, OPENAI_API_KEY, etc.)
- **Claude Code is responsible for starting ALL required services** — see Phase 0

**Execution:**
- Follow phases 0-8 sequentially (strict order — each gates the next)
- Each phase: execute Steps, check Validation, follow Decision Tree on failure
- Record results as PASS / PARTIAL / SKIP / FAIL per phase
- Save report to `docs/testing/YYYY-MM-DD-autonomous-goal-to-program-report.md`

**State Variables** (carried across phases):
```
SCENARIO = "S2"   # Selected scenario (S1/S2/S3/S4) — default S2 for first run
ENV = {}          # Phase 0: models, devMode, token
PROJECT_ID = ""   # Phase 1: created project UUID
WORKSPACE = ""    # Phase 1: project workspace path
BRANCH = ""       # Phase 1: test branch name
GOAL_COUNT = 0    # Phase 2-3: detected goals
ROADMAP_OK = false # Phase 4: roadmap created
EXEC_STARTED = false # Phase 5-6: execution dispatched
TOOL_CALLS = {}   # Phase 6-7: tool call counts by type
```

---

## Phase Dependency Graph

```
Phase 0: Environment Discovery & Login
    |
    v
Phase 1: Project Setup (create project via UI, adopt/clone repo)
    |
    v
Phase 2: Goal Discovery (AI Discover via chat)
    |
    v
Phase 3: Goal Validation (verify goals in GoalsPanel)
    |
    v
Phase 4: Roadmap Creation (build roadmap + milestones + features)
    |
    v
Phase 5: Autonomous Execution (agentic chat executes roadmap)
    |    \
    |     v
    |   Phase 5b: Blocker & HITL Handling
    |     |
    v     v
Phase 6: Execution Monitoring (poll progress, check commits)
    |
    v
Phase 7: Program Validation (run program in workspace)
    |
    v
Phase 8: Report & Cleanup
```

---

## Test Scenarios — Difficulty Levels

Each test run picks **one scenario**. Scenarios are ordered by difficulty and exercise progressively more tool calls. Run the easiest first to validate the pipeline, then increase difficulty.

### Scenario Selection

| ID | Difficulty | Language | Primary Tool Calls | Description |
|----|-----------|----------|-------------------|-------------|
| S1 | Easy | Python | Write, Bash | Greenfield: single-file script from scratch |
| S2 | Medium | Python | Write, Read, Bash, Glob, Search | Greenfield: multi-module CLI app with tests |
| S3 | Hard | Python | Read, Edit, Search, Glob, Bash, ListDir | Brownfield: extend existing codebase with new feature |
| S4 | Expert | TypeScript | Write, Read, Edit, Bash, Search, Glob, ListDir | Full-stack: REST API + tests + config + build |

### Tool Call Coverage Matrix

| Tool | S1 Easy | S2 Medium | S3 Hard | S4 Expert |
|------|---------|-----------|---------|-----------|
| **Write** | Heavy (create all files) | Heavy (create modules) | Light (new files only) | Heavy (new + config) |
| **Read** | None | Light (check existing) | Heavy (understand codebase) | Heavy (understand + validate) |
| **Edit** | None | None | Heavy (modify existing code) | Heavy (refactor + extend) |
| **Bash** | Light (run script) | Medium (pytest, pip) | Heavy (tests, lint, git) | Heavy (npm, build, test, lint) |
| **Search** | None | Light (find patterns) | Heavy (find implementation points) | Heavy (trace dependencies) |
| **Glob** | None | Light (find test files) | Medium (find related files) | Heavy (find configs, types) |
| **ListDir** | None | Light (check structure) | Medium (navigate codebase) | Medium (explore structure) |
| **propose_goal** | 3-4 goals | 5-7 goals | 6-8 goals | 8-12 goals |

---

### S1: Easy — CSV-to-JSON Converter (Greenfield, Single File)

**Chat Prompt:**
> "I want a simple Python script called csv2json.py that converts CSV files to JSON. It should read a CSV file path from the command line, parse it, and output a JSON file with the same name but .json extension. Include error handling for missing files and invalid CSV. Add a small test file test_csv2json.py that tests with a sample CSV."

**Expected Goals:** 3-4
- File conversion logic
- CLI argument parsing
- Error handling
- Basic tests

**Expected Roadmap:** 2 milestones, 5-7 features
- Milestone 1: Core script (argparse, csv parsing, json output)
- Milestone 2: Error handling + tests

**Primary Tool Calls:** Write (create 2-3 files), Bash (run script, run test)

**Validation Commands:**
```
python csv2json.py sample.csv        # -> produces sample.json
python csv2json.py nonexistent.csv   # -> error message, no crash
python -m pytest test_csv2json.py -v # -> tests pass
```

**Success Criteria:**
- `csv2json.py` exists and runs without errors
- Converts a valid CSV to correct JSON
- Handles missing file gracefully
- Test file exists and passes

---

### S2: Medium — CLI Task Manager (Greenfield, Multi-Module)

**Chat Prompt:**
> "I want a Python CLI task manager. It should support: adding tasks with a title and optional priority (low/medium/high), listing all tasks filtered by status (open/done), completing a task by ID, deleting a task by ID, persistent storage in a JSON file, a simple test suite that validates all commands, and a README with usage instructions."

**Expected Goals:** 5-7
- CRUD operations (add, list, complete, delete)
- Data model (Task with ID, title, priority, status)
- JSON persistence
- CLI interface (argparse)
- Test suite
- Documentation

**Expected Roadmap:** 6 milestones, 15-20 features
- Milestone 1: Project Setup (pyproject.toml, package structure)
- Milestone 2: Core Data Model (dataclass, enums, serialization)
- Milestone 3: Storage Layer (JSON load/save, atomic writes)
- Milestone 4: CLI Interface (argparse subcommands)
- Milestone 5: Test Suite (pytest, fixtures, all commands)
- Milestone 6: Documentation (README)

**Primary Tool Calls:** Write (create 8-12 files), Read (verify structure), Bash (pytest, pip install), Glob (find test files), Search (check imports)

**Validation Commands:**
```
python -m task_manager --help           # -> shows usage
python -m task_manager add --title "Buy groceries" --priority high
python -m task_manager list             # -> shows task
python -m task_manager complete 1       # -> marks done
python -m task_manager delete 1         # -> removes task
python -m pytest -v                     # -> all tests pass
```

**Success Criteria:**
- `python -m task_manager --help` exits 0
- Full CRUD cycle works (add -> list -> complete -> delete)
- `python -m pytest` exits 0
- JSON file created and valid

---

### S3: Hard — Add Feature to Existing Codebase (Brownfield)

**Precondition:** The TestRepo must contain an existing Python project. If the TestRepo is empty or not suitable, use Phase 1 to seed it with the S2 output from a previous run, OR create a starter codebase via `browser_evaluate`:

```js
// Seed starter codebase via API — creates a basic Flask app with existing routes
// This gives the agent an existing codebase to READ, UNDERSTAND, and EXTEND
```

**Chat Prompt:**
> "This project has an existing Python application. I want you to: 1) Explore the codebase to understand the current architecture, 2) Add a new 'tags' feature — each task should support multiple string tags, 3) Add a CLI command 'search' that finds tasks by tag, 4) Update the JSON storage to include tags, 5) Migrate existing data (tasks without tags get an empty list), 6) Add tests for the new feature, 7) Update the README to document the tags feature. Do NOT break existing functionality — all existing tests must still pass after your changes."

**Expected Goals:** 6-8
- Codebase exploration / understanding
- Data model extension (tags field)
- New CLI command (search by tag)
- Storage migration (backward compatibility)
- Add tag to existing commands (--tags on add)
- Test coverage for new feature
- Documentation update
- Regression safety (existing tests pass)

**Expected Roadmap:** 5 milestones, 12-18 features
- Milestone 1: Codebase Analysis (read files, understand architecture)
- Milestone 2: Data Model Extension (add tags to Task, migration)
- Milestone 3: CLI Extension (--tags on add, search command)
- Milestone 4: Storage Update (backward-compatible JSON)
- Milestone 5: Tests + Docs (new tests, update README)

**Primary Tool Calls:**
- **Read** — Heavy: agent must read existing models.py, cli.py, storage.py, tests
- **Search** — Heavy: find where Task is defined, where CLI commands are registered, where JSON is parsed
- **Glob** — Medium: find all test files, find all Python files
- **ListDir** — Medium: understand project structure
- **Edit** — Heavy: modify existing files (add tags field, update CLI, extend storage)
- **Write** — Light: new test files, possibly new modules
- **Bash** — Heavy: run existing tests first (regression), run new tests, check lint

**Validation Commands:**
```
python -m pytest -v                              # -> ALL tests pass (old + new)
python -m task_manager add --title "Work" --tags "urgent,office"
python -m task_manager search --tag "urgent"     # -> finds the task
python -m task_manager list                      # -> tags visible in output
cat tasks.json | python -m json.tool             # -> tags field present
```

**Success Criteria:**
- All **existing** tests still pass (no regression)
- New tag-related tests pass
- `search --tag` command works
- Tasks can be created with tags
- Existing tasks without tags don't crash (migration)
- README updated with tags documentation

---

### S4: Expert — REST API with Tests and Build Pipeline (TypeScript)

**Chat Prompt:**
> "Build a REST API for a bookmark manager using TypeScript and Node.js (no framework — use native http module or Express if needed). It should have: 1) CRUD endpoints for bookmarks (title, url, tags, created_at), 2) Validation (URL format, required fields), 3) In-memory storage with optional JSON file persistence, 4) A test suite using the built-in Node.js test runner or vitest, 5) TypeScript strict mode with proper types (no 'any'), 6) A package.json with build and test scripts, 7) A tsconfig.json configured for ES modules, 8) Error handling with proper HTTP status codes, 9) A README with API documentation and curl examples."

**Expected Goals:** 8-12
- REST API design (routes, methods, status codes)
- Bookmark data model (TypeScript interfaces)
- URL validation
- CRUD operations
- In-memory + file persistence
- TypeScript configuration (strict, ESM)
- Build pipeline (compile, run, test)
- Test suite
- Error handling
- API documentation

**Expected Roadmap:** 7-8 milestones, 20-30 features
- Milestone 1: Project Setup (package.json, tsconfig.json, directory structure)
- Milestone 2: Types & Models (Bookmark interface, validation types, error types)
- Milestone 3: Storage Layer (in-memory store, JSON persistence, interface)
- Milestone 4: HTTP Server (routing, request parsing, response helpers)
- Milestone 5: CRUD Endpoints (GET/POST/PUT/DELETE /bookmarks)
- Milestone 6: Validation & Error Handling (URL validation, field validation, error responses)
- Milestone 7: Test Suite (API tests, model tests, storage tests)
- Milestone 8: Documentation (README with curl examples)

**Primary Tool Calls:**
- **Write** — Heavy: create 15-25 files (src/, tests/, configs)
- **Read** — Heavy: verify generated code, check imports
- **Edit** — Heavy: iterative fixes after test runs fail
- **Bash** — Heavy: npm init, npm install, tsc (compile), npm test, npm run build, curl tests
- **Search** — Heavy: find type definitions, trace imports, check error handling
- **Glob** — Heavy: find all .ts files, find config files, find test files
- **ListDir** — Medium: verify directory structure

**Validation Commands:**
```
npm run build                                    # -> TypeScript compiles without errors
npm test                                         # -> test suite passes
npm start &                                      # -> server starts on port 3000
curl -X POST http://localhost:3000/bookmarks \
  -H "Content-Type: application/json" \
  -d '{"title":"GitHub","url":"https://github.com","tags":["dev"]}'
                                                 # -> 201 Created
curl http://localhost:3000/bookmarks             # -> returns bookmark list
curl http://localhost:3000/bookmarks/1           # -> returns single bookmark
curl -X DELETE http://localhost:3000/bookmarks/1 # -> 204 No Content
curl -X POST http://localhost:3000/bookmarks \
  -d '{"title":"Bad"}'                           # -> 400 Bad Request (missing url)
```

**Success Criteria:**
- `npm run build` exits 0 (TypeScript compiles cleanly)
- `npm test` exits 0
- Server starts and responds to HTTP requests
- CRUD cycle works (POST -> GET -> PUT -> DELETE)
- Invalid requests return proper 4xx errors
- No `any` types in source code (strict mode)

---

### Scenario Selection Logic

Claude Code selects the scenario based on test run parameters:

```
If this is the FIRST test run:
  -> Use S1 (Easy) to validate the pipeline works at all

If S1 passed on a previous run:
  -> Use S2 (Medium) as the primary validation scenario

If S2 passed AND TestRepo has existing code:
  -> Use S3 (Hard) to test brownfield capabilities

If S2 passed AND want to test TypeScript/multi-language:
  -> Use S4 (Expert)

For regression testing (scheduled/cron):
  -> Rotate: S1 -> S2 -> S3 -> S4 -> S1 -> ...
```

**The scenario ID (S1-S4) must be recorded in the test report.**

---

## Phase 0: Service Startup & Environment Discovery & Login

**Goal:** Ensure ALL required services are running, then login and discover models.

**CRITICAL:** Claude Code MUST start and verify every service. Do NOT assume anything is already running. Check each service and start it if missing.

### Step 0a: Start Infrastructure Services (Docker)

1. **Check and start Docker services:**
   ```bash
   docker compose ps --format '{{.Name}}: {{.State}}' 2>/dev/null
   ```

2. **Start missing services:**
   ```bash
   docker compose up -d postgres nats litellm
   ```

3. **Wait for healthy state:**
   ```bash
   # Postgres
   docker compose exec postgres pg_isready -U codeforge
   # NATS (check monitoring port)
   curl -s http://localhost:8222/varz | head -1
   # LiteLLM
   curl -s http://localhost:4000/health
   ```

4. **Find NATS container IP** (needed for WSL2 — localhost:4222 may not work):
   ```bash
   docker inspect codeforge-nats | grep -m1 '"IPAddress"' | grep -oP '[\d.]+'
   ```
   -> Store as `NATS_IP`

### Step 0b: Start Go Backend

1. **Check if backend is running:**
   ```bash
   curl -s http://localhost:8080/health 2>/dev/null || echo "NOT_RUNNING"
   ```

2. **If not running — start in background:**
   ```bash
   cd /workspaces/CodeForge && APP_ENV=development go run ./cmd/codeforge/ > /tmp/codeforge-backend.log 2>&1 &
   ```

3. **Wait for backend ready (poll up to 60s):**
   ```bash
   for i in $(seq 1 30); do
     curl -s http://localhost:8080/health | grep -q '"ok"' && echo "READY" && break
     sleep 2
   done
   ```
   -> Expected: `READY`

### Step 0c: Start Frontend Dev Server

1. **Check if frontend is running:**
   ```bash
   curl -s -o /dev/null -w "%{http_code}" http://localhost:3000 2>/dev/null || echo "NOT_RUNNING"
   ```

2. **If not running — start in background:**
   ```bash
   cd /workspaces/CodeForge/frontend && npm run dev > /tmp/codeforge-frontend.log 2>&1 &
   ```

3. **Wait for frontend ready (poll up to 30s):**
   ```bash
   for i in $(seq 1 15); do
     curl -s -o /dev/null -w "%{http_code}" http://localhost:3000 2>/dev/null | grep -q "200" && echo "READY" && break
     sleep 2
   done
   ```

### Step 0d: Start Python Worker

1. **Check if worker is running:**
   ```bash
   pgrep -f "codeforge.consumer" > /dev/null && echo "RUNNING" || echo "NOT_RUNNING"
   ```

2. **If not running — start in background:**
   ```bash
   cd /workspaces/CodeForge && \
     NATS_URL="nats://${NATS_IP}:4222" \
     APP_ENV=development \
     .venv/bin/python -m codeforge.consumer > /tmp/codeforge-worker.log 2>&1 &
   ```

3. **Wait for worker ready (check log for startup message):**
   ```bash
   for i in $(seq 1 15); do
     grep -q "Consumer started\|connected\|listening" /tmp/codeforge-worker.log 2>/dev/null && echo "READY" && break
     sleep 2
   done
   ```

### Step 0e: Verify All Services & Login via Browser

1. `browser_navigate` -> `http://localhost:3000`
2. `browser_snapshot` -> login page visible? Look for email/password fields

3. Login with seeded admin:
   - `browser_fill_form` -> email: `admin@localhost`, password: `Changeme123`
   - `browser_click` -> Login/Submit button
   - `browser_snapshot` -> Dashboard page visible? (project cards or empty state)

4. Store auth token for API calls:
   ```js
   // browser_evaluate
   fetch('/api/v1/auth/login', {
     method: 'POST',
     headers: {'Content-Type': 'application/json'},
     body: JSON.stringify({email: 'admin@localhost', password: 'Changeme123'})
   }).then(r => r.json()).then(d => d.token)
   ```
   -> Store as `ENV.token`

5. Model discovery:
   ```js
   // browser_evaluate
   fetch('/api/v1/llm/discover', {
     method: 'POST',
     headers: {'Authorization': 'Bearer ' + ENV.token}
   }).then(r => r.json())
   ```
   -> Store `ENV.models`, identify tool-capable models

**Validation:**
- All 6 services running: postgres, nats, litellm, go backend, frontend, python worker
- Frontend renders (not blank, no JS errors)
- Backend responds with `dev_mode: true`
- Login succeeds, dashboard visible
- At least 1 model available

**Decision Tree:**
```
Docker services not starting?
├─ Docker daemon not running -> ABORT: "Start Docker first"
├─ Port conflict -> docker compose down, then up again
└─ postgres unhealthy -> Check docker compose logs postgres

Go backend fails to start?
├─ Port 8080 in use -> kill existing process: lsof -ti:8080 | xargs kill
├─ Migration errors -> Check /tmp/codeforge-backend.log
├─ Missing env vars -> Ensure APP_ENV=development
└─ Build errors -> go build ./cmd/codeforge/ first to check

Frontend fails to start?
├─ Port 3000 in use -> kill existing: lsof -ti:3000 | xargs kill
├─ node_modules missing -> cd frontend && npm install
└─ Build errors -> Check /tmp/codeforge-frontend.log

Worker fails to start?
├─ NATS unreachable -> Find correct NATS container IP (WSL2 issue)
│  docker inspect codeforge-nats | grep IPAddress
├─ .venv not found -> cd /workspaces/CodeForge && poetry install
├─ Import errors -> Check /tmp/codeforge-worker.log
└─ bytecache stale -> find workers -name "*.pyc" -delete

Frontend unreachable after all services started?
├─ Timeout -> Check /tmp/codeforge-frontend.log
├─ Blank page -> browser_console_messages for JS errors
└─ Error page -> browser_take_screenshot + check backend logs

Login fails?
├─ "Invalid credentials" -> Check DB seeding (admin@localhost / Changeme123)
├─ 502/503 -> Backend crashed -> Check /tmp/codeforge-backend.log
└─ Stuck on login page -> browser_console_messages, check CORS

No models found?
├─ Check LiteLLM: browser_evaluate fetch('http://localhost:4000/health')
│  ├─ Down -> ABORT: "LiteLLM proxy not started"
│  └─ Up but 0 models -> WARN: "No API keys configured"
└─ Models but none tool-capable ->
   FLAG: "Simple chat only, agentic execution will be limited"
```

---

## Phase 1: Project Setup

**Goal:** Create a new project pointing to TestRepo (or local workspace), create test branch.

**Steps:**

1. From Dashboard, `browser_click` -> "New Project" button (primary button in header)
2. `browser_snapshot` -> CreateProjectModal visible? (tabs: Remote / Local / Empty)

3. **Try Remote first:**
   - `browser_click` -> "Remote" tab (if not already selected)
   - `browser_fill_form`:
     - Project Name: `Autonomous Test - {timestamp}`
     - Repository URL: `https://github.com/Strob0t/TestRepo.git`
   - `browser_snapshot` -> Check if URL is parsed (branch dropdown should populate)
   - `browser_click` -> "Create" button
   - `browser_wait_for` -> Project detail page loads (URL changes to `/projects/:id`)

4. **If Remote fails** (clone error, GitHub unreachable):
   - `browser_click` -> close modal, re-open "New Project"
   - `browser_click` -> "Local" tab
   - First, create a local workspace via `browser_evaluate`:
     ```js
     await fetch('/api/v1/projects', {
       method: 'POST',
       headers: {
         'Authorization': 'Bearer ' + ENV.token,
         'Content-Type': 'application/json'
       },
       body: JSON.stringify({
         name: 'Autonomous Test - local',
         description: 'Automated test: goal-to-program pipeline'
       })
     }).then(r => r.json())
     ```
   - Navigate to the created project

5. `browser_snapshot` -> ProjectDetailPage visible? (header with project name, left panels, chat panel)

6. Extract project info via `browser_evaluate`:
   ```js
   const url = window.location.pathname;
   const projectId = url.split('/projects/')[1];
   const project = await fetch('/api/v1/projects/' + projectId, {
     headers: {'Authorization': 'Bearer ' + ENV.token}
   }).then(r => r.json());
   [project.id, project.workspace_path]
   ```
   -> Store `PROJECT_ID`, `WORKSPACE`

7. Create test branch (via API since UI may not have branch creation):
   ```js
   const branch = 'test/autonomous-' + new Date().toISOString().replace(/[:.]/g, '-').slice(0, 19);
   await fetch('/api/v1/projects/' + PROJECT_ID + '/git/checkout', {
     method: 'POST',
     headers: {
       'Authorization': 'Bearer ' + ENV.token,
       'Content-Type': 'application/json'
     },
     body: JSON.stringify({branch: branch, create: true})
   });
   branch
   ```
   -> Store `BRANCH`

8. `browser_snapshot` -> Verify git status badge in header shows new branch name

**Validation:**
- Project exists with valid `workspace_path`
- ProjectDetailPage renders (left panels + chat visible)
- Git status badge shows test branch
- `PROJECT_ID` and `WORKSPACE` are non-empty

**Decision Tree:**
```
Clone fails?
├─ "repository not found" -> Fallback to Local tab
├─ Timeout -> Check GitHub accessibility, fallback to local
└─ "permission denied" -> Fallback to local

Project page not loading?
├─ Spinner stuck -> browser_evaluate check API response
├─ Error banner -> browser_snapshot + read error text
└─ 404 -> Project creation may have failed silently, check browser_console_messages

No workspace_path?
├─ Remote clone still in progress -> browser_wait_for (up to 60s)
├─ Clone failed silently -> Check project status via API
└─ ABORT if workspace can't be established (agentic mode requires it)
```

---

## Phase 2: Goal Discovery via Chat

**Goal:** Use AI Goal Discovery to detect project goals from natural language.

**Steps:**

1. **Open Goals panel:**
   - `browser_click` -> "More panels..." dropdown in left panel
   - `browser_click` -> "Goals" option
   - `browser_snapshot` -> GoalsPanel visible? (empty state or existing goals)

2. **Trigger AI Discovery:**
   - `browser_click` -> "AI Discover" button in GoalsPanel header
   - `browser_snapshot` -> Should switch to chat panel with a new conversation in `goal-researcher` mode
   - `browser_wait_for` -> Chat input area visible, possibly with initial agent message

3. **Send program description (from selected scenario):**
   - `browser_click` -> Chat input textarea
   - `browser_type` -> the **Chat Prompt** from the selected scenario (S1/S2/S3/S4)
     - S1: CSV-to-JSON converter prompt
     - S2: CLI task manager prompt
     - S3: Add-tags-feature prompt (requires existing codebase)
     - S4: TypeScript REST API prompt
   - `browser_click` -> Send button
   - `browser_snapshot` -> User message appears in chat

4. **Wait for agent response:**
   - `browser_wait_for` -> Assistant message appears (may take 30-120s depending on model)
   - Poll with `browser_snapshot` every 15s, up to 5 minutes
   - Look for: streaming text, tool call indicators, GoalProposalCards

5. **If agent asks clarifying questions** (assistant message contains `?` but no GoalProposalCards):
   - `browser_click` -> Chat input
   - `browser_type` -> Scenario-specific clarification:
     - **S1:** `Yes, just a single Python script with argparse. No dependencies beyond the standard library. Keep it simple.`
     - **S2:** `Yes, that covers everything. Please create the goals now. Simple Python CLI app with argparse, JSON file storage, and pytest tests. No web framework, no database, just files.`
     - **S3:** `Yes, extend the existing codebase. Read it first, understand the patterns, then add the tags feature following the same code style. All existing tests must still pass.`
     - **S4:** `Yes, TypeScript with strict mode, no 'any' types. Use Express or native http. Vitest or Node.js test runner. ES modules. Keep dependencies minimal.`
   - `browser_click` -> Send button
   - `browser_wait_for` -> Next agent response (up to 3 minutes)

6. **Handle GoalProposalCards:**
   - `browser_snapshot` -> Look for GoalProposalCard components in chat (bordered cards with Approve/Reject buttons)
   - For each GoalProposalCard visible:
     - `browser_snapshot` -> Read goal title, kind, content from the card
     - `browser_click` -> "Approve" button on the card
     - `browser_snapshot` -> Card changes to approved state (green border, checkmark)
   - Repeat until all proposal cards are approved

7. **Verify goals created via GoalsPanel:**
   - `browser_click` -> "More panels..." dropdown
   - `browser_click` -> "Goals"
   - `browser_snapshot` -> GoalsPanel should show the approved goals
   - Count visible goal items

**Validation:**
- Agent responded with at least one message
- At least 3 GoalProposalCards appeared
- All proposals approved (cards show green/approved state)
- GoalsPanel shows >= 3 goals with toggle ON (enabled)
- Goals cover key concepts: CLI, tasks, JSON, tests

**Decision Tree:**
```
Agent never responds?
├─ browser_console_messages -> check for WS errors
├─ browser_evaluate: check conversation messages via API
│  fetch('/api/v1/conversations/' + convId + '/messages', {headers:...})
├─ Check: is worker running? (agent loop needs Python worker)
└─ ABORT after 5 min: "Agent unresponsive, check worker logs"

Agent responds but no GoalProposalCards?
├─ Agent in interview phase (GSD methodology) -> Send clarification (step 5)
├─ Agent proposed goals via text (not tool) -> Create goals manually via GoalsPanel
│  - browser_click -> "+ Add" in GoalsPanel
│  - browser_fill_form -> kind, title, content from agent's text
│  - browser_click -> Create
└─ propose_goal tool not registered -> FLAG as implementation bug

Agent stuck / infinite loop?
├─ Check: ActiveWorkPanel shows running task?
├─ browser_click -> Stop button (red) in chat header if visible
└─ Wait for stall detector (auto-aborts after 2 escape attempts)

GoalProposalCard Approve button doesn't work?
├─ browser_console_messages -> check for API errors
├─ browser_evaluate: manually create goal via API as fallback
└─ FLAG: "GoalProposalCard approval broken"
```

### Phase 2b: Manual Goal Creation (Fallback)

If AI Discovery fails completely, create goals manually via the GoalsPanel UI.

For each goal: `browser_click` -> "+ Add" -> `browser_fill_form` (kind, title, content) -> `browser_click` "Create"

**S1 Fallback Goals:**

| Kind | Title | Content |
|------|-------|---------|
| requirement | CSV to JSON Conversion | Python script that reads CSV file and outputs JSON file |
| requirement | CLI Arguments | Accept file path via argparse command line argument |
| requirement | Error Handling | Handle missing files and invalid CSV gracefully |

**S2 Fallback Goals:**

| Kind | Title | Content |
|------|-------|---------|
| requirement | CLI Task CRUD | Python CLI with argparse: add, list, complete, delete tasks. Each task has ID, title, priority (low/medium/high), status (open/done). |
| requirement | JSON Persistence | Store tasks in a tasks.json file. Load on startup, save after each mutation. |
| requirement | Test Suite | Pytest test suite covering add, list, complete, delete operations. Use tmp_path fixture for isolated tests. |
| requirement | README Documentation | README.md with installation instructions, usage examples for each command, and project structure. |

**S3 Fallback Goals:**

| Kind | Title | Content |
|------|-------|---------|
| requirement | Codebase Understanding | Read and understand the existing Python project architecture |
| requirement | Tags Data Model | Extend Task model with optional tags field (list of strings) |
| requirement | Search Command | Add CLI search command that filters tasks by tag |
| requirement | Data Migration | Existing tasks without tags should get empty list, no crashes |
| requirement | Regression Safety | All existing tests must still pass after changes |
| requirement | Documentation Update | Update README to document the new tags feature |

**S4 Fallback Goals:**

| Kind | Title | Content |
|------|-------|---------|
| requirement | REST API Endpoints | CRUD endpoints for bookmarks (GET, POST, PUT, DELETE) |
| requirement | TypeScript Types | Strict TypeScript interfaces for Bookmark, no any types |
| requirement | Validation | URL format validation, required field checks, proper HTTP status codes |
| requirement | Storage Layer | In-memory storage with optional JSON file persistence |
| requirement | Build Pipeline | package.json with build/test scripts, tsconfig.json for ESM |
| requirement | Test Suite | API tests using vitest or Node.js test runner |
| requirement | API Documentation | README with endpoint docs and curl examples |

---

## Phase 3: Goal Validation

**Goal:** Confirm goals are correctly persisted and cover the program specification.

**Steps:**

1. **Navigate to GoalsPanel** (if not already there):
   - `browser_click` -> "More panels..." -> "Goals"
   - `browser_snapshot` -> GoalsPanel with goal list visible

2. **Count goals:**
   - `browser_evaluate`:
     ```js
     const goals = await fetch('/api/v1/projects/' + PROJECT_ID + '/goals', {
       headers: {'Authorization': 'Bearer ' + ENV.token}
     }).then(r => r.json());
     JSON.stringify({
       count: goals.length,
       kinds: goals.map(g => g.kind),
       titles: goals.map(g => g.title),
       allEnabled: goals.every(g => g.enabled),
       sources: [...new Set(goals.map(g => g.source))]
     })
     ```
   -> Store `GOAL_COUNT`

3. **Validate coverage** (scenario-specific keyword check):
   - `browser_evaluate`:
     ```js
     const goals = await fetch('/api/v1/projects/' + PROJECT_ID + '/goals', {
       headers: {'Authorization': 'Bearer ' + ENV.token}
     }).then(r => r.json());
     const text = goals.map(g => g.title + ' ' + g.content).join(' ').toLowerCase();

     // Scenario-specific keywords:
     // S1: ['csv', 'json', 'convert', 'error', 'test', 'cli']
     // S2: ['add', 'list', 'complete', 'delete', 'test', 'readme', 'json', 'cli']
     // S3: ['tag', 'search', 'existing', 'migration', 'test', 'edit', 'extend']
     // S4: ['rest', 'api', 'typescript', 'crud', 'test', 'validation', 'build']
     const features = SCENARIO_KEYWORDS;  // select based on active scenario
     const found = features.filter(f => text.includes(f));
     const missing = features.filter(f => !text.includes(f));
     JSON.stringify({found, missing, coverage: found.length + '/' + features.length})
     ```

4. `browser_take_screenshot` -> "Phase 3: Goals Validated"

**Validation (scenario-specific):**

| Check | S1 Easy | S2 Medium | S3 Hard | S4 Expert |
|-------|---------|-----------|---------|-----------|
| Min goal count | >= 3 | >= 4 | >= 5 | >= 6 |
| Requirement goals | >= 2 | >= 3 | >= 4 | >= 5 |
| Keyword coverage | >= 4/6 | >= 5/8 | >= 5/7 | >= 5/7 |
| All enabled | Yes | Yes | Yes | Yes |

**Decision Tree:**
```
0 goals found?
├─ Phase 2 failed silently -> Execute Phase 2b (manual creation)
├─ Goals created but not persisted -> Check auto-persistence in runtime.go
└─ FLAG: "Goal auto-persistence broken"

Coverage < 5/8?
├─ Accept if agent grouped features (e.g., "CRUD operations" covers add/list/complete/delete)
├─ Add missing goals manually via GoalsPanel "+ Add"
└─ Log which features were missing
```

---

## Phase 4: Roadmap Creation

**Goal:** Build a structured roadmap with milestones and atomic features from the goals.

**Steps:**

1. **Open Roadmap panel:**
   - `browser_click` -> "More panels..." dropdown
   - `browser_click` -> "Roadmap"
   - `browser_snapshot` -> RoadmapPanel visible (empty state with "Create Roadmap" form, or existing roadmap)

2. **Create roadmap:**
   - `browser_fill_form`:
     - Title: scenario-specific (see below)
     - Description: `Build from detected goals`
   - `browser_click` -> "Create Roadmap" button
   - `browser_snapshot` -> Roadmap created, empty milestone list visible

3. **Add milestones and features (scenario-specific):**

   For each milestone:
   - `browser_click` -> "Add Milestone" button at bottom of RoadmapPanel
   - `browser_fill_form` -> Title input
   - `browser_click` -> Confirm/Create button

   For each feature within a milestone:
   - `browser_click` -> "Add Feature" link within that milestone
   - `browser_fill_form` -> Feature title (atomic, single responsibility)
   - `browser_click` -> Create/Confirm button

---

#### S1 Roadmap: CSV-to-JSON Converter

**Title:** `CSV to JSON Converter`

| Milestone | Features (atomic tasks) |
|-----------|------------------------|
| **1. Core Script** | Create csv2json.py with argparse for input file path |
| | Read CSV using csv.DictReader and convert rows to list of dicts |
| | Write output as JSON with same filename but .json extension |
| **2. Error Handling & Tests** | Handle FileNotFoundError with user-friendly message |
| | Handle csv.Error for malformed CSV with error message |
| | Create test_csv2json.py with sample CSV fixture |
| | Test successful conversion (valid CSV -> correct JSON) |
| | Test error cases (missing file, invalid CSV) |

**Milestones:** 2 | **Features:** 8

---

#### S2 Roadmap: CLI Task Manager

**Title:** `Python CLI Task Manager`

| Milestone | Features (atomic tasks) |
|-----------|------------------------|
| **1. Project Setup** | Create pyproject.toml with project metadata and pytest dependency |
| | Create task_manager/__init__.py and __main__.py entry point |
| | Create .gitignore for Python projects |
| **2. Core Data Model** | Create task_manager/models.py with Task dataclass (id, title, priority, status, created_at) |
| | Add Task.to_dict() and Task.from_dict() for JSON serialization |
| | Add TaskStatus and Priority enums |
| **3. Storage Layer** | Create task_manager/storage.py with load_tasks(path) returning list of Task |
| | Add save_tasks(path, tasks) with atomic write (write to .tmp then rename) |
| | Handle missing file gracefully (return empty list) |
| **4. CLI Interface** | Create task_manager/cli.py with argparse main parser and subparsers |
| | Implement add subcommand: --title (required), --priority (default medium) |
| | Implement list subcommand: --status filter (optional, open/done/all) |
| | Implement complete subcommand: task_id positional argument |
| | Implement delete subcommand: task_id positional argument |
| | Wire CLI to storage layer and print formatted output |
| **5. Test Suite** | Create tests/test_models.py: test Task creation, serialization, enums |
| | Create tests/test_storage.py: test load/save with tmp_path, missing file |
| | Create tests/test_cli.py: test add, list, complete, delete via subprocess |
| | Create tests/conftest.py with shared fixtures (tmp storage path) |
| **6. Documentation** | Create README.md with project overview, installation, and usage examples |

**Milestones:** 6 | **Features:** 20

---

#### S3 Roadmap: Add Tags Feature (Brownfield)

**Title:** `Add Tags Feature to Existing Task Manager`

> **Precondition:** The workspace must contain an existing Python task manager (from a prior S2 run or seeded codebase). If empty, seed via Phase 2b first.

| Milestone | Features (atomic tasks) |
|-----------|------------------------|
| **1. Codebase Analysis** | Read and list all existing Python files to understand project structure |
| | Read models.py to understand Task dataclass and existing fields |
| | Read cli.py to understand existing subcommands and argparse setup |
| | Read storage.py to understand JSON serialization format |
| | Run existing tests to confirm baseline passes |
| **2. Data Model Extension** | Edit models.py: add tags field (list[str], default empty) to Task dataclass |
| | Edit models.py: update to_dict() and from_dict() to include tags |
| | Verify existing tests still pass after model change |
| **3. Storage Migration** | Edit storage.py: handle loading old JSON without tags field (default to []) |
| | Create a migration test: load old-format JSON, verify tags default to [] |
| **4. CLI Extension** | Edit cli.py: add --tags option to add subcommand (comma-separated) |
| | Edit cli.py: add search subcommand with --tag filter argument |
| | Edit cli.py: show tags in list output format |
| **5. Tests & Docs** | Create tests/test_tags.py: test add with tags, search by tag, empty tags |
| | Run full test suite (old + new tests must all pass) |
| | Edit README.md: add tags feature documentation with examples |

**Milestones:** 5 | **Features:** 15

**Key difference:** This scenario forces the agent to **Read, Search, and Edit** existing files rather than creating everything from scratch. The agent must understand existing code patterns before modifying them.

---

#### S4 Roadmap: TypeScript REST API

**Title:** `TypeScript Bookmark Manager REST API`

| Milestone | Features (atomic tasks) |
|-----------|------------------------|
| **1. Project Setup** | Create package.json with name, scripts (build, start, test, dev), dependencies |
| | Create tsconfig.json with strict mode, ES modules, outDir |
| | Create directory structure: src/, src/routes/, src/models/, tests/ |
| | Create .gitignore for Node.js/TypeScript projects |
| **2. Types & Models** | Create src/models/bookmark.ts with Bookmark interface (id, title, url, tags, created_at) |
| | Create src/models/errors.ts with AppError class and HTTP status codes |
| | Create src/models/validation.ts with URL validator and field validator |
| **3. Storage Layer** | Create src/storage/store.ts with BookmarkStore interface |
| | Create src/storage/memory.ts with InMemoryStore implementation |
| | Create src/storage/file.ts with JsonFileStore (optional persistence) |
| **4. HTTP Server** | Create src/server.ts with HTTP server setup (Express or native http) |
| | Create src/routes/bookmarks.ts with router for /bookmarks endpoints |
| | Create src/middleware/error-handler.ts for centralized error handling |
| | Create src/middleware/json-parser.ts for request body parsing |
| **5. CRUD Endpoints** | Implement GET /bookmarks (list all, optional ?tag= filter) |
| | Implement GET /bookmarks/:id (get single, 404 if not found) |
| | Implement POST /bookmarks (create, validate fields, return 201) |
| | Implement PUT /bookmarks/:id (update, validate, 404 if not found) |
| | Implement DELETE /bookmarks/:id (delete, 204 on success, 404 if not found) |
| **6. Validation & Errors** | Add URL format validation (reject invalid URLs with 400) |
| | Add required field validation (title, url required; 400 if missing) |
| | Return proper error JSON format: { error: string, status: number } |
| **7. Test Suite** | Create tests/bookmarks.test.ts: test all CRUD operations |
| | Create tests/validation.test.ts: test URL and field validation |
| | Create tests/storage.test.ts: test in-memory and file stores |
| **8. Documentation** | Create README.md with API endpoints, curl examples, setup instructions |

**Milestones:** 8 | **Features:** 26

**Key difference:** This scenario exercises a **different language** (TypeScript), requires **build compilation** (tsc), uses **npm** for dependency management, and introduces **HTTP server testing** (start server, curl, verify responses). The agent needs Bash heavily for npm commands and must handle the compile-fix-test cycle.

---

5. **Verify roadmap structure:**
   - `browser_snapshot` -> Full roadmap with milestones and features
   - `browser_evaluate`:
     ```js
     const roadmap = await fetch('/api/v1/projects/' + PROJECT_ID + '/roadmap', {
       headers: {'Authorization': 'Bearer ' + ENV.token}
     }).then(r => r.json());
     JSON.stringify({
       title: roadmap.title,
       status: roadmap.status,
       milestones: roadmap.milestones.map(m => ({
         title: m.title,
         features: m.features ? m.features.length : 0
       })),
       totalFeatures: roadmap.milestones.reduce((s, m) => s + (m.features ? m.features.length : 0), 0)
     })
     ```

6. `browser_take_screenshot` -> "Phase 4: Roadmap Created"

**Validation (scenario-specific):**

| Check | S1 | S2 | S3 | S4 |
|-------|----|----|----|----|
| Min milestones | 2 | 6 | 5 | 8 |
| Min features | 8 | 15 | 12 | 20 |
| `ROADMAP_OK` | true | true | true | true |

**Decision Tree:**
```
"Create Roadmap" button not visible?
├─ Roadmap already exists -> browser_evaluate check API
├─ Panel rendering error -> browser_console_messages
└─ Use API fallback via browser_evaluate

Milestone creation fails?
├─ UI form not appearing -> browser_snapshot, retry click
├─ API error -> browser_console_messages
└─ Create via browser_evaluate (API fallback)

Feature creation fails per milestone?
├─ "Add Feature" link not clickable -> browser_snapshot to identify correct element
├─ Form validation error -> Check required fields
└─ Create via browser_evaluate (POST /milestones/{id}/features)
```

---

## Phase 5: Autonomous Execution

**Goal:** Start an agentic chat conversation that executes the roadmap tasks.

**Steps:**

1. **Get AI-friendly roadmap:**
   - `browser_evaluate`:
     ```js
     const aiView = await fetch('/api/v1/projects/' + PROJECT_ID + '/roadmap/ai?format=markdown', {
       headers: {'Authorization': 'Bearer ' + ENV.token}
     }).then(r => r.json());
     aiView.content
     ```
   -> Store as `AI_ROADMAP`

2. **Navigate to chat panel:**
   - `browser_click` -> Chat area on the right side of ProjectDetailPage
   - If a conversation from Phase 2 is active, create a new one:
     - `browser_click` -> (+) "New Conversation" button (`data-testid="new-conversation-btn"`)
   - `browser_snapshot` -> Clean chat with input area visible

3. **Send execution instruction (scenario-specific):**
   - `browser_click` -> Chat input textarea
   - `browser_type` -> Scenario-specific execution prompt:

   **S1 prompt:**
   ```
   Create a CSV-to-JSON converter script in this workspace. Follow the roadmap below exactly.
   Commit your changes when done.
   RULES: Use only Python standard library. Keep it simple. Run the test when done.
   ROADMAP: [AI_ROADMAP]
   ```

   **S2 prompt:**
   ```
   Build a Python CLI task manager in this workspace. Follow the roadmap exactly,
   implementing each milestone in order. Commit after each milestone.
   RULES: Use argparse for CLI, dataclasses for models, json for storage, pytest for tests.
   No external dependencies beyond pytest. If unsure, ask me.
   ROADMAP: [AI_ROADMAP]
   ```

   **S3 prompt:**
   ```
   This workspace contains an existing Python task manager. Your job is to ADD a tags feature.
   IMPORTANT: Read and understand the existing code FIRST before making any changes.
   Do NOT rewrite files from scratch - EDIT existing files to add the feature.
   All existing tests MUST still pass after your changes.
   Follow the roadmap exactly. Commit after each milestone.
   If you are unsure about the existing architecture, ask me.
   ROADMAP: [AI_ROADMAP]
   ```

   **S4 prompt:**
   ```
   Build a TypeScript REST API for a bookmark manager in this workspace.
   Follow the roadmap exactly, implementing each milestone in order.
   RULES: TypeScript strict mode, no 'any' types. ES modules. Express or native http.
   Run 'npm run build' after code changes to catch type errors early.
   Run 'npm test' after implementing the test suite. Commit after each milestone.
   If you are unsure, ask me.
   ROADMAP: [AI_ROADMAP]
   ```

   - `browser_click` -> Send button

4. **Verify agentic mode activated:**
   - `browser_snapshot` -> Look for:
     - "Agentic" badge with pulsing dot in chat header
     - Streaming content appearing
     - Tool call indicators (left-bordered blocks)
   - `EXEC_STARTED = true`

5. `browser_take_screenshot` -> "Phase 5: Execution Started"

**Validation:**
- Message sent successfully (appears in chat)
- Agentic badge visible (agent is running)
- Streaming content or tool calls appearing within 30 seconds
- `EXEC_STARTED = true`

**Decision Tree:**
```
Message sent but no agentic badge?
├─ Agent mode not activated -> Project may lack workspace_path
│  browser_evaluate: check project.workspace_path
├─ Message treated as simple chat -> Check mode/autonomy settings
│  May need to set autonomy to 4+ via settings popover (gear icon)
└─ Worker not consuming -> Check NATS delivery

No streaming content after 30s?
├─ browser_console_messages -> WS errors?
├─ Check ActiveWorkPanel -> Shows queued/running task?
├─ LiteLLM model timeout -> Check /api/v1/llm/discover for available models
└─ Wait longer (some models take 60s+ for first token)

Agent immediately finishes with short response?
├─ Model may not support tool use -> Check ENV.models for tool-capable
├─ Policy blocked all tools -> Check autonomy level (need >= 3 for auto-edit)
└─ Context too short -> Check message was sent correctly
```

---

## Phase 5b: Blocker & HITL Handling

**Goal:** Handle permission requests and agent questions during execution.

This phase runs **concurrently with Phase 6** — check for these events on every monitoring poll.

**Steps:**

1. **Handle PermissionRequestCards (HITL):**
   - `browser_snapshot` -> Look for PermissionRequestCard (amber-bordered card with countdown timer)
   - If found:
     - Read tool name and command from the card
     - `browser_click` -> "Allow" button (approve the tool execution)
     - Alternatively: `browser_click` -> "Allow Always" (saves policy rule, prevents re-asking for same tool)
     - `browser_snapshot` -> Card changes to approved state (green border)
   - Repeat for every new PermissionRequestCard that appears

2. **Handle agent questions (blockers):**
   - `browser_snapshot` -> Check if latest assistant message contains a question (ends with `?`)
   - If agent is asking a question and NOT executing tools:
     - `browser_click` -> Chat input
     - `browser_type` -> Scenario-specific unblock response:
       - **S1:** `Use your best judgment. Standard library only, keep it minimal.`
       - **S2:** `Use your best judgment and proceed. Standard Python practices, no external dependencies beyond pytest. Use argparse for CLI, dataclasses for models, json module for storage.`
       - **S3:** `Follow the existing code patterns and conventions. Read the existing files for guidance. Do not change the overall architecture. All existing tests must still pass.`
       - **S4:** `Use Express for the HTTP server and vitest for tests. Keep dependencies minimal. Follow TypeScript best practices with strict mode.`
     - `browser_click` -> Send button
     - `browser_wait_for` -> Agent resumes execution

3. **Handle timeout on PermissionRequestCards:**
   - If countdown reaches < 10s without user action:
     - `browser_click` -> "Allow" IMMEDIATELY (before auto-deny kicks in)
   - If auto-denied (card turns red):
     - Agent may need to retry or work around the denied action
     - Monitor for agent adapting

**Validation:**
- All PermissionRequestCards resolved (approved or allowed-always)
- Agent questions answered and execution resumed
- No auto-denied actions (all approved in time)

---

## Phase 6: Execution Monitoring

**Goal:** Monitor agent progress until completion. Poll browser state regularly.

**Steps:**

1. **Monitoring loop** (run for up to 30 minutes):

   Every 30 seconds:
   - `browser_snapshot` -> Observe:
     - **Streaming content**: Is new text appearing?
     - **Tool calls**: Are new tool call blocks appearing? (Read, Write, Edit, Bash indicators)
     - **Step counter**: Check "Step N" in chat header — is it incrementing?
     - **Cost display**: Check running cost in header
     - **Agentic badge**: Still pulsing? (agent still running)
     - **PermissionRequestCards**: Any pending? -> Handle per Phase 5b
     - **Agent questions**: Any new questions? -> Handle per Phase 5b
     - **Error banners**: Any red error messages?

2. **Check for completion:**
   - Agent run finishes when:
     - Agentic badge disappears (no more pulsing dot)
     - `agui.run_finished` event received (no more streaming)
     - Final assistant message appears without tool calls
   - `browser_snapshot` -> Confirm: no active streaming, no pending tool calls

3. **Verify git activity:**
   - `browser_evaluate`:
     ```js
     const gitStatus = await fetch('/api/v1/projects/' + PROJECT_ID + '/git/status', {
       headers: {'Authorization': 'Bearer ' + ENV.token}
     }).then(r => r.json());
     JSON.stringify({branch: gitStatus.branch, dirty: gitStatus.dirty, ahead: gitStatus.ahead})
     ```
   - `browser_evaluate`:
     ```js
     const log = await fetch('/api/v1/projects/' + PROJECT_ID + '/git/log?limit=20', {
       headers: {'Authorization': 'Bearer ' + ENV.token}
     }).then(r => r.json());
     log.map(c => c.message).join('\n')
     ```
   -> Store commit messages

4. **Check for agent stall:**
   - If step counter hasn't changed in 2 minutes AND streaming is idle:
     - `browser_snapshot` -> Check for stall message ("You are repeating...")
     - If stalled: wait for stall detector to abort
     - If no progress at all: consider sending a nudge message
       - `browser_click` -> Chat input
       - `browser_type` -> `Please continue with the next milestone.`
       - `browser_click` -> Send

5. **Check ActiveWorkPanel:**
   - `browser_snapshot` -> Look for ActiveWorkPanel above chat
   - Should show running task with agent mode, step count, cost

6. `browser_take_screenshot` -> "Phase 6: Execution Progress" (capture mid-execution)
7. `browser_take_screenshot` -> "Phase 6: Execution Complete" (capture after agent finishes)

**Validation:**
- Agent executed for at least 5 tool call iterations
- At least 1 git commit in the test branch
- Agent run completed (not stuck/stalled)
- No unhandled errors

**Decision Tree:**
```
Agent stuck (no progress for 5 minutes)?
├─ PermissionRequestCard pending -> Approve it (Phase 5b)
├─ Agent asking question -> Answer it (Phase 5b)
├─ Stall detected -> Wait for auto-abort, then send "Continue with next milestone"
├─ Model rate limited -> Wait 60s, agent should auto-retry
└─ Hard stuck -> browser_click Stop button, assess partial results

Agent errored out?
├─ Read error message in chat
├─ browser_console_messages -> WS/API errors?
├─ Check if partial work was committed (git log)
├─ Send follow-up: "There was an error. Please continue from where you left off."
└─ If error persists -> Proceed to Phase 7 with partial results

Agent finished too quickly (< 2 minutes)?
├─ Check: did it actually create files? (browser_evaluate: ls workspace)
├─ Check: was response just text advice, not actual tool execution?
├─ May need higher autonomy level -> Open settings (gear icon), increase autonomy
└─ Resend instruction with explicit "Use the Write tool to create files"

0 git commits?
├─ Agent may have written files but not committed
├─ Check workspace for files: browser_evaluate fetch git/status
├─ OK if files exist uncommitted (agent may commit at end)
└─ FLAG if workspace is empty: "Agent did not produce any files"
```

---

## Phase 7: Program Validation (Scenario-Specific)

**Goal:** Verify the resulting program is functional by running it in the workspace.

All validation steps are executed **via the agent chat** — ask the agent to run commands and observe the Bash tool output in `browser_snapshot`.

---

### S1 Validation: CSV-to-JSON Converter

1. **Check files exist:**
   - `browser_click` -> Chat input
   - `browser_type` -> `List all Python files in the workspace: ls -la *.py`
   - `browser_click` -> Send
   - `browser_snapshot` -> Verify `csv2json.py` and `test_csv2json.py` exist

2. **Create sample CSV and test conversion:**
   - `browser_click` -> Chat input
   - `browser_type`:
     ```
     Create a sample test.csv with this content, then run the converter:
     echo 'name,age,city\nAlice,30,Berlin\nBob,25,Munich' > test.csv
     python csv2json.py test.csv
     cat test.json
     ```
   - `browser_click` -> Send
   - `browser_snapshot` -> Verify JSON output is correct

3. **Test error handling:**
   - `browser_type` -> `python csv2json.py nonexistent.csv`
   - `browser_snapshot` -> Should show error message, not traceback

4. **Run tests:**
   - `browser_type` -> `python -m pytest test_csv2json.py -v`
   - `browser_snapshot` -> Check for "passed"

5. `browser_take_screenshot` -> "Phase 7: S1 Validation"

**S1 Validation Matrix:**

| Check | Verify | Required |
|-------|--------|----------|
| csv2json.py exists | File listing | Yes |
| Converts valid CSV | JSON output correct | Yes |
| Error on missing file | No traceback, user-friendly message | Yes |
| Tests pass | pytest output | Yes |

---

### S2 Validation: CLI Task Manager

1. **Check file structure:**
   - `browser_click` -> Chat input
   - `browser_type` -> `Show the project structure: find . -name "*.py" -o -name "*.toml" -o -name "*.md" | grep -v __pycache__ | sort`
   - `browser_snapshot` -> Verify task_manager package and test files exist

2. **Run help:**
   - `browser_type` -> `python -m task_manager --help`
   - `browser_snapshot` -> Shows "usage" and subcommand names

3. **Run full CRUD cycle:**
   - `browser_type`:
     ```
     Run these commands in sequence and show output:
     1. python -m task_manager add --title "Buy groceries" --priority high
     2. python -m task_manager add --title "Clean house" --priority low
     3. python -m task_manager list
     4. python -m task_manager list --status open
     5. python -m task_manager complete 1
     6. python -m task_manager list --status done
     7. python -m task_manager delete 2
     8. python -m task_manager list
     ```
   - `browser_snapshot` -> Verify each step

4. **Run tests:**
   - `browser_type` -> `python -m pytest -v`
   - `browser_snapshot` -> All pass

5. **Check JSON persistence:**
   - `browser_type` -> `cat tasks.json | python -m json.tool`
   - `browser_snapshot` -> Valid JSON

6. `browser_take_screenshot` -> "Phase 7: S2 Validation"

**S2 Validation Matrix:**

| Check | Verify | Required |
|-------|--------|----------|
| Package structure | task_manager/__init__.py, __main__.py | Yes |
| --help exits 0 | Shows usage and commands | Yes |
| Add command | Creates task | Yes |
| List command | Shows tasks | Yes |
| Complete command | Marks done | Yes |
| Delete command | Removes task | Yes |
| Tests pass | pytest output | Yes |
| JSON valid | json.tool parses file | Yes |
| README exists | File listing | No |

---

### S3 Validation: Tags Feature (Brownfield)

1. **Run existing tests first (regression check):**
   - `browser_click` -> Chat input
   - `browser_type` -> `Run the full test suite to verify no regressions: python -m pytest -v`
   - `browser_snapshot` -> ALL tests must pass (old + new)

2. **Test tags on add:**
   - `browser_type` -> `python -m task_manager add --title "Tagged task" --tags "urgent,work"`
   - `browser_snapshot` -> Task created with tags

3. **Test search by tag:**
   - `browser_type` -> `python -m task_manager search --tag "urgent"`
   - `browser_snapshot` -> Found the tagged task

4. **Test tags in list output:**
   - `browser_type` -> `python -m task_manager list`
   - `browser_snapshot` -> Tags visible in output

5. **Test data migration (existing tasks without tags):**
   - `browser_type` -> `python -m task_manager list`
   - `browser_snapshot` -> Existing tasks show empty tags, no crash

6. **Verify existing CRUD still works:**
   - `browser_type`:
     ```
     python -m task_manager add --title "No tags task"
     python -m task_manager complete 1
     python -m task_manager delete 1
     ```
   - `browser_snapshot` -> All work without errors

7. **Check README updated:**
   - `browser_type` -> `grep -i "tag" README.md`
   - `browser_snapshot` -> Tags feature documented

8. `browser_take_screenshot` -> "Phase 7: S3 Validation"

**S3 Validation Matrix:**

| Check | Verify | Required |
|-------|--------|----------|
| Existing tests pass | No regression | **Critical** |
| New tag tests pass | pytest shows new tests passing | Yes |
| Add with --tags | Command succeeds | Yes |
| Search --tag | Finds tagged tasks | Yes |
| Tags in list | Tags visible in output | Yes |
| Old tasks survive | No crash on tagless tasks | Yes |
| Existing CRUD works | add/complete/delete still functional | Yes |
| README updated | grep finds "tag" mention | No |

---

### S4 Validation: TypeScript REST API

1. **Check TypeScript compiles:**
   - `browser_click` -> Chat input
   - `browser_type` -> `npm run build`
   - `browser_snapshot` -> No TypeScript errors, exit 0

2. **Run tests:**
   - `browser_type` -> `npm test`
   - `browser_snapshot` -> Tests pass

3. **Start server and test endpoints:**
   - `browser_type`:
     ```
     Run these commands (start server in background, test, then stop):
     npm start &
     sleep 2
     echo "=== POST ==="
     curl -s -X POST http://localhost:3000/bookmarks -H "Content-Type: application/json" -d '{"title":"GitHub","url":"https://github.com","tags":["dev"]}'
     echo "=== GET ALL ==="
     curl -s http://localhost:3000/bookmarks
     echo "=== GET ONE ==="
     curl -s http://localhost:3000/bookmarks/1
     echo "=== DELETE ==="
     curl -s -X DELETE http://localhost:3000/bookmarks/1 -w "\nHTTP %{http_code}"
     echo "=== VALIDATION ==="
     curl -s -X POST http://localhost:3000/bookmarks -H "Content-Type: application/json" -d '{"title":"Bad"}'
     kill %1 2>/dev/null
     ```
   - `browser_snapshot` -> Verify:
     - POST returns 201 with bookmark data
     - GET ALL returns array
     - GET ONE returns single bookmark
     - DELETE returns 204
     - Missing URL returns 400

4. **Check no 'any' types:**
   - `browser_type` -> `grep -rn "any" src/ --include="*.ts" | grep -v node_modules | grep -v "// " | head -20`
   - `browser_snapshot` -> Should find 0 or minimal `any` occurrences

5. **Check project structure:**
   - `browser_type` -> `find src tests -name "*.ts" | sort`
   - `browser_snapshot` -> Proper directory structure

6. `browser_take_screenshot` -> "Phase 7: S4 Validation"

**S4 Validation Matrix:**

| Check | Verify | Required |
|-------|--------|----------|
| tsc compiles | `npm run build` exit 0 | Yes |
| Tests pass | `npm test` exit 0 | Yes |
| POST /bookmarks | Returns 201, bookmark JSON | Yes |
| GET /bookmarks | Returns array | Yes |
| GET /bookmarks/:id | Returns single bookmark | Yes |
| DELETE /bookmarks/:id | Returns 204 | Yes |
| Validation works | Missing URL -> 400 | Yes |
| No `any` types | grep finds 0 | No |
| README exists | File listing | No |

---

### General Validation Decision Tree

```
Files don't exist?
├─ Agent may have created files in wrong directory
│  Ask agent: "List all files you created. Where is the project root?"
├─ Agent may not have executed (Phase 6 issue)
└─ FAIL: "No program files produced"

Build/compile fails? (S4)
├─ TypeScript errors -> Ask agent: "Fix the TypeScript compilation errors"
├─ Missing dependencies -> Ask agent: "Run npm install"
└─ tsconfig issue -> Ask agent: "Check tsconfig.json"

Tests fail?
├─ Read test output carefully for specific failures
├─ If > 50% pass -> PARTIAL
├─ Ask agent: "The tests are failing. Here is the output: [paste]. Fix the issues."
├─ Re-run tests after fix (allow 1 fix attempt)
└─ If still failing after fix -> PARTIAL

Regression in S3?
├─ Existing tests fail -> CRITICAL FAIL (the whole point of S3 is non-regression)
├─ Ask agent: "You broke existing tests. Revert your changes and try a different approach."
└─ If agent can fix -> re-run all tests

Functional test fails?
├─ Wrong output format -> Accept if core logic works
├─ Crash/traceback -> Ask agent to fix, re-test (1 attempt)
└─ PARTIAL if >= 60% of checks pass
```

5. `browser_take_screenshot` -> "Phase 7: Program Validation Results"

---

## Phase 8: Report & Cleanup

**Goal:** Collect metrics, generate report, preserve test branch.

**Steps:**

1. **Collect metrics via API:**
   - `browser_evaluate`:
     ```js
     // Goal count
     const goals = await fetch('/api/v1/projects/' + PROJECT_ID + '/goals', {
       headers: {'Authorization': 'Bearer ' + ENV.token}
     }).then(r => r.json());

     // Roadmap stats
     const roadmap = await fetch('/api/v1/projects/' + PROJECT_ID + '/roadmap', {
       headers: {'Authorization': 'Bearer ' + ENV.token}
     }).then(r => r.json());

     // Git stats
     const gitLog = await fetch('/api/v1/projects/' + PROJECT_ID + '/git/log?limit=50', {
       headers: {'Authorization': 'Bearer ' + ENV.token}
     }).then(r => r.json());

     JSON.stringify({
       goals: goals.length,
       milestones: roadmap.milestones ? roadmap.milestones.length : 0,
       features: roadmap.milestones ?
         roadmap.milestones.reduce((s, m) => s + (m.features ? m.features.length : 0), 0) : 0,
       commits: gitLog.length,
       commitMessages: gitLog.map(c => c.message)
     })
     ```

2. **Collect conversation stats (including tool call breakdown):**
   - `browser_evaluate`:
     ```js
     const convs = await fetch('/api/v1/projects/' + PROJECT_ID + '/conversations', {
       headers: {'Authorization': 'Bearer ' + ENV.token}
     }).then(r => r.json());

     let totalMessages = 0;
     let totalToolCalls = 0;
     const toolCallsByType = {};  // Track which tools were used

     for (const conv of convs) {
       const msgs = await fetch('/api/v1/conversations/' + conv.id + '/messages', {
         headers: {'Authorization': 'Bearer ' + ENV.token}
       }).then(r => r.json());
       totalMessages += msgs.length;

       for (const msg of msgs) {
         if (msg.tool_calls) {
           totalToolCalls += msg.tool_calls.length;
           for (const tc of msg.tool_calls) {
             toolCallsByType[tc.name] = (toolCallsByType[tc.name] || 0) + 1;
           }
         }
       }
     }

     JSON.stringify({
       conversations: convs.length,
       totalMessages,
       totalToolCalls,
       toolCallsByType  // e.g. {Write: 12, Read: 8, Bash: 15, Edit: 5, Search: 3, ...}
     })
     ```

3. **Take final screenshots:**
   - `browser_click` -> "More panels..." -> "Goals"
   - `browser_take_screenshot` -> "Final: GoalsPanel"
   - `browser_click` -> "More panels..." -> "Roadmap"
   - `browser_take_screenshot` -> "Final: RoadmapPanel"
   - `browser_click` -> Chat tab
   - `browser_take_screenshot` -> "Final: Chat History"

4. **Generate report summary** (Claude Code writes to report file):

   ```
   ============================================
   AUTONOMOUS GOAL-TO-PROGRAM TEST REPORT
   ============================================
   Date:          YYYY-MM-DDTHH:MM:SSZ
   Scenario:      S1/S2/S3/S4 (Easy/Medium/Hard/Expert)
   Branch:        test/autonomous-YYYYMMDD-HHmmss
   Project ID:    <uuid>

   PHASE RESULTS:
   - Phase 0 (Environment):     PASS/FAIL
   - Phase 1 (Project Setup):   PASS/FAIL
   - Phase 2 (Goal Discovery):  PASS/FAIL (N goals)
   - Phase 3 (Goal Validation): PASS/FAIL (N/M coverage)
   - Phase 4 (Roadmap):         PASS/FAIL (N milestones, N features)
   - Phase 5 (Execution):       PASS/FAIL
   - Phase 6 (Monitoring):      PASS/FAIL (N commits)
   - Phase 7 (Validation):      PASS/FAIL (see scenario-specific checks)

   SCENARIO-SPECIFIC CHECKS (Phase 7):
   [List all checks from the scenario's Validation Matrix with PASS/FAIL]

   METRICS:
   - Goals:          N
   - Milestones:     N
   - Features:       N
   - Commits:        N
   - Messages:       N
   - Conversations:  N

   TOOL CALL BREAKDOWN:
   - Write:          N calls
   - Read:           N calls
   - Edit:           N calls
   - Bash:           N calls
   - Search:         N calls
   - Glob:           N calls
   - ListDir:        N calls
   - propose_goal:   N calls
   - Other:          N calls
   - TOTAL:          N calls

   TOOL CALL DIVERSITY SCORE:
   - Tools used:     N/7 unique tools
   - Expected:       [from Tool Call Coverage Matrix for scenario]
   - Match:          PASS/PARTIAL/FAIL

   OVERALL: PASS / PARTIAL / FAIL
   ============================================
   ```

5. **Preserve branch** (do NOT delete):
   - Branch `$BRANCH` remains in the workspace for manual review
   - Push to remote if available:
     ```js
     fetch('/api/v1/projects/' + PROJECT_ID + '/git/push', {
       method: 'POST',
       headers: {'Authorization': 'Bearer ' + ENV.token, 'Content-Type': 'application/json'},
       body: JSON.stringify({branch: BRANCH})
     })
     ```

6. `browser_take_screenshot` -> "Phase 8: Test Complete"

---

## Implementation Gaps Identified

Features required for full automation but potentially not yet implemented:

| Gap | Current State | Impact on Test |
|-----|---------------|---------------|
| Goals -> Roadmap auto-generation | Manual/import only | Phase 4 uses manual UI creation |
| Roadmap -> Task auto-decomposition | PlanFeature exists but not wired | Phase 5 uses chat-based execution |
| AI Discover opens new conversation | May redirect away from GoalsPanel | Phase 2 adapts to navigation change |
| File listing API | May not exist at /files endpoint | Phase 7 uses agent to check files |
| Direct command execution API | May not exist at /exec endpoint | Phase 7 uses agent Bash tool |

**Fallback strategy:** Where auto-generation is not available, the test uses the UI (milestones/features created manually in RoadmapPanel) and agent-based execution (agentic chat with full roadmap as prompt).

---

## Estimated Resources per Scenario

| Metric | S1 Easy | S2 Medium | S3 Hard | S4 Expert |
|--------|---------|-----------|---------|-----------|
| **Time** | 10-15 min | 20-35 min | 25-45 min | 30-60 min |
| **LLM Cost** | $0.20-$1.00 | $0.50-$3.00 | $1.00-$5.00 | $2.00-$8.00 |
| **Tokens** | ~30K-80K | ~80K-200K | ~100K-300K | ~150K-400K |
| **Screenshots** | ~8 | ~12 | ~14 | ~16 |
| **Browser interactions** | ~30-50 | ~50-80 | ~60-100 | ~80-120 |
| **Expected tool calls** | 5-15 | 15-40 | 25-60 | 40-80 |
| **Expected unique tools** | 2 (Write, Bash) | 4-5 | 6-7 | 6-7 |

---

## Key Files Reference

| Component | File |
|-----------|------|
| GoalsPanel | `frontend/src/features/project/GoalsPanel.tsx` |
| GoalProposalCard | `frontend/src/features/project/GoalProposalCard.tsx` |
| RoadmapPanel | `frontend/src/features/project/RoadmapPanel.tsx` |
| ChatPanel | `frontend/src/features/project/ChatPanel.tsx` |
| PermissionRequestCard | `frontend/src/features/project/PermissionRequestCard.tsx` |
| ProjectDetailPage | `frontend/src/features/project/ProjectDetailPage.tsx` |
| CreateProjectModal | `frontend/src/features/dashboard/CreateProjectModal.tsx` |
| ActiveWorkPanel | `frontend/src/features/project/ActiveWorkPanel.tsx` |
| Goal domain model | `internal/domain/goal/goal.go` |
| Goal discovery service | `internal/service/goal_discovery.go` |
| Goal researcher prompt | `internal/service/prompts/modes/goal_researcher.yaml` |
| propose_goal tool | `workers/codeforge/tools/propose_goal.py` |
| Roadmap service | `internal/service/roadmap.go` |
| Orchestrator | `internal/service/orchestrator.go` |
| Agent loop | `workers/codeforge/agent_loop.py` |
| Conversation dispatch | `internal/service/conversation.go` |
| HITL approval | `internal/service/runtime_approval.go` |
