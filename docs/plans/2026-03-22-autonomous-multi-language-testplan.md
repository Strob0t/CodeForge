# Autonomous Multi-Language Testplan — Implementation Plan

> **For agentic workers:** Steps use checkbox (`- [ ]`) syntax for tracking. Execute task-by-task, commit after each task.

**Goal:** Convert the design spec into an executable runbook that Claude Code follows to drive CodeForge via Playwright-MCP, build a multi-language project autonomously, and verify the results.

**Architecture:** Single Markdown testplan document with 11 phases (0-10), decision trees per phase, concrete Playwright-MCP commands, state variable tracking, and a report template. Modeled after the existing `autonomous-goal-to-program-testplan.md` format but extended with goal conversation, 4-tier verification, and cross-language integration checks.

**Tech Stack:** Markdown runbook, Playwright-MCP browser commands, CodeForge REST API, bash verification scripts.

**Spec:** `docs/specs/2026-03-22-autonomous-multi-language-testplan-design.md`
**Reference:** `docs/testing/autonomous-goal-to-program-testplan.md` (existing S1-S4 testplan format)

---

## File Structure

| File | Action | Purpose |
|---|---|---|
| `docs/testing/autonomous-multi-language-testplan.md` | Create | The executable runbook (main deliverable) |
| `docs/todo.md` | Modify | Add testplan entry, mark completed |
| `docs/testing/README.md` | Modify (if exists) | Cross-reference new testplan |

---

## Task 1: Testplan Header, How-to-Use, and State Variables

**Files:**
- Create: `docs/testing/autonomous-multi-language-testplan.md`

- [ ] **Step 1: Create testplan file with header and usage section**

Write the document header, "How to Use This Document" section, prerequisites, execution instructions, and state variables block. Follow the exact format of the existing testplan (`autonomous-goal-to-program-testplan.md` lines 1-36).

```markdown
# Autonomous Multi-Language Project Test Plan — Claude Code + Playwright-MCP

**Date:** 2026-03-22
**Type:** Interactive AI QA Test (Claude Code drives browser via playwright-mcp)
**Scope:** Multi-language autonomous workflow: Chat -> Goal Conversation -> Roadmap -> Execution -> Working Multi-Language Program -> 4-Tier Verification
**Coverage:** 11 phases (0-10), sequential, real LLM calls (no mocks)
**Design Spec:** `docs/specs/2026-03-22-autonomous-multi-language-testplan-design.md`

---

## How to Use This Document

This is a **runbook for Claude Code sessions** using playwright-mcp tools. Claude Code drives the CodeForge frontend interactively, converses with the CodeForge agent to establish goals, monitors autonomous execution, and verifies results across 4 dimensions.

**Prerequisites:**
- Playwright-MCP connected (`/mcp` shows playwright-mcp)
- At least one LLM API key configured (ANTHROPIC_API_KEY, OPENAI_API_KEY, or local model via LM Studio/Ollama)
- **Claude Code is responsible for starting ALL required services** — see Phase 0
- Docker running with `postgres`, `nats`, `litellm` images available
- ~2GB free disk space for workspace + node_modules + venv

**Execution:**
- Follow phases 0-10 sequentially (strict order — each gates the next)
- Each phase: execute Steps, check Validation, follow Decision Tree on failure
- Record results as PASS / PARTIAL / SKIP / FAIL per phase
- **Every time Claude Code intervenes to help the CodeForge agent = BUG** (document with severity)
- Save report to `docs/testing/YYYY-MM-DD-multi-language-autonomous-report.md`

**State Variables** (carried across phases):
```
MODE = ""            # Phase 0: "A" (Showcase) or "D" (Free Choice)
MODEL = ""           # Phase 1: LLM model name
ENV = {}             # Phase 0: container IPs, PIDs
PROJECT_ID = ""      # Phase 1: created project UUID
WORKSPACE = ""       # Phase 1: project workspace path
CONVERSATION_ID = "" # Phase 1: chat session UUID
GOAL_COUNT = 0       # Phase 2: approved goals
GOALS_VALIDATED = false # Phase 2: all goals checked
ROADMAP_OK = false   # Phase 3: roadmap created
MILESTONES = 0       # Phase 3: milestone count
FEATURES = 0         # Phase 3: feature count
EXEC_STARTED = false # Phase 4: execution dispatched
EXEC_DURATION = 0    # Phase 4: minutes
EXEC_STATUS = ""     # Phase 4: completed/stalled/timeout/error
BYPASS_APPROVALS_USED = false # Phase 4
TOOL_CALLS = {}      # Phase 5: counts by type
METRICS = {}         # Phase 5: duration, cost, files, commits
BUGS = []            # All phases: accumulated bug list
```
```

- [ ] **Step 2: Add Phase Dependency Graph**

```markdown
---

## Phase Dependency Graph

```
Phase 0: Setup & Mode Selection (services, login, user selects A/D)
    |
    v
Phase 1: Project Setup (create project, patch config, set model)
    |
    v
Phase 2: Goal Conversation (switch to goal_researcher, converse, approve goals)
    |
    v
Phase 3: Roadmap Creation (agent describes, Claude Code creates via UI)
    |
    v
Phase 4: Autonomous Execution (agent implements, Claude Code monitors)
    |    \
    |     v
    |   Phase 4b: HITL & Stall Handling
    |     |
    v     v
Phase 5: Metrics Collection (tool calls, duration, costs)
    |
    v
Phase 6: Functional Verification (files exist, deps install, builds, tests)
    |
    v
Phase 7: Code Quality Verification (lint, types, structure)
    |
    v
Phase 8: Semantic Verification (LLM-as-Judge: goals fulfilled?)
    |
    v
Phase 9: Cross-Language Integration (API contract, live roundtrip)
    |
    v
Phase 10: Report Generation
```
```

- [ ] **Step 3: Commit**

```bash
git add docs/testing/autonomous-multi-language-testplan.md
git commit -m "docs(testing): multi-language testplan — header, usage, state variables"
```

---

## Task 2: Phase 0 — Setup & Mode Selection

**Files:**
- Modify: `docs/testing/autonomous-multi-language-testplan.md`

- [ ] **Step 1: Write Phase 0 with all steps, decision tree, and validation**

Include:
1. Interactive mode selection question (Claude Code asks user)
2. Docker services startup with concrete commands
3. Container IP resolution (WSL2 workaround) with exact `docker inspect` commands
4. NATS JetStream purge with two options (NATS CLI and Python script)
5. Go backend startup command
6. NATS consumer verification with curl command
7. Python worker startup with all env vars (LITELLM_BASE_URL, NATS_URL, DATABASE_URL, CODEFORGE_ROUTING_ENABLED=false)
8. Frontend startup command
9. Playwright-MCP browser open (`http://host.docker.internal:3000`)
10. Login step with credentials
11. Health check verification for all 6 services (concrete commands per service)

Decision Tree:
```
Docker fails to start?
  -> Check Docker daemon: `docker info`
  -> Check images: `docker compose pull`
  -> Retry: `docker compose up -d`

NATS consumer not found?
  -> Restart Go backend (consumers auto-recreate)
  -> Verify: `curl http://${NATS_IP}:8222/jsz?consumers=1`

Login fails?
  -> Check backend health: `curl http://localhost:8080/health`
  -> Verify dev_mode: true in response
```

- [ ] **Step 2: Commit**

```bash
git add docs/testing/autonomous-multi-language-testplan.md
git commit -m "docs(testing): multi-language testplan — Phase 0 setup & mode selection"
```

---

## Task 3: Phase 1 — Project Setup

**Files:**
- Modify: `docs/testing/autonomous-multi-language-testplan.md`

- [ ] **Step 1: Write Phase 1 with UI + API hybrid steps**

Include:
1. Navigate to Projects page via Playwright (`browser_click` on Projects nav item)
2. Click "New Project" button
3. Fill project name via `browser_fill_form` — `multi-lang-showcase` (A) or `multi-lang-freeform` (D)
4. Set autonomy level to 4 via UI
5. Submit project creation
6. Capture PROJECT_ID: extract from browser URL via `browser_evaluate(window.location.pathname.split('/').pop())` or from the API response after creation
7. **API PATCH** for policy_preset and execution_mode (not available in UI):
   ```
   browser_evaluate: fetch('/api/v1/projects/' + PROJECT_ID, {
     method: 'PATCH',
     headers: {'Content-Type': 'application/json', 'Authorization': 'Bearer ' + TOKEN},
     body: JSON.stringify({config: {policy_preset: 'trusted-mount-autonomous', execution_mode: 'mount'}})
   })
   ```
8. Create workspace directory: `mkdir -p /tmp/codeforge-multi-lang-{timestamp}`
9. Adopt workspace via API: `POST /api/v1/projects/{PROJECT_ID}/adopt` with `{"path": "..."}`
10. Open project chat panel
11. Set model via `/model` command in chat input (`browser_fill_form` + `browser_press_key Enter`)
12. Capture CONVERSATION_ID: extract from the chat panel URL (e.g. `/projects/{id}/chat/{convId}`) via `browser_evaluate(window.location.pathname.split('/').pop())` or from the messages API response

Decision Tree:
```
Project creation fails?
  -> browser_snapshot to see error message
  -> Check backend logs for constraint violations
  -> Retry with different name

Adopt fails?
  -> Verify workspace directory exists: `ls -la /tmp/codeforge-multi-lang-*`
  -> Check path is absolute

/model command not recognized?
  -> Verify chat panel is open
  -> Try typing in message input and pressing Enter
```

- [ ] **Step 2: Commit**

```bash
git add docs/testing/autonomous-multi-language-testplan.md
git commit -m "docs(testing): multi-language testplan — Phase 1 project setup"
```

---

## Task 4: Phase 2 — Goal Conversation

**Files:**
- Modify: `docs/testing/autonomous-multi-language-testplan.md`

- [ ] **Step 1: Write Phase 2 with conversation loop and GoalProposalCard handling**

Include:
1. Switch to goal_researcher mode: type `/mode goal_researcher` in chat input, press Enter
2. Mode A prompt (Weather Dashboard — full text from design spec)
3. Mode D prompt (Free Choice — full text from design spec)
4. Send initial message via `browser_fill_form` on chat input + `browser_press_key Enter`
5. **Conversation Loop:**
   - Wait for agent response (`browser_snapshot` every 15s to check for new messages)
   - If agent asks questions: answer them via chat input
   - If `GoalProposalCard` appears: click "Approve" button (or "Reject" if clearly wrong)
   - After each approval: `browser_snapshot` to check GoalsPanel count
6. **GoalsPanel Validation** (after all proposals processed):
   - Navigate to GoalsPanel or check goal count via `browser_snapshot`
   - Verify: >= 5 goals, both languages mentioned, cross-language integration goal present
7. If goals incomplete: type correction hint in chat, wait for new proposals, approve/reject
8. Switch back: type `/mode coder` in chat input, press Enter

**Bug Severity Table:**
| Trigger | Severity | Document |
|---|---|---|
| Each correction hint given | INFO | What was missing, what hint was given |
| Goals wrong after 3 correction rounds | WARNING | Phase assessment escalated |
| Claude Code suggests all goals manually | CRITICAL | Agent failed to understand requirements |

Decision Tree:
```
Agent doesn't respond after 60s?
  -> browser_snapshot to check for errors
  -> Check NATS consumers (API monitoring)
  -> If stalled: type "Are you there? Please propose goals for this project." in chat

No GoalProposalCard appears (agent responds with text only)?
  -> Agent may not be in goal_researcher mode
  -> Verify mode: check mode indicator in UI via browser_snapshot
  -> If wrong mode: type `/mode goal_researcher` again

GoalsPanel shows 0 goals after approving cards?
  -> Possible: approval didn't register (Playwright click missed)
  -> Retry: browser_snapshot, find Approve button, browser_click again
  -> BUG if cards were approved but goals don't appear
```

- [ ] **Step 2: Commit**

```bash
git add docs/testing/autonomous-multi-language-testplan.md
git commit -m "docs(testing): multi-language testplan — Phase 2 goal conversation"
```

---

## Task 5: Phase 3 — Roadmap Creation

**Files:**
- Modify: `docs/testing/autonomous-multi-language-testplan.md`

- [ ] **Step 1: Write Phase 3 with agent-describes / Claude-creates flow**

Include:
1. Note: Agent has NO tool to create roadmap entities — this is expected, not a bug
2. Send message: "Based on the goals we've established, describe a roadmap with milestones and features. Group the work into logical phases (e.g. Backend, Frontend, Integration, Testing)."
3. Wait for agent response (may take 30-60s)
4. Parse agent's description from chat (via `browser_snapshot`)
5. Navigate to RoadmapPanel in UI
6. Create roadmap via UI:
   - Click "Create Roadmap" button
   - Fill roadmap title
   - For each milestone the agent described: add milestone via UI
   - For each feature: add feature to the appropriate milestone
7. Verify in Roadmap UI: >= 2 milestones, features cover goals, both languages represented

**Fallback Escalation Table:**
| Trigger | Action | Severity |
|---|---|---|
| 30s no response | Repeat request in chat | INFO |
| Description incomplete | Ask for clarification in chat | WARNING |
| Description incoherent | Describe desired structure, ask to confirm | WARNING |
| Agent cannot describe roadmap at all | Claude Code designs independently | CRITICAL |

Decision Tree:
```
Agent describes roadmap but structure is flat (no milestones)?
  -> Ask: "Can you group these into milestones? E.g. Milestone 1: Backend, Milestone 2: Frontend"
  -> Document as WARNING bug

RoadmapPanel UI not found?
  -> Look for tab/button in project view
  -> browser_snapshot to see available panels
  -> If no Roadmap UI: create via API as fallback

Create Milestone button missing?
  -> Check if roadmap must be created first (parent entity)
  -> browser_snapshot to identify UI flow
```

- [ ] **Step 2: Commit**

```bash
git add docs/testing/autonomous-multi-language-testplan.md
git commit -m "docs(testing): multi-language testplan — Phase 3 roadmap creation"
```

---

## Task 6: Phase 4 & 5 — Autonomous Execution & Metrics

**Files:**
- Modify: `docs/testing/autonomous-multi-language-testplan.md`

- [ ] **Step 1: Write Phase 4 (execution) with monitoring loop and stall handling**

Include:
1. Optional: bypass-approvals endpoint (`POST /api/v1/conversations/{CONVERSATION_ID}/bypass-approvals`)
2. Send execution message: "The roadmap is set up. Please start implementing it now. Begin with the first milestone."
3. **Monitoring Loop (every 30 seconds):**
   - Primary: API poll `GET /api/v1/conversations/{CONVERSATION_ID}/messages` (count messages, check for tool calls)
   - Secondary: `browser_snapshot` only for HITL approval clicks and visual checks
   - If Playwright disconnects: continue API-only, reconnect browser when needed
4. HITL handling: if approval requested, click Approve in UI or call `POST /api/v1/runs/{CONVERSATION_ID}/approve/{callId}` with `{"decision": "allow"}`
5. Stall detection: no new tool calls for 10 minutes = CRITICAL bug, try hint in chat
6. Timeout: Mode A = 120 min, Mode D = 180 min
7. Exit when agent reports "done" or stall/timeout

Decision Tree:
```
Agent stalls (no new tool calls for 10 minutes)?
  -> Check last message via API — is it waiting for something?
  -> If HITL pending: approve it
  -> If genuinely stuck: send hint "What's blocking you? Continue with the implementation."
  -> Document as CRITICAL bug

Playwright-MCP disconnects?
  -> Switch to API-only monitoring
  -> Reconnect browser: browser_navigate to host.docker.internal:3000
  -> Re-login if needed

Agent writes to wrong directory?
  -> Check workspace_path in project config
  -> BUG: workspace_path may not be set correctly (known issue from Run 5)
```

- [ ] **Step 2: Write Phase 5 (metrics collection)**

Metrics to collect from the monitoring data:
- Tool calls by type (count from messages API)
- LLM iterations (count reasoning steps)
- Duration (Phase 4 start to finish)
- Cost (from cost badge in UI or messages API)
- Files created/modified (`git status` in workspace, or `ls -R` + `wc -l`)
- Git commits (`git log --oneline` in workspace)
- Self-corrections proxy (`edit_file` calls targeting files from `write_file`)
- Errors (count error messages)

- [ ] **Step 3: Commit**

```bash
git add docs/testing/autonomous-multi-language-testplan.md
git commit -m "docs(testing): multi-language testplan — Phase 4-5 execution & metrics"
```

---

## Task 7: Phase 6 — Functional Verification

**Files:**
- Modify: `docs/testing/autonomous-multi-language-testplan.md`

- [ ] **Step 1: Write Phase 6 with workspace discovery and verification checks**

Include:

**Step 6.0 — Workspace Discovery (both modes):**
```bash
# Discover project structure
cd ${WORKSPACE}
ls -la
find . -name "*.py" -o -name "*.ts" -o -name "*.tsx" -o -name "*.js" -o -name "*.go" | head -50
cat pyproject.toml 2>/dev/null || cat requirements.txt 2>/dev/null || echo "NO_PYTHON_CONFIG"
cat package.json 2>/dev/null || echo "NO_NODE_CONFIG"
cat go.mod 2>/dev/null || echo "NO_GO_CONFIG"
```

**Mode A checks (11 checks):**
Write each check with concrete commands, PASS criteria, and FAIL handling. Use the discovery results to determine entry points (not hardcoded `uvicorn main:app`).

**Mode D checks (dynamic):**
Write the algorithm Claude Code follows to determine which checks to run based on discovered languages.

Decision Tree for common failures:
```
pip install fails?
  -> Check Python version: `python3 --version`
  -> Check if venv needed: `python3 -m venv .venv && source .venv/bin/activate`
  -> Check requirements format: `cat requirements.txt`

npm install fails?
  -> Check Node version: `node --version`
  -> Check package.json validity: `node -e "require('./package.json')"`
  -> Try: `npm install --legacy-peer-deps`

Backend won't start?
  -> Read entry point file to find the start command
  -> Check for missing env vars (API keys, ports)
  -> Check port conflicts: `lsof -i :8000`

Tests fail?
  -> Read test output to identify which tests fail
  -> Document as PARTIAL (not FAIL) if some tests pass
```

- [ ] **Step 2: Commit**

```bash
git add docs/testing/autonomous-multi-language-testplan.md
git commit -m "docs(testing): multi-language testplan — Phase 6 functional verification"
```

---

## Task 8: Phase 7 & 8 — Quality & Semantic Verification

**Files:**
- Modify: `docs/testing/autonomous-multi-language-testplan.md`

- [ ] **Step 1: Write Phase 7 (code quality)**

Per-language checks with concrete commands:
- Python: `ruff check .`, `mypy .` (if configured)
- TypeScript: `npx tsc --noEmit`, `npx eslint .` (if configured)
- TypeScript any-check: `grep -rnP ": any\b|as any\b|<any>" --include="*.ts" --include="*.tsx"`
- All: `grep -rn "TODO\|FIXME\|HACK" --include="*.py" --include="*.ts" --include="*.tsx" --include="*.js"`
- Structure check: visual inspection via `find . -type f | head -30` and `tree -L 3` (if available)

Mode D adaptation: describe how to select equivalent lint/type tools for Go (`go vet`, `golangci-lint`), Rust (`cargo clippy`), etc.

- [ ] **Step 2: Write Phase 8 (semantic verification / LLM-as-Judge)**

Claude Code reads the generated source code and evaluates:
- Mode A: 4 checks (backend fulfills goals, frontend fulfills goals, architecture sensible, code non-trivial)
- Mode D: 4 checks (matches proposal, both languages meaningful, architecture sensible, code non-trivial)

For each check: describe what Claude Code reads, what it looks for, and the PASS/FAIL criteria. This phase is qualitative (Claude Code uses its own judgment).

- [ ] **Step 3: Commit**

```bash
git add docs/testing/autonomous-multi-language-testplan.md
git commit -m "docs(testing): multi-language testplan — Phase 7-8 quality & semantic verification"
```

---

## Task 9: Phase 9 — Cross-Language Integration Verification

**Files:**
- Modify: `docs/testing/autonomous-multi-language-testplan.md`

- [ ] **Step 1: Write Phase 9 with 5 integration checks**

Check 9.1 — API Contract Match:
```bash
# Find backend routes
grep -rn "app.get\|app.post\|@app.route\|@router" --include="*.py" ${WORKSPACE}
# Find frontend fetch calls
grep -rn "fetch(\|axios\.\|http\." --include="*.ts" --include="*.tsx" ${WORKSPACE}
# Compare: do the URLs and methods match?
```

Check 9.2 — Data Format Match:
```bash
# Find backend response models/types
grep -rn "class.*Model\|TypedDict\|BaseModel\|dict(" --include="*.py" ${WORKSPACE}
# Find frontend interfaces/types
grep -rn "interface \|type " --include="*.ts" --include="*.tsx" ${WORKSPACE}
# Compare: do field names align?
```

Check 9.3 — Live Roundtrip (stretch goal):
- Start backend in background: `python -m uvicorn ... &` (discovered entry point)
- Wait for port to be available: `timeout 10 bash -c 'until curl -s http://localhost:PORT; do sleep 1; done'`
- Start frontend: `npm run dev &` or `npm run preview &`
- Open in Playwright browser: `browser_navigate` to frontend URL
- `browser_snapshot` — verify data from backend appears in frontend
- SKIP if backend or frontend failed to start in Phase 6

Check 9.4 — Error Handling:
- Kill backend process
- `browser_snapshot` frontend — should show error message, not crash
- SKIP if 9.3 was skipped

Check 9.5 — Shared Types/Schema (bonus):
- Search for OpenAPI spec, shared types file, or schema definitions
- PASS if any shared contract exists, SKIP otherwise

Mode D adaptation note for non-REST communication (WebSocket, gRPC, MQ).

- [ ] **Step 2: Commit**

```bash
git add docs/testing/autonomous-multi-language-testplan.md
git commit -m "docs(testing): multi-language testplan — Phase 9 cross-language integration"
```

---

## Task 10: Phase 10 — Report Generation & Success Criteria

**Files:**
- Modify: `docs/testing/autonomous-multi-language-testplan.md`

- [ ] **Step 1: Write Phase 10 with full report template**

Include the complete report template from the design spec (Markdown, ready to copy):
- Header (date, mode, model, project, languages, bypass-approvals)
- Phase Results table
- Metrics table
- Verification Summary table (with scores)
- Bug List table
- Goal Conversation Log section
- Conclusion section

- [ ] **Step 2: Write Success Criteria section**

From design spec:
- Showcase-worthy (Mode A): >= 9/11 functional, >= 6/8 quality, >= 3/4 semantic, >= 3/5 cross-language, no CRITICAL bugs
- Evaluation-worthy (Mode D): >= 70% functional, >= 2/5 cross-language, 2+ languages meaningful

- [ ] **Step 3: Write Mode Comparison table**

Quick reference showing how each mode differs across all phases.

- [ ] **Step 4: Commit**

```bash
git add docs/testing/autonomous-multi-language-testplan.md
git commit -m "docs(testing): multi-language testplan — Phase 10 report & success criteria"
```

---

## Task 11: Documentation Updates

**Files:**
- Modify: `docs/todo.md`

- [ ] **Step 1: Add testplan entry to todo.md**

Add under the appropriate section:
```markdown
- [x] Multi-language autonomous testplan (2026-03-22) — `docs/testing/autonomous-multi-language-testplan.md`
  - Design spec: `docs/specs/2026-03-22-autonomous-multi-language-testplan-design.md`
  - Modes: A (Weather Dashboard showcase) / D (Free choice)
  - 4-tier verification: functional, quality, semantic, cross-language
```

- [ ] **Step 2: Update docs/testing/README.md (if it exists)**

Add a cross-reference to the new testplan:
```markdown
- [autonomous-multi-language-testplan.md](autonomous-multi-language-testplan.md) — Multi-language project (Python+TS), 4-tier verification, Mode A/D
```

- [ ] **Step 3: Final commit**

```bash
git add docs/todo.md docs/testing/README.md
git commit -m "docs: add multi-language autonomous testplan to todo tracker"
```

---

## Task Summary

| Task | Description | Files | Est. Steps |
|---|---|---|---|
| 1 | Header, usage, state variables | Create testplan | 3 |
| 2 | Phase 0: Setup & mode selection | Modify testplan | 2 |
| 3 | Phase 1: Project setup | Modify testplan | 2 |
| 4 | Phase 2: Goal conversation | Modify testplan | 2 |
| 5 | Phase 3: Roadmap creation | Modify testplan | 2 |
| 6 | Phase 4-5: Execution & metrics | Modify testplan | 3 |
| 7 | Phase 6: Functional verification | Modify testplan | 2 |
| 8 | Phase 7-8: Quality & semantic | Modify testplan | 3 |
| 9 | Phase 9: Cross-language integration | Modify testplan | 2 |
| 10 | Phase 10: Report & success criteria | Modify testplan | 4 |
| 11 | Documentation updates | Modify todo.md | 2 |
| **Total** | | | **27 steps** |
