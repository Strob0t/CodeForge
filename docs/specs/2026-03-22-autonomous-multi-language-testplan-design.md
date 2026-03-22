# Autonomous Multi-Language Project Testplan — Design Spec

**Date:** 2026-03-22
**Type:** Design specification for an automated AI agent workflow
**Scope:** Claude Code drives CodeForge via Playwright-MCP to autonomously build and verify a multi-language project

---

## 1. Overview

### What This Is

A single testplan document that Claude Code follows to:

1. Start all CodeForge services
2. Ask the user which mode to run (interactive dialog, not a config variable)
3. Create a project in CodeForge via the UI (with API-assisted config)
4. Converse with the CodeForge agent in the chat to establish goals
5. Let the CodeForge agent derive a roadmap from those goals (with fallback assistance)
6. Monitor autonomous execution
7. Verify the results across 4 dimensions (functional, quality, semantic, cross-language)
8. Produce a structured report with bugs documented

### Design Goals

- **Primary: Showcase/Benchmark (A)** — Prove that CodeForge can autonomously build a real multi-language project
- **Secondary: Regression Test (B)** — Repeatable with the same parameters for CI/release validation
- **Secondary: Eval Framework (C)** — Compare different LLMs using the same testplan

### Mode Selection

Claude Code asks the user at the start of Phase 0:

> "Which mode would you like to run?
> **A** — Showcase: Weather Dashboard (Python FastAPI + TypeScript/SolidJS, predefined goals)
> **D** — Free Choice: The CodeForge agent decides the project and languages (minimum 2 languages)"

This is an interactive question, not a hardcoded variable.

---

## 2. Architecture

### Actors

| Actor | Role | Channel |
|---|---|---|
| **User** | Selects mode, observes, receives report | Terminal (Claude Code session) |
| **Claude Code** | Orchestrator — drives UI, converses with CodeForge agent, verifies results | Playwright-MCP + Workspace filesystem |
| **CodeForge Agent** | Developer — understands goals, creates roadmap, writes code | CodeForge Chat UI (agentic mode) |
| **CodeForge Platform** | Infrastructure — project mgmt, NATS, agent execution, tool dispatch | Docker services |

### Flow

```
User selects mode (A/D)
    |
    v
Claude Code --> Playwright-MCP --> CodeForge UI
    |
    v
Phase 0: Setup (services, login, model selection)
    |
    v
Phase 1: Create project (UI + API config patch)
    |
    v
Phase 2: Goal conversation (Claude Code <-> CodeForge Agent via chat)
    |   - Switch to goal_researcher mode
    |   - Iterative dialog until goals are complete
    |   - Approve each GoalProposalCard in the UI
    |   - Validate goals in GoalsPanel
    |   - Switch back to coder mode
    |
    v
Phase 3: Roadmap (Claude Code asks agent to describe structure, creates via UI)
    |   - Agent describes milestones/features in chat
    |   - Claude Code creates roadmap entities via RoadmapPanel UI
    |   - If agent cannot describe roadmap = BUG
    |
    v
Phase 4: Autonomous execution (CodeForge Agent works, Claude Code monitors)
    |   - Optional: bypass-approvals endpoint
    |   - HITL approvals if needed
    |   - Stall detection
    |
    v
Phase 5: Metrics collection (tool calls, duration, costs)
    |
    v
Phase 6-9: Verification (functional, quality, semantic, cross-language)
    |
    v
Phase 10: Report generation
```

### Bug Documentation Rule

**Every time Claude Code must intervene to help the CodeForge agent, this is a bug.** The report must document:

- **What** failed (agent couldn't describe roadmap, wrong file structure, stall, etc.)
- **Where** (phase, chat exchange number, timestamp)
- **Workaround** applied by Claude Code
- **Severity** (INFO = minor hint given, WARNING = significant correction, CRITICAL = Claude Code had to do the work)

Every correction hint counts as an individual INFO bug. If a phase requires more than 3 corrections, escalate the phase assessment to WARNING.

---

## 3. Phase Definitions

### Phase 0: Setup & Mode Selection

**Steps:**

1. Claude Code asks the user: "Which mode? A (Showcase) or D (Free Choice)?"
2. Start Docker services: `docker compose up -d postgres nats litellm`
3. Resolve container IPs (WSL2 workaround):
   ```bash
   NATS_IP=$(docker inspect codeforge-nats | grep -m1 '"IPAddress"' | grep -oP '[\d.]+')
   LITELLM_IP=$(docker inspect codeforge-litellm | grep -m1 '"IPAddress"' | grep -oP '[\d.]+')
   POSTGRES_IP=$(docker inspect codeforge-postgres | grep -m1 '"IPAddress"' | grep -oP '[\d.]+')
   ```
4. Purge NATS JetStream (clean state):
   ```bash
   # Option A: via NATS CLI (if installed)
   nats stream purge CODEFORGE --force --server=nats://${NATS_IP}:4222
   # Option B: via Python helper
   python -c "
   import asyncio, nats
   async def purge():
       nc = await nats.connect('nats://${NATS_IP}:4222')
       js = nc.jetstream()
       await js.purge_stream('CODEFORGE')
       await nc.close()
   asyncio.run(purge())
   "
   ```
5. Start Go backend: `APP_ENV=development go run ./cmd/codeforge/`
6. Verify NATS consumers: `curl http://${NATS_IP}:8222/jsz?consumers=1`
7. Start Python worker with correct env vars:
   ```bash
   PYTHONPATH=/workspaces/CodeForge/workers \
     NATS_URL="nats://${NATS_IP}:4222" \
     LITELLM_BASE_URL="http://${LITELLM_IP}:4000" \
     LITELLM_MASTER_KEY="sk-codeforge-dev" \
     DATABASE_URL="postgresql://codeforge:codeforge_dev@${POSTGRES_IP}:5432/codeforge" \
     CODEFORGE_ROUTING_ENABLED=false \
     APP_ENV=development \
     .venv/bin/python -m codeforge.consumer
   ```
8. Start frontend: `cd frontend && npm run dev`
9. Open Playwright-MCP browser: `http://host.docker.internal:3000`
10. Login: `admin@localhost` / `Changeme123`
11. Health check — verify all 6 services:
    - PostgreSQL: `pg_isready -h ${POSTGRES_IP}` or `docker inspect codeforge-postgres` shows running
    - NATS: `curl http://${NATS_IP}:8222/varz` returns JSON
    - LiteLLM: `curl http://${LITELLM_IP}:4000/health` returns OK
    - Go backend: `curl http://localhost:8080/health` shows `dev_mode: true`
    - Python worker: `curl http://${NATS_IP}:8222/jsz?consumers=1` shows active consumers
    - Frontend: browser loaded, no errors in console

**Validation:** All 6 services running with verified health checks, browser logged in.

**State variables set:**
```
MODE = "A" or "D"  (from user's answer)
ENV = { nats_ip, litellm_ip, postgres_ip, backend_pid, worker_pid, frontend_pid }
```

---

### Phase 1: Project Setup

**Steps:**

1. Navigate to Projects page
2. Click "New Project"
3. Fill in via UI:
   - Name: `multi-lang-showcase` (Mode A) or `multi-lang-freeform` (Mode D)
   - Set autonomy level to 4 (available in UI)
4. After project is created, **patch config via API** (policy_preset and execution_mode are NOT available in the UI):
   ```
   PATCH /api/v1/projects/{PROJECT_ID}
   {
     "config": {
       "policy_preset": "trusted-mount-autonomous",
       "execution_mode": "mount"
     }
   }
   ```
   Execute via `browser_evaluate(fetch(...))` or direct API call.
5. Set workspace path (local directory, e.g. `/tmp/codeforge-test-{timestamp}`)
6. Adopt the workspace: `POST /api/v1/projects/{PROJECT_ID}/adopt` with `{"path": "<workspace_path>"}`
7. Select model via `/model` command in the chat:
   - Local: `/model lm_studio/qwen/qwen3-30b-a3b`
   - Cloud: `/model anthropic/claude-sonnet-4-20250514` or `/model openai/gpt-4o`

**Validation:**
- Project visible in Dashboard
- Workspace path set and adopted
- Config shows autonomy level 4, policy_preset trusted-mount-autonomous, execution_mode mount
- Model set and confirmed

**State variables set:**
```
PROJECT_ID = "<uuid>"
WORKSPACE = "<path>"
CONVERSATION_ID = "<uuid>"  (from the chat session opened during model selection)
MODEL = "<model name>"
```

---

### Phase 2: Goal Conversation

Claude Code opens the project chat and converses with the CodeForge agent to establish goals.

**Step 0: Switch to goal_researcher mode**

Claude Code types `/mode goal_researcher` in the chat input. This switches the agent to a mode that has the `propose_goal` tool and uses a goal-discovery system prompt.

**Mode A — Showcase (Weather Dashboard):**

Claude Code sends an initial message like:
> "I need a Real-Time Weather Dashboard application. It must use two programming languages that communicate with each other:
> 1. **Python FastAPI Backend** — fetches weather data from a public API (e.g. OpenWeatherMap or wttr.in), caches responses, and serves them via a REST API
> 2. **TypeScript/SolidJS Frontend** — displays the weather data as an interactive dashboard with charts, current conditions, and a search/filter for cities
>
> The backend and frontend must communicate via REST API. Both must have tests. The project must be runnable locally."

**Mode D — Free Choice:**

Claude Code sends:
> "I need you to design and build a useful software project that uses at least two different programming languages. The languages must actively communicate with each other (via API, message queue, IPC, or similar). Propose a project idea, explain your choice of languages, and describe the architecture before we start."

**Conversation Loop:**

1. Claude Code sends the initial description (or request for proposal)
2. CodeForge agent responds — may ask clarifying questions, or may immediately propose goals (both are valid at autonomy level 4)
3. If the agent asks questions: Claude Code answers them
4. As the agent proposes goals via `GoalProposalCard` components in the chat stream, **Claude Code clicks the "Approve" button on each card** (unless the goal is clearly wrong, in which case click "Reject" and give feedback in the chat)
5. After all proposals are processed, Claude Code checks the GoalsPanel in the UI:
   - Are there at least 5 goals?
   - Are both languages explicitly mentioned?
   - Is there a goal for cross-language integration?
   - Do the goals cover: project setup, backend, frontend, tests, integration?
6. If goals are incomplete or incorrect:
   - Claude Code gives hints in the chat: "I notice the goals don't mention testing. Can you add a goal for test coverage?"
   - Wait for new `GoalProposalCard` and approve/reject
7. If goals are complete and correct: proceed to Phase 3

**Step Final: Switch back to coder mode**

Claude Code types `/mode coder` in the chat input to switch back to implementation mode.

**Exit Criteria:**
- Minimum 5 goals in GoalsPanel (approved via GoalProposalCard clicks)
- Both languages explicitly mentioned in goals
- Cross-language integration goal present
- Claude Code confirms: "Goals look complete and correct"

**Bug Documentation:**
- Every correction hint = INFO bug (documented individually)
- Goals completely wrong after 3 correction rounds = WARNING bug
- Claude Code has to manually suggest all goals = CRITICAL bug

**State variables set:**
```
GOAL_COUNT = <number>
GOALS_VALIDATED = true
```

---

### Phase 3: Roadmap Creation

**Important:** The CodeForge agent does NOT have a tool to create roadmap entities (milestones, features) directly. The agent can only describe the desired roadmap structure in the chat. Claude Code must create the roadmap entities via the RoadmapPanel UI.

**Steps:**

1. Claude Code sends in the chat: "Based on the goals we've established, describe a roadmap with milestones and features. Group the work into logical phases (e.g. Backend, Frontend, Integration, Testing)."
2. Wait for the CodeForge agent to describe the roadmap structure in the chat
3. Claude Code creates the roadmap via the RoadmapPanel UI:
   - Click "Create Roadmap" (or equivalent)
   - Add milestones based on the agent's description
   - Add features to each milestone based on the agent's description
4. Verify in the Roadmap UI:
   - At least 2 milestones
   - Features assigned to milestones
   - Features cover all goals
   - Both languages represented

**Note:** Claude Code creating the roadmap via UI is **expected behavior** (not a bug) because the agent lacks the tool. The bug trigger is if the **agent cannot describe** the roadmap structure.

**Fallback Escalation (BUG triggers):**

| Trigger | Action | Severity |
|---|---|---|
| 30s no response to roadmap request | Claude Code repeats the request | INFO |
| Agent's roadmap description is incomplete (missing language/phases) | Claude Code asks for clarification in chat | WARNING |
| Agent's roadmap description is completely wrong or incoherent | Claude Code describes the desired structure and asks agent to confirm | WARNING |
| Agent cannot describe a roadmap at all (hallucination, stall, refusal) | Claude Code designs the roadmap independently | CRITICAL |

**Exit Criteria:**
- Roadmap visible in UI
- At least 2 milestones
- Features cover all goals
- Both languages represented in milestone/feature structure

**State variables set:**
```
ROADMAP_OK = true
MILESTONES = <count>
FEATURES = <count>
```

---

### Phase 4: Autonomous Execution

**Steps:**

1. (Optional) Call bypass-approvals endpoint to prevent HITL interruptions:
   ```
   POST /api/v1/conversations/{CONVERSATION_ID}/bypass-approvals
   ```
   Document in the report whether this was used.
2. Claude Code sends in the chat: "The roadmap is set up. Please start implementing it now. Begin with the first milestone."
3. CodeForge agent begins autonomous execution (Autonomy Level 4)
4. Claude Code monitors via the UI and API

**Monitoring Loop (check every 30 seconds):**

Primary: Use API monitoring (`GET /api/v1/conversations/{CONVERSATION_ID}/messages`) — more reliable than browser polling. Use browser (`browser_snapshot`) only for specific UI interactions (HITL approval clicks, checking visual state).

If Playwright-MCP disconnects during monitoring, continue with API-only monitoring and reconnect the browser when needed for UI interactions.

| Check | Action |
|---|---|
| New messages in chat (API or browser)? | Agent is working — continue monitoring |
| HITL approval requested? | Click Approve in UI (or approve via API: `POST /api/v1/runs/{CONVERSATION_ID}/approve/{callId}` with `{"decision": "allow"}`). Document as WARNING bug if bypass-approvals was active |
| Stall detected (no new tool calls for 10 min)? | Document as CRITICAL bug. Give hint in chat if possible |
| Error messages? | Document. Observe if agent self-corrects |
| Agent reports "done"? | Proceed to Phase 5 |

**Timeout:**
- Mode A: Maximum 120 minutes (local models may be slow)
- Mode D: Maximum 180 minutes
- If timeout reached: stop, document, proceed to verification with whatever exists

**State variables set:**
```
EXEC_STARTED = true
EXEC_DURATION = <minutes>
EXEC_STATUS = "completed" | "stalled" | "timeout" | "error"
BYPASS_APPROVALS_USED = true | false
```

---

### Phase 5: Metrics Collection

Collected during Phase 4, summarized here:

| Metric | How |
|---|---|
| Tool calls by type | Count from chat messages (write_file, read_file, edit_file, bash, search_files, glob_files, list_directory) |
| LLM iterations | Count reasoning steps |
| Total duration | Start to finish timestamp |
| Cost | From cost badge in UI (or $0.00 for local models) |
| Files created/modified | `git status` or filesystem scan in workspace |
| Git commits | `git log` in workspace |
| Self-corrections | Count `edit_file` calls targeting files created by `write_file` in the same session (proxy metric) |
| Errors encountered | Count error messages in chat |

**State variables set:**
```
TOOL_CALLS = { write: N, read: N, edit: N, bash: N, search: N, glob: N, listdir: N }
METRICS = { duration, cost, files, commits, self_corrections, errors }
```

---

### Phase 6: Functional Verification

Claude Code inspects the workspace and runs the generated code.

**Step 6.0 (both modes): Workspace Discovery**

Before running any checks, Claude Code inspects the workspace structure:
- List all files and directories
- Identify languages present (file extensions, config files like `pyproject.toml`, `package.json`, `go.mod`, `Cargo.toml`)
- Identify entry points (read main files, check scripts in `package.json` / `pyproject.toml`)
- Identify test frameworks and test file locations
- Identify build/start commands

This discovery step informs which specific commands to run in subsequent checks.

**Mode A (Weather Dashboard) — Specific Checks:**

| # | Check | Command / Method | PASS when |
|---|---|---|---|
| 6.1 | Backend files exist | `ls` workspace | Python files + `requirements.txt` or `pyproject.toml` present |
| 6.2 | Frontend files exist | `ls` workspace | TS/JS files + `package.json` present |
| 6.3 | Backend dependencies install | `pip install -r requirements.txt` or `poetry install` (based on discovery) | Exit 0 |
| 6.4 | Frontend dependencies install | `npm install` | Exit 0 |
| 6.5 | Backend starts | Run discovered entry point (e.g. `uvicorn main:app`, `python app.py`, `fastapi dev`) | Process runs, no crash within 5s |
| 6.6 | Backend API responds | `curl http://localhost:PORT/` (discover port from code) | HTTP 200, JSON response |
| 6.7 | Frontend builds | `npm run build` | Exit 0, no errors |
| 6.8 | Backend tests exist | Search for `test_*.py` or `*_test.py` anywhere in workspace | At least 1 test file |
| 6.9 | Frontend tests exist | Search for `*.test.*` or `*.spec.*` anywhere in workspace | At least 1 test file |
| 6.10 | Backend tests pass | `pytest` (from discovered test directory) | All tests PASS |
| 6.11 | Frontend tests pass | `npm test` or `npx vitest` (based on package.json) | All tests PASS |

**Mode D (Free Choice) — Dynamic Checks:**

Claude Code uses the workspace discovery from Step 6.0 to determine:
- Which languages are present (file extensions, config files)
- What the entry points are (main files, package managers)
- What test frameworks are used
- What build commands exist

Then runs equivalent checks dynamically:
- Dependencies install for each language
- Each component starts without crash
- Tests exist and pass for each language
- At least one integration point exists between languages

**Result per check:** PASS / FAIL / SKIP (with reason)

---

### Phase 7: Code Quality Verification

**Step 7.0: Determine applicable checks based on discovered languages from Phase 6.**

For Python projects:

| # | Check | Tool | PASS when |
|---|---|---|---|
| 7.1 | Python lint | `ruff check .` | 0 errors (warnings OK) |
| 7.2 | Python type check | `mypy .` or `pyright` (if configured in project) | No critical errors |

For TypeScript projects:

| # | Check | Tool | PASS when |
|---|---|---|---|
| 7.3 | TypeScript type check | `npx tsc --noEmit` | Exit 0 |
| 7.4 | TypeScript lint | `npx eslint .` (if eslint configured) | 0 errors |
| 7.5 | No `any` type hacks | `grep -rnP ": any\b|as any\b|<any>" --include="*.ts" --include="*.tsx"` | None or justified |

For all projects:

| # | Check | Tool | PASS when |
|---|---|---|---|
| 7.6 | No TODO/FIXME/HACK | `grep -rn "TODO\|FIXME\|HACK" --include="*.py" --include="*.ts" --include="*.tsx" --include="*.js"` | None or justified |
| 7.7 | Dependencies resolved | Install commands from Phase 6 succeeded | No unresolved deps |
| 7.8 | Project structure | Visual inspection | Sensible directory layout (not all files in root) |

**Mode D adaptation:** Replace Python/TypeScript specific checks with equivalent checks for whatever languages the agent chose (e.g. `go vet`, `cargo clippy`, `golangci-lint`).

---

### Phase 8: Semantic Verification (LLM-as-Judge)

Claude Code reads the generated source code and evaluates:

**Mode A:**

| # | Check | Method | PASS when |
|---|---|---|---|
| 8.1 | Backend fulfills goals | Read Python source files | Fetches weather data, has caching, serves REST API |
| 8.2 | Frontend fulfills goals | Read TypeScript source files | Displays weather data, has charts, has city search/filter |
| 8.3 | Architecture is sensible | Read all files | Clean separation (no god-files), proper module structure |
| 8.4 | Code is non-trivial | Count LOC, analyze logic | Not just boilerplate/stubs — real implementation |

**Mode D:**

| # | Check | Method | PASS when |
|---|---|---|---|
| 8.1 | Result matches proposal | Compare code to goals from Phase 2 | Agent built what it proposed |
| 8.2 | Both languages are meaningful | Read source for each language | Neither language is trivial/stub (each has real logic) |
| 8.3 | Architecture is sensible | Read all files | Clean separation, proper structure |
| 8.4 | Code is non-trivial | Count LOC, analyze logic | Real implementation, not boilerplate |

---

### Phase 9: Cross-Language Integration Verification

The hardest test — do the languages actually work together?

| # | Check | Method | PASS when |
|---|---|---|---|
| 9.1 | API contract match | Read frontend fetch/request URLs + backend route definitions | URLs and methods match |
| 9.2 | Data format match | Read backend response types + frontend interface/type definitions | Field names and types align |
| 9.3 | Live roundtrip (stretch goal) | Start backend, start frontend, open in browser | Data flows from backend to frontend visibly |
| 9.4 | Error handling | Stop backend, check frontend | Frontend shows error message, does not crash |
| 9.5 | Shared types/schema | Check for shared type definitions or API schema (OpenAPI, etc.) | Bonus: shared contract exists |

**Note on 9.3 (Live roundtrip):** This requires both components to start successfully (verified in Phase 6, checks 6.5-6.7). If either component failed to start in Phase 6, SKIP this check and note the reason. Running both simultaneously requires managing two background processes and waiting for ports to become available.

**Note on 9.4 (Error handling):** SKIP if 9.3 was skipped.

**Mode D adaptation:** Same checks but adapted to whatever communication mechanism the agent chose (REST, WebSocket, gRPC, message queue, etc.). Claude Code determines the communication mechanism from workspace discovery.

---

### Phase 10: Report Generation

Save a structured report as `docs/testing/YYYY-MM-DD-multi-language-autonomous-report.md`.

**Report Structure:**

```markdown
# Multi-Language Autonomous Project Report

**Date:** YYYY-MM-DD
**Mode:** A (Showcase) / D (Free Choice)
**Model:** <LLM used by CodeForge agent>
**Project:** <project name and description>
**Languages:** <language 1> + <language 2> [+ <language N>]
**Bypass-Approvals:** yes / no

## Phase Results

| Phase | Result | Notes |
|-------|--------|-------|
| 0 - Setup & Mode Selection | PASS/FAIL | ... |
| 1 - Project Setup | PASS/FAIL | ... |
| 2 - Goal Conversation | PASS/FAIL | Rounds: N, corrections: N |
| 3 - Roadmap Creation | PASS/FAIL | Agent described: yes/no |
| 4 - Autonomous Execution | PASS/FAIL | Status: completed/stalled/timeout |
| 5 - Metrics | N/A | ... |
| 6 - Functional Verification | PASS/PARTIAL/FAIL | X/Y checks passed |
| 7 - Code Quality | PASS/PARTIAL/FAIL | X/Y checks passed |
| 8 - Semantic Verification | PASS/PARTIAL/FAIL | X/Y checks passed |
| 9 - Cross-Language Integration | PASS/PARTIAL/FAIL | X/Y checks passed |

## Metrics

| Metric | Value |
|--------|-------|
| Model | <model name> |
| Total tool calls | N |
| Tool call breakdown | write: N, read: N, edit: N, bash: N, ... |
| LLM iterations | N |
| Duration | Xm |
| Cost | $X.XX |
| Files created | N |
| Lines of code | N |
| Git commits | N |
| Self-corrections (proxy) | N |

## Verification Summary

| Category | Passed | Total | Score |
|----------|--------|-------|-------|
| Functional (Phase 6) | X | Y | X/Y |
| Quality (Phase 7) | X | Y | X/Y |
| Semantic (Phase 8) | X | Y | X/Y |
| Cross-Language (Phase 9) | X | Y | X/Y |
| **Overall** | **X** | **Y** | **X/Y** |

## Bug List

| # | Phase | Severity | Description | Workaround | Status |
|---|-------|----------|-------------|------------|--------|
| 1 | ... | INFO/WARNING/CRITICAL | ... | ... | OPEN/FIXED |

## Goal Conversation Log

<Key exchanges from Phase 2 — how many rounds, what corrections were needed>

## Conclusion

<Overall assessment: Is this showcase-worthy? What worked well? What needs fixing?>
```

---

## 4. Mode Comparison

| Aspect | A (Showcase) | D (Free Choice) |
|---|---|---|
| Project | Weather Dashboard | Agent decides |
| Languages | Python FastAPI + TypeScript/SolidJS | Agent decides (min. 2) |
| Goal prompt | Detailed, specific | Open-ended |
| Expected files | Known checklist | Dynamically discovered |
| Verification: functional | Specific checks (API endpoints, build commands) | Generic checks (starts? builds? tests?) |
| Verification: semantic | "Shows weather data? Charts? Filters?" | "Does what the agent proposed?" |
| Verification: cross-language | "Frontend calls FastAPI?" | "Languages communicate?" |
| Timeout | 120 min | 180 min |
| Difficulty | Medium (clear spec) | Hard (open-ended) |
| Best for | Showcase demos, regression testing | LLM comparison, capability evaluation |

---

## 5. Prerequisites

| Requirement | Details |
|---|---|
| Playwright-MCP | Connected (`/mcp` shows playwright-mcp) |
| LLM API key | At least one configured (ANTHROPIC_API_KEY, OPENAI_API_KEY, or local model) |
| Docker | Running with `postgres`, `nats`, `litellm` images available |
| Disk space | ~2GB free for workspace + node_modules + venv |
| Time | 60-180 minutes depending on mode and model |

---

## 6. Known Limitations & Risks

| Risk | Mitigation |
|---|---|
| Playwright-MCP disconnects during long waits | Monitor via API (`GET /conversations/{id}/messages`) as primary, reconnect browser for UI interactions |
| Local models (LM Studio) very slow | Extended timeouts (120-180 min), do not abort early |
| NATS message backlog from previous runs | Purge in Phase 0 (mandatory, concrete commands provided) |
| CodeForge agent may not understand multi-language requirement | Explicit in goal prompt, validate in GoalsPanel before proceeding |
| Frontend build may need specific Node version | Check `package.json` engines field |
| Weather API may require API key | Use keyless alternatives (wttr.in) or mock data |
| Agent has no roadmap creation tool | Expected: agent describes, Claude Code creates via UI (Phase 3) |
| Goal proposals need manual approval clicks | Claude Code must click Approve on each GoalProposalCard (Phase 2) |
| policy_preset/execution_mode not in UI | API PATCH after project creation (Phase 1) |

---

## 7. Success Criteria

**Showcase-worthy (Mode A):**
- Phase 6 (Functional): >= 9/11 checks PASS
- Phase 7 (Quality): >= 6/8 checks PASS
- Phase 8 (Semantic): >= 3/4 checks PASS
- Phase 9 (Cross-Language): >= 3/5 checks PASS (including live roundtrip)
- Bug list: No CRITICAL bugs, max 3 WARNING bugs

**Evaluation-worthy (Mode D):**
- Phase 6: >= 70% checks PASS
- Phase 9: >= 2/5 checks PASS (including API contract match)
- Agent chose and used 2+ languages meaningfully
- Bug list documented for LLM comparison

---

## 8. Relationship to Existing Testplans

This testplan extends the existing `autonomous-goal-to-program-testplan.md` (S1-S4 scenarios) with:

- **Multi-language requirement** (S1-S4 are single-language)
- **Goal conversation** (S1-S4 skip straight to execution)
- **4-tier verification** (S1-S4 only do functional checks)
- **Cross-language integration testing** (entirely new)
- **Bug documentation protocol** (every agent assist = bug)
- **Mode selection** (A vs D, interactive)
- **Mode switching** (`/mode goal_researcher` then `/mode coder`)
- **Explicit model selection** (`/model` command in Phase 1)
- **GoalProposalCard approval** (click Approve/Reject in UI)
- **Roadmap via UI** (agent describes, Claude Code creates — agent lacks the tool)

The existing S1-S4 testplan remains valid for single-language scenarios.
