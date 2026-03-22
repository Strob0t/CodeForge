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
- **Claude Code is responsible for starting ALL required services** -- see Phase 0
- Docker running with `postgres`, `nats`, `litellm` images available
- ~2GB free disk space for workspace + node_modules + venv

**Execution:**
- Follow phases 0-10 sequentially (strict order -- each gates the next)
- Each phase: execute Steps, check Validation, follow Decision Tree on failure
- Record results as PASS / PARTIAL / SKIP / FAIL per phase
- **Every time Claude Code intervenes to help the CodeForge agent = BUG** (document with severity)
- Save report to `docs/testing/YYYY-MM-DD-multi-language-autonomous-report.md`

**Bug Severity Levels:**
- **INFO** = minor hint given (each correction counts individually)
- **WARNING** = significant correction, or >3 corrections in one phase
- **CRITICAL** = Claude Code had to do the work instead of the agent

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

---

## Phase 0: Setup & Mode Selection

### Step 0.1: Ask User for Mode

Claude Code asks the user:

> "Which mode would you like to run?
> **A** -- Showcase: Weather Dashboard (Python FastAPI + TypeScript/SolidJS, predefined goals)
> **D** -- Free Choice: The CodeForge agent decides the project and languages (minimum 2 languages)"

Set `MODE` from the user's answer.

### Step 0.2: Start Docker Services

```bash
docker compose up -d postgres nats litellm
```

### Step 0.3: Resolve Container IPs (WSL2 Workaround)

```bash
NATS_IP=$(docker inspect codeforge-nats | grep -m1 '"IPAddress"' | grep -oP '[\d.]+')
LITELLM_IP=$(docker inspect codeforge-litellm | grep -m1 '"IPAddress"' | grep -oP '[\d.]+')
POSTGRES_IP=$(docker inspect codeforge-postgres | grep -m1 '"IPAddress"' | grep -oP '[\d.]+')
```

### Step 0.4: Purge NATS JetStream (Clean State)

```bash
# Option A: via NATS CLI (if installed)
nats stream purge CODEFORGE --force --server=nats://${NATS_IP}:4222

# Option B: via Python helper
.venv/bin/python -c "
import asyncio, nats
async def purge():
    nc = await nats.connect('nats://${NATS_IP}:4222')
    js = nc.jetstream()
    await js.purge_stream('CODEFORGE')
    await nc.close()
asyncio.run(purge())
"
```

### Step 0.5: Start Go Backend

```bash
APP_ENV=development go run ./cmd/codeforge/ &
```

### Step 0.6: Verify NATS Consumers

```bash
curl -s http://${NATS_IP}:8222/jsz?consumers=1 | jq '.streams[0].state.consumer_count'
# Expected: >= 1
```

### Step 0.7: Start Python Worker

```bash
PYTHONPATH=/workspaces/CodeForge/workers \
  NATS_URL="nats://${NATS_IP}:4222" \
  LITELLM_BASE_URL="http://${LITELLM_IP}:4000" \
  LITELLM_MASTER_KEY="sk-codeforge-dev" \
  DATABASE_URL="postgresql://codeforge:codeforge_dev@${POSTGRES_IP}:5432/codeforge" \
  CODEFORGE_ROUTING_ENABLED=false \
  APP_ENV=development \
  .venv/bin/python -m codeforge.consumer &
```

### Step 0.8: Start Frontend

```bash
cd frontend && npm run dev &
```

### Step 0.9: Open Browser & Login

```
browser_navigate: http://host.docker.internal:3000
browser_snapshot  (verify login page loaded)
browser_fill_form: [email field] = "admin@localhost"
browser_fill_form: [password field] = "Changeme123"
browser_click: [Login button]
browser_snapshot  (verify dashboard loaded)
```

### Step 0.10: Health Check -- Verify All 6 Services

| Service | Check | Expected |
|---|---|---|
| PostgreSQL | `pg_isready -h ${POSTGRES_IP}` | exit 0 |
| NATS | `curl -s http://${NATS_IP}:8222/varz` | JSON response |
| LiteLLM | `curl -s http://${LITELLM_IP}:4000/health` | OK / healthy |
| Go Backend | `curl -s http://localhost:8080/health` | `dev_mode: true` |
| Python Worker | `curl -s http://${NATS_IP}:8222/jsz?consumers=1` | consumer_count >= 1 |
| Frontend | Browser loaded, no console errors | Dashboard visible |

### Validation

All 6 services running, browser logged in, health checks passed.

### Decision Tree

```
Docker fails to start?
  -> Check Docker daemon: `docker info`
  -> Check images: `docker compose pull`
  -> Retry: `docker compose up -d`
  -> Result: FAIL Phase 0 if Docker won't start

NATS consumer not found after Go backend started?
  -> Wait 10s (consumers auto-recreate)
  -> Restart Go backend
  -> Verify: `curl http://${NATS_IP}:8222/jsz?consumers=1`

Login fails in browser?
  -> Check backend health: `curl http://localhost:8080/health`
  -> Verify `dev_mode: true` in response (endpoints return 403 without it)
  -> Check credentials: admin@localhost / Changeme123

LiteLLM health check fails?
  -> Check Docker logs: `docker logs codeforge-litellm --tail 50`
  -> Verify config: `docker exec codeforge-litellm cat /app/config.yaml`
```

### State Variables Set

```
MODE = "A" or "D"
ENV = { nats_ip, litellm_ip, postgres_ip }
```

---

## Phase 1: Project Setup

### Step 1.1: Navigate to Projects

```
browser_click: [Projects nav item / sidebar link]
browser_snapshot  (verify Projects page loaded)
```

### Step 1.2: Create New Project

```
browser_click: [New Project / Create Project button]
browser_snapshot  (verify creation modal/form appeared)
```

### Step 1.3: Fill Project Details

```
browser_fill_form: [Name field] = "multi-lang-showcase"  (Mode A)
                                   "multi-lang-freeform"  (Mode D)
browser_fill_form: [Autonomy Level] = "4"  (or select from dropdown)
browser_click: [Create / Submit button]
browser_snapshot  (verify project created, redirected to project page)
```

### Step 1.4: Capture PROJECT_ID

```
browser_evaluate: window.location.pathname.split('/').pop()
# Or extract from the URL shown in the browser: /projects/{uuid}
```

Set `PROJECT_ID` from result.

### Step 1.5: Patch Config via API

`policy_preset` and `execution_mode` are NOT available in the UI. Patch via API:

```
browser_evaluate:
  fetch('/api/v1/projects/' + PROJECT_ID, {
    method: 'PATCH',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': 'Bearer ' + localStorage.getItem('access_token')
    },
    body: JSON.stringify({
      config: {
        policy_preset: 'trusted-mount-autonomous',
        execution_mode: 'mount'
      }
    })
  }).then(r => r.json())
```

Verify response shows updated config.

### Step 1.6: Create & Adopt Workspace

```bash
TIMESTAMP=$(date +%s)
mkdir -p /tmp/codeforge-multi-lang-${TIMESTAMP}
cd /tmp/codeforge-multi-lang-${TIMESTAMP} && git init -b main
```

Adopt via API:

```
browser_evaluate:
  fetch('/api/v1/projects/' + PROJECT_ID + '/adopt', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': 'Bearer ' + localStorage.getItem('access_token')
    },
    body: JSON.stringify({ path: '/tmp/codeforge-multi-lang-TIMESTAMP' })
  }).then(r => r.json())
```

Set `WORKSPACE = "/tmp/codeforge-multi-lang-{TIMESTAMP}"`.

### Step 1.7: Open Chat & Set Model

```
browser_click: [Chat tab/panel in project view]
browser_snapshot  (verify chat panel opened)
```

Type `/model` command to set the LLM:

```
browser_fill_form: [chat input] = "/model lm_studio/qwen/qwen3-30b-a3b"
                                   (or "/model anthropic/claude-sonnet-4-20250514" for cloud)
browser_press_key: Enter
browser_snapshot  (verify model set confirmation)
```

### Step 1.8: Capture CONVERSATION_ID

```
browser_evaluate: window.location.pathname.split('/').pop()
# Or extract conversation ID from the chat panel URL
```

Set `CONVERSATION_ID` and `MODEL` from results.

### Validation

- Project visible in Dashboard
- Config: autonomy_level=4, policy_preset=trusted-mount-autonomous, execution_mode=mount
- Workspace adopted
- Model set and confirmed
- CONVERSATION_ID captured

### Decision Tree

```
Project creation fails?
  -> browser_snapshot to see error message
  -> Check backend logs for constraint violations (duplicate name, etc.)
  -> Retry with different name: "multi-lang-showcase-2"

Adopt fails?
  -> Verify workspace directory exists: `ls -la /tmp/codeforge-multi-lang-*`
  -> Check path is absolute (must start with /)
  -> Check project doesn't already have a workspace

/model command not recognized?
  -> Verify chat panel is open and input field is focused
  -> Try clicking the chat input first, then type and press Enter
  -> Check if model exists: `curl http://${LITELLM_IP}:4000/v1/models`
```

### State Variables Set

```
PROJECT_ID = "<uuid>"
WORKSPACE = "/tmp/codeforge-multi-lang-<timestamp>"
CONVERSATION_ID = "<uuid>"
MODEL = "<model name>"
```

---

## Phase 2: Goal Conversation

### Step 2.1: Switch to Goal Researcher Mode

```
browser_fill_form: [chat input] = "/mode goal_researcher"
browser_press_key: Enter
browser_snapshot  (verify mode switch confirmation message)
```

### Step 2.2: Send Initial Message (Conversational Strategy)

**Important:** Use multiple short messages instead of one large prompt. Local models handle smaller inputs faster and produce better goal proposals. Build the project description naturally through dialog.

**Mode A (Showcase -- Weather Dashboard) -- Conversational Flow:**

**Message 1** (introduction):
```
"Hi! I want to build a weather dashboard. It should have a Python backend and a TypeScript frontend that talk to each other via REST API."
```
Wait for agent response (may ask questions or propose initial goals).

**Message 2** (backend details, after agent responds):
```
"For the backend, use Python with FastAPI. It should fetch weather data from wttr.in (no API key needed), cache responses, and serve 3 endpoints: current weather, forecast, and city search."
```
Wait for agent response.

**Message 3** (frontend details, after agent responds):
```
"For the frontend, use TypeScript with SolidJS (NOT React). Use createSignal and createEffect, not useState/useEffect. Show current weather, a temperature chart, and a city search. Use Vite as bundler."
```
Wait for agent response.

**Message 4** (requirements, after agent responds):
```
"Use separate directories: backend/ and frontend/. Write tests for both (pytest for Python, vitest for TypeScript). Everything must run locally. Commit all work to git when done."
```
Wait for agent to propose remaining goals.

**Fallback: Single-Message Approach** (if conversational flow is too slow or agent doesn't respond well):

```
browser_fill_form: [chat input] =
  "I need a Real-Time Weather Dashboard application. It must use two programming
   languages that communicate with each other:
   1. Python FastAPI Backend -- fetches weather data from wttr.in (no API key),
      caches responses, serves REST API (current weather, forecast, city search)
   2. TypeScript/SolidJS Frontend -- interactive dashboard with charts and city
      search. Use SolidJS primitives (createSignal, createEffect, NOT React hooks).
   Separate directories: backend/ and frontend/. Tests: pytest + vitest.
   Runnable locally. Git commit when done."

browser_press_key: Enter
```

**Mode D (Free Choice):**

```
browser_fill_form: [chat input] =
  "I need you to design and build a useful software project that uses at least
   two different programming languages. The languages must actively communicate
   with each other (via API, message queue, IPC, or similar). Propose a project
   idea, explain your choice of languages, and describe the architecture before
   we start."

browser_press_key: Enter
```

### Step 2.3: Conversation Loop

Repeat until goals are validated:

1. **Wait for agent response** (check every 15 seconds):
   ```
   browser_snapshot  (check for new messages in chat)
   ```

2. **If agent asks clarifying questions:**
   ```
   browser_fill_form: [chat input] = "<answer to question>"
   browser_press_key: Enter
   ```
   Note: At autonomy level 4, the agent may skip questions and immediately propose goals. This is valid.

3. **If GoalProposalCard appears in chat stream:**
   ```
   browser_snapshot  (identify GoalProposalCard with Approve/Reject buttons)
   browser_click: [Approve button on GoalProposalCard]
   ```
   Click "Reject" only if the goal is clearly wrong or irrelevant. If rejecting, give feedback:
   ```
   browser_fill_form: [chat input] = "That goal doesn't match. I need <correction>."
   browser_press_key: Enter
   ```

4. **After proposals stop arriving, check GoalsPanel:**
   ```
   browser_snapshot  (check GoalsPanel for approved goals)
   ```
   Verify:
   - [ ] At least 5 goals visible
   - [ ] Both languages explicitly mentioned in goals
   - [ ] Cross-language integration goal present
   - [ ] Goals cover: project setup, backend, frontend, tests, integration

5. **If goals incomplete -- give correction hints:**
   ```
   browser_fill_form: [chat input] = "I notice the goals don't mention testing.
     Can you add a goal for test coverage for both the Python backend and
     TypeScript frontend?"
   browser_press_key: Enter
   ```
   Wait for new GoalProposalCard, approve/reject, re-check GoalsPanel.
   **Each correction hint = INFO bug. Document what was missing and what hint was given.**

6. **If goals complete and correct:** proceed to Step 2.4.

### Step 2.4: Switch Back to Coder Mode

```
browser_fill_form: [chat input] = "/mode coder"
browser_press_key: Enter
browser_snapshot  (verify mode switch confirmation)
```

### Validation

- Minimum 5 goals in GoalsPanel (approved via GoalProposalCard clicks)
- Both languages explicitly mentioned in goals
- Cross-language integration goal present
- Claude Code confirms: "Goals look complete and correct"

### Bug Documentation

| Trigger | Severity | Document |
|---|---|---|
| Each correction hint given to agent | INFO | What was missing, what hint was given |
| Goals still wrong after 3 correction rounds | WARNING | Escalate phase assessment |
| Claude Code has to manually dictate all goals | CRITICAL | Agent failed to understand requirements |

### Decision Tree

```
Agent doesn't respond after 60s?
  -> browser_snapshot to check for error messages
  -> Check NATS consumers via API: `curl http://${NATS_IP}:8222/jsz?consumers=1`
  -> If stalled: type "Are you there? Please propose goals for this project." in chat
  -> If still no response after 120s: FAIL Phase 2, document CRITICAL bug

No GoalProposalCard appears (agent responds with text only)?
  -> Agent may not be in goal_researcher mode
  -> browser_snapshot to check mode indicator in UI
  -> If wrong mode: type `/mode goal_researcher` again and resend initial message
  -> BUG (WARNING): mode switch didn't take effect

GoalsPanel shows 0 goals after approving cards?
  -> Possible: Playwright click missed the Approve button
  -> browser_snapshot, find Approve button coordinates, browser_click again
  -> If still 0: BUG (CRITICAL) -- goal approval mechanism broken
```

### State Variables Set

```
GOAL_COUNT = <number of approved goals>
GOALS_VALIDATED = true
```

---

## Phase 3: Roadmap Creation

**Important:** The CodeForge agent does NOT have a tool to create roadmap entities (milestones, features). Claude Code creating the roadmap via UI is **expected behavior**, not a bug. The bug trigger is if the **agent cannot describe** the roadmap structure.

### Step 3.1: Ask Agent to Describe Roadmap

```
browser_fill_form: [chat input] =
  "Based on the goals we've established, describe a roadmap with milestones
   and features. Group the work into logical phases (e.g. Backend, Frontend,
   Integration, Testing). List each milestone with its features."

browser_press_key: Enter
```

### Step 3.2: Wait for Agent Response

```
browser_snapshot  (check every 15s for agent's roadmap description)
```

Parse the agent's description. It should contain:
- At least 2 milestones (e.g. "Backend", "Frontend", "Integration")
- Features listed under each milestone
- Both languages represented

### Step 3.3: Create Roadmap via UI

Navigate to RoadmapPanel:
```
browser_click: [Roadmap tab/panel in project view]
browser_snapshot  (verify RoadmapPanel loaded)
```

Create roadmap:
```
browser_click: [Create Roadmap / New Roadmap button]
browser_fill_form: [Title] = "Multi-Language Weather Dashboard"  (Mode A)
                               "<agent's proposed project name>"  (Mode D)
browser_click: [Create / Submit]
browser_snapshot
```

For each milestone the agent described:
```
browser_click: [Add Milestone button]
browser_fill_form: [Milestone title] = "<milestone name from agent>"
browser_click: [Save / Create]
```

For each feature:
```
browser_click: [Add Feature button under milestone]
browser_fill_form: [Feature title] = "<feature name from agent>"
browser_click: [Save / Create]
```

### Step 3.4: Verify Roadmap

```
browser_snapshot  (verify roadmap with milestones and features visible)
```

Check:
- [ ] At least 2 milestones
- [ ] Features assigned to milestones
- [ ] Features cover all goals from Phase 2
- [ ] Both languages represented in the structure

### Fallback Escalation (BUG Triggers)

| Trigger | Action | Severity |
|---|---|---|
| 30s no response to roadmap request | Repeat the request in chat | INFO |
| Agent's description is incomplete (missing a language/phase) | Ask for clarification in chat: "You didn't mention testing. Can you add a Testing milestone?" | WARNING |
| Agent's description is completely wrong or incoherent | Describe the desired structure: "I need: Milestone 1: Backend (Python FastAPI), Milestone 2: Frontend (TypeScript), Milestone 3: Integration & Testing. Can you confirm?" | WARNING |
| Agent cannot describe a roadmap at all (stall, hallucination, refusal) | Claude Code designs the roadmap independently and creates it via UI | CRITICAL |

### Decision Tree

```
Agent describes roadmap but as flat list (no milestones)?
  -> Ask: "Can you group these into milestones? E.g. Milestone 1: Backend, Milestone 2: Frontend"
  -> Document as WARNING bug

RoadmapPanel not found in UI?
  -> browser_snapshot to see available tabs/panels
  -> Look for "Roadmap", "Milestones", or "Features" in the navigation
  -> If no Roadmap UI exists: create via API as fallback:
     POST /api/v1/projects/{PROJECT_ID}/roadmap

Create Milestone button missing?
  -> Roadmap entity may need to be created first (parent)
  -> browser_snapshot to identify the correct UI flow
```

### State Variables Set

```
ROADMAP_OK = true
MILESTONES = <count>
FEATURES = <count>
```

---

## Phase 4: Autonomous Execution

### Step 4.1: (Optional) Bypass Approvals

To prevent HITL interruptions during autonomous execution:

```
browser_evaluate:
  fetch('/api/v1/conversations/' + CONVERSATION_ID + '/bypass-approvals', {
    method: 'POST',
    headers: { 'Authorization': 'Bearer ' + localStorage.getItem('access_token') }
  }).then(r => r.json())
```

Set `BYPASS_APPROVALS_USED = true` if used. Document in the report.

### Step 4.2: Send Execution Command

```
browser_fill_form: [chat input] =
  "The roadmap is set up. Please start implementing it now. Begin with the
   first milestone. Create all files in the workspace, install dependencies,
   write tests, and commit your work."

browser_press_key: Enter
```

Record `EXEC_START_TIME = now()`.

### Step 4.3: Monitoring Loop (Every 30 Seconds)

**Primary monitoring: API** (more reliable than browser polling)

```bash
curl -s -H "Authorization: Bearer ${TOKEN}" \
  "http://localhost:8080/api/v1/conversations/${CONVERSATION_ID}/messages?limit=5" \
  | jq '.[-1]'
```

Check:
- New messages since last check? -> Agent is working, continue
- Last message contains `tool_call`? -> Agent is using tools, continue
- Last message is from agent with no tool calls for >5 min? -> Possible stall

**Secondary monitoring: Browser** (for HITL and visual checks)

```
browser_snapshot  (check for HITL approval cards, error states)
```

If Playwright-MCP disconnects:
- Continue with API-only monitoring
- Reconnect browser when needed:
  ```
  browser_navigate: http://host.docker.internal:3000
  ```
- Re-login if session expired

### Step 4.4: HITL Handling

If an approval request appears:

**Via UI:**
```
browser_snapshot  (identify PermissionRequestCard)
browser_click: [Approve / Allow button]
```

**Via API (if browser disconnected):**
```bash
# Find pending approval
curl -s -H "Authorization: Bearer ${TOKEN}" \
  "http://localhost:8080/api/v1/conversations/${CONVERSATION_ID}/messages?limit=10" \
  | jq '.[] | select(.type == "permission_request")'

# Approve it
curl -s -X POST -H "Authorization: Bearer ${TOKEN}" \
  -H "Content-Type: application/json" \
  "http://localhost:8080/api/v1/runs/${CONVERSATION_ID}/approve/${CALL_ID}" \
  -d '{"decision": "allow"}'
```

**BUG:** If HITL approval is requested AND `BYPASS_APPROVALS_USED = true`, document as WARNING bug.

### Step 4.5: Stall Detection

**No new tool calls for 10 minutes = CRITICAL bug.**

If stall detected:
1. Document as CRITICAL bug (what was the last action, what seems to be blocking)
2. Try giving a hint in chat:
   ```
   browser_fill_form: [chat input] = "What's blocking you? Continue with the implementation."
   browser_press_key: Enter
   ```
3. Wait another 5 minutes
4. If still stalled: set `EXEC_STATUS = "stalled"`, proceed to Phase 5

### Step 4.6: Timeout

- **Mode A:** Maximum 120 minutes
- **Mode D:** Maximum 180 minutes

If timeout reached:
1. Set `EXEC_STATUS = "timeout"`
2. Document remaining work
3. Proceed to Phase 5 and verify whatever exists

### Step 4.7: Completion

When agent reports completion (or stall/timeout):
1. Record `EXEC_DURATION = (now - EXEC_START_TIME) in minutes`
2. Set `EXEC_STATUS = "completed"` (or "stalled" / "timeout" / "error")
3. Proceed to Phase 5

### Decision Tree

```
Agent stalls (no new tool calls for 10 minutes)?
  -> Check last message via API -- is it waiting for HITL approval?
  -> If HITL pending: approve it (see Step 4.4)
  -> If genuinely stuck: send hint in chat
  -> Document as CRITICAL bug

Playwright-MCP disconnects?
  -> Switch to API-only monitoring (curl commands above)
  -> Reconnect browser: browser_navigate http://host.docker.internal:3000
  -> Re-login if needed (admin@localhost / Changeme123)

Agent writes files to wrong directory?
  -> Check workspace_path in project config via API
  -> Known issue from previous testing (Run 5) -- workspace_path may be empty
  -> BUG (CRITICAL): workspace adoption didn't persist

Agent runs but produces no files?
  -> Check workspace: `ls -la ${WORKSPACE}`
  -> Check if agent is writing to a different path
  -> Check NATS for tool call messages
```

### State Variables Set

```
EXEC_STARTED = true
EXEC_DURATION = <minutes>
EXEC_STATUS = "completed" | "stalled" | "timeout" | "error"
BYPASS_APPROVALS_USED = true | false
```

---

## Phase 5: Metrics Collection

Collected during Phase 4, summarized here. Run these commands after execution completes.

### Step 5.1: Count Tool Calls

```bash
# Via API: get all messages and count tool calls
curl -s -H "Authorization: Bearer ${TOKEN}" \
  "http://localhost:8080/api/v1/conversations/${CONVERSATION_ID}/messages?limit=500" \
  | jq '[.[] | select(.tool_calls != null) | .tool_calls[]] | group_by(.name) | map({(.[0].name): length}) | add'
```

### Step 5.2: Count Files and LOC

```bash
cd ${WORKSPACE}
echo "=== Files ==="
find . -type f -not -path './.git/*' -not -path './node_modules/*' -not -path './.venv/*' | wc -l
echo "=== LOC ==="
find . -type f \( -name "*.py" -o -name "*.ts" -o -name "*.tsx" -o -name "*.js" \) \
  -not -path './node_modules/*' -not -path './.venv/*' | xargs wc -l 2>/dev/null | tail -1
echo "=== Git Commits ==="
git log --oneline 2>/dev/null | wc -l
```

### Step 5.3: Collect Cost

```
browser_snapshot  (check for cost badge in chat messages)
```

Or via API: check `state_delta` events for cost information.

### Step 5.4: Compile Metrics

```
TOOL_CALLS = { write: N, read: N, edit: N, bash: N, search: N, glob: N, listdir: N }
METRICS = {
  duration: <EXEC_DURATION>m,
  cost: $<amount>,
  files: <count>,
  loc: <count>,
  commits: <count>,
  self_corrections: <edit_file calls on previously write_file'd files>,
  errors: <count of error messages in chat>
}
```

---

## Phase 6: Functional Verification

### Step 6.0: Workspace Discovery (Both Modes)

Before running any checks, inspect the workspace:

```bash
cd ${WORKSPACE}

echo "=== Directory Structure ==="
find . -type f -not -path './.git/*' -not -path './node_modules/*' -not -path './.venv/*' | head -50

echo "=== Python Config ==="
cat pyproject.toml 2>/dev/null || cat requirements.txt 2>/dev/null || echo "NO_PYTHON_CONFIG"

echo "=== Node Config ==="
cat package.json 2>/dev/null || echo "NO_NODE_CONFIG"

echo "=== Go Config ==="
cat go.mod 2>/dev/null || echo "NO_GO_CONFIG"

echo "=== Entry Points ==="
head -5 $(find . -name "main.py" -o -name "app.py" -o -name "server.py" -o -name "index.ts" -o -name "main.ts" 2>/dev/null | head -5) 2>/dev/null

echo "=== Test Files ==="
find . -name "test_*" -o -name "*_test.*" -o -name "*.test.*" -o -name "*.spec.*" 2>/dev/null | head -20
```

Use discovery results to determine exact commands for subsequent checks.

### Mode A: Weather Dashboard -- Specific Checks

| # | Check | Command / Method | PASS when |
|---|---|---|---|
| 6.1 | Backend files exist | `find ${WORKSPACE} -name "*.py" \| head -10` | Python files + requirements.txt/pyproject.toml present |
| 6.2 | Frontend files exist | `find ${WORKSPACE} -name "*.ts" -o -name "*.tsx" \| head -10` | TS/JS files + package.json present |
| 6.3 | Backend deps install | `cd <backend_dir> && pip install -r requirements.txt` (or `poetry install`) | Exit 0 |
| 6.4 | Frontend deps install | `cd <frontend_dir> && npm install` | Exit 0 |
| 6.5 | Backend starts | Run discovered entry point (e.g. `cd <backend_dir> && uvicorn main:app --port 8001 &`) | Process runs, no crash within 5s |
| 6.6 | Backend API responds | `curl -s http://localhost:8001/` (discover port from code) | HTTP 200, JSON response |
| 6.7 | Frontend builds | `cd <frontend_dir> && npm run build` | Exit 0, no errors |
| 6.8 | Backend tests exist | `find ${WORKSPACE} -name "test_*.py" -o -name "*_test.py"` | At least 1 test file |
| 6.9 | Frontend tests exist | `find ${WORKSPACE} -name "*.test.*" -o -name "*.spec.*"` | At least 1 test file |
| 6.10 | Backend tests pass | `cd <backend_dir> && pytest -v` | All tests PASS |
| 6.11 | Frontend tests pass | `cd <frontend_dir> && npm test` (or `npx vitest run`) | All tests PASS |

### Mode D: Free Choice -- Dynamic Checks

Claude Code uses workspace discovery to determine:
1. Which languages are present (file extensions, config files)
2. What the entry points are (main files, scripts in package managers)
3. What test frameworks are used
4. What build/start commands exist

Then run equivalent checks:
- [ ] Dependencies install for **each** language (exit 0)
- [ ] **Each** component starts without crash (process runs 5s)
- [ ] Tests exist for **each** language (at least 1 test file)
- [ ] Tests pass for **each** language (all PASS)
- [ ] At least one integration point exists between languages (shared API, config, etc.)

### Result per check: PASS / FAIL / SKIP (with reason)

### Decision Tree

```
pip install fails?
  -> Check Python version: `python3 --version` (need 3.10+)
  -> Try with venv: `python3 -m venv .venv && source .venv/bin/activate && pip install -r requirements.txt`
  -> Check requirements format: `cat requirements.txt` (invalid entries?)

npm install fails?
  -> Check Node version: `node --version` (need 18+)
  -> Check package.json validity: `node -e "require('./package.json')"`
  -> Try: `npm install --legacy-peer-deps`

Backend won't start?
  -> Read the entry point file to find the correct start command
  -> Check for missing env vars (API keys, database URLs, port configs)
  -> Check port conflicts: `lsof -i :8001`

Tests fail?
  -> Read test output to identify which tests fail and why
  -> Document as PARTIAL (not FAIL) if some tests pass
  -> Count: X out of Y tests pass
```

---

## Phase 7: Code Quality Verification

### Step 7.0: Determine Applicable Checks

Based on workspace discovery from Phase 6, select which quality checks apply.

### Python Quality Checks

| # | Check | Command | PASS when |
|---|---|---|---|
| 7.1 | Python lint | `cd <backend_dir> && ruff check .` | 0 errors (warnings OK) |
| 7.2 | Python type check | `cd <backend_dir> && mypy .` (if mypy in deps) or `pyright` | No critical errors |

### TypeScript Quality Checks

| # | Check | Command | PASS when |
|---|---|---|---|
| 7.3 | TS type check | `cd <frontend_dir> && npx tsc --noEmit` | Exit 0 |
| 7.4 | TS lint | `cd <frontend_dir> && npx eslint .` (if eslint configured) | 0 errors |
| 7.5 | No `any` hacks | `grep -rnP ": any\b\|as any\b\|<any>" --include="*.ts" --include="*.tsx" <frontend_dir>` | None or justified |

### General Quality Checks

| # | Check | Command | PASS when |
|---|---|---|---|
| 7.6 | No TODO/FIXME/HACK | `grep -rn "TODO\|FIXME\|HACK" --include="*.py" --include="*.ts" --include="*.tsx" --include="*.js" ${WORKSPACE}` | None or justified |
| 7.7 | Dependencies resolved | Phase 6 install checks passed | No unresolved deps |
| 7.8 | Project structure | `find ${WORKSPACE} -type f -not -path './.git/*' -not -path './node_modules/*' \| head -30` | Sensible directory layout (not all files in root) |

### Mode D Adaptation

For other languages, use equivalent tools:
- **Go:** `go vet ./...`, `golangci-lint run`
- **Rust:** `cargo clippy`
- **Java:** `mvn checkstyle:check` or `gradle check`

### Result per check: PASS / FAIL / SKIP (with reason)

---

## Phase 8: Semantic Verification (LLM-as-Judge)

Claude Code reads the generated source code and evaluates whether it fulfills the goals.

### Mode A: Weather Dashboard

| # | Check | Method | PASS when |
|---|---|---|---|
| 8.1 | Backend fulfills goals | Read all Python source files in workspace | Code fetches weather data (API call present), has caching (cache logic/decorator), serves REST API (FastAPI/Flask routes) |
| 8.2 | Frontend fulfills goals | Read all TypeScript source files | Code displays weather data (component renders data), has charts (chart library import or SVG), has city search/filter (input + filter logic) |
| 8.3 | Architecture is sensible | Read all source files | Clean separation (backend/ and frontend/ dirs), no god-files (>500 LOC), proper module structure |
| 8.4 | Code is non-trivial | `find . -name "*.py" -o -name "*.ts" \| xargs wc -l` | Total LOC >= 200, real implementation (not just stubs or boilerplate) |

### Mode D: Free Choice

| # | Check | Method | PASS when |
|---|---|---|---|
| 8.1 | Result matches proposal | Compare code against goals from Phase 2 | Agent built what it proposed in the goal conversation |
| 8.2 | Both languages meaningful | Read source for each language | Neither language is trivial/stub -- each has real logic (>=50 LOC of non-boilerplate) |
| 8.3 | Architecture is sensible | Read all source files | Clean separation, proper module structure, no god-files |
| 8.4 | Code is non-trivial | Count LOC, analyze logic | Total LOC >= 200, real implementation |

### Evaluation Method

For each check, Claude Code:
1. Reads the relevant source files
2. Evaluates against the criteria
3. Records PASS/FAIL with a brief justification (1-2 sentences)

This phase is qualitative -- Claude Code uses its own judgment as an experienced developer.

---

## Phase 9: Cross-Language Integration Verification

The hardest test -- do the languages actually work together?

### Check 9.1: API Contract Match

```bash
# Find backend routes
grep -rn "app.get\|app.post\|@app.route\|@router\|@app.api_route" --include="*.py" ${WORKSPACE}

# Find frontend fetch calls
grep -rn "fetch(\|axios\.\|http\.\|createResource\|createSignal.*fetch" --include="*.ts" --include="*.tsx" ${WORKSPACE}
```

**PASS when:** Frontend fetch URLs match backend route definitions (same paths, same HTTP methods).

### Check 9.2: Data Format Match

```bash
# Find backend response models/types
grep -rn "class.*BaseModel\|class.*Model\|TypedDict\|return {" --include="*.py" ${WORKSPACE}

# Find frontend interfaces/types
grep -rn "interface \|type .*=" --include="*.ts" --include="*.tsx" ${WORKSPACE}
```

Claude Code reads both and compares: **PASS when** field names and types align between backend responses and frontend type definitions.

### Check 9.3: Live Roundtrip (Stretch Goal)

**Prerequisites:** Backend started successfully (Phase 6, check 6.5) AND frontend builds (Phase 6, check 6.7). **SKIP if either failed.**

```bash
# Start backend in background (use discovered entry point from Phase 6)
cd <backend_dir> && uvicorn main:app --port 8001 &
BACKEND_PID=$!

# Wait for backend to be ready
timeout 15 bash -c 'until curl -s http://localhost:8001 > /dev/null 2>&1; do sleep 1; done'

# Start frontend in preview mode
cd <frontend_dir> && npm run build && npx serve dist -l 3001 &
FRONTEND_PID=$!

# Wait for frontend to be ready
timeout 15 bash -c 'until curl -s http://localhost:3001 > /dev/null 2>&1; do sleep 1; done'
```

Open in Playwright:
```
browser_navigate: http://localhost:3001
browser_snapshot  (verify frontend loads)
browser_snapshot  (verify data from backend appears -- weather data, charts, etc.)
```

**PASS when:** Data from backend is visibly rendered in the frontend.

Cleanup:
```bash
kill $BACKEND_PID $FRONTEND_PID 2>/dev/null
```

### Check 9.4: Error Handling

**SKIP if 9.3 was skipped.**

```bash
# Start backend, then kill it
cd <backend_dir> && uvicorn main:app --port 8001 &
BACKEND_PID=$!
sleep 3

cd <frontend_dir> && npx serve dist -l 3001 &
FRONTEND_PID=$!
sleep 3

# Kill backend
kill $BACKEND_PID

# Check frontend
```

```
browser_navigate: http://localhost:3001
browser_snapshot  (verify frontend shows error message or loading state, not a crash)
```

**PASS when:** Frontend displays a user-friendly error message or loading state. Does NOT crash or show a white screen.

Cleanup:
```bash
kill $FRONTEND_PID 2>/dev/null
```

### Check 9.5: Shared Types/Schema (Bonus)

```bash
# Search for OpenAPI spec
find ${WORKSPACE} -name "openapi*" -o -name "swagger*" -o -name "api-spec*" 2>/dev/null

# Search for shared type definitions
find ${WORKSPACE} -name "types.ts" -o -name "schema.*" -o -name "api.d.ts" 2>/dev/null
```

**PASS when:** Any shared contract exists (OpenAPI spec, shared types file, JSON schema). **SKIP** if no shared contract found (this is a bonus check).

### Mode D Adaptation

For non-REST communication (WebSocket, gRPC, message queue):
- **9.1:** Match message types / protobuf definitions instead of REST routes
- **9.2:** Match serialization formats on both sides
- **9.3:** Verify bidirectional communication works live
- **9.4:** Verify graceful degradation when one component is down

### Result per check: PASS / FAIL / SKIP (with reason)

---

## Phase 10: Report Generation

### Step 10.1: Compile Results

Gather all state variables, check results, and bug list from Phases 0-9.

### Step 10.2: Generate Report

Save to `docs/testing/YYYY-MM-DD-multi-language-autonomous-report.md` with this template:

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
| 5 - Metrics | N/A | collected |
| 6 - Functional Verification | PASS/PARTIAL/FAIL | X/Y checks passed |
| 7 - Code Quality | PASS/PARTIAL/FAIL | X/Y checks passed |
| 8 - Semantic Verification | PASS/PARTIAL/FAIL | X/Y checks passed |
| 9 - Cross-Language Integration | PASS/PARTIAL/FAIL | X/Y checks passed |

## Metrics

| Metric | Value |
|--------|-------|
| Model | <model name> |
| Total tool calls | N |
| Tool call breakdown | write: N, read: N, edit: N, bash: N, search: N, glob: N, listdir: N |
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

<Key exchanges from Phase 2: how many rounds, what corrections were needed,
what the agent proposed vs. what was accepted>

## Conclusion

<Overall assessment:
- Is this showcase-worthy? (Mode A threshold: no CRITICAL bugs, >= 75% overall checks)
- What worked well?
- What needs fixing? (reference bug list)
- Recommendation for next run (different model? different prompt? fix bugs first?)>
```

---

## Success Criteria

### Showcase-Worthy (Mode A)

| Category | Threshold |
|---|---|
| Phase 6 (Functional) | >= 9/11 checks PASS |
| Phase 7 (Quality) | >= 6/8 checks PASS |
| Phase 8 (Semantic) | >= 3/4 checks PASS |
| Phase 9 (Cross-Language) | >= 3/5 checks PASS (including 9.3 live roundtrip) |
| Bug list | No CRITICAL bugs, max 3 WARNING bugs |

### Evaluation-Worthy (Mode D)

| Category | Threshold |
|---|---|
| Phase 6 (Functional) | >= 70% checks PASS |
| Phase 9 (Cross-Language) | >= 2/5 checks PASS (including 9.1 API contract match) |
| Language usage | Agent chose and used 2+ languages meaningfully |
| Bug list | Documented for LLM comparison |

---

## Mode Comparison Quick Reference

| Aspect | A (Showcase) | D (Free Choice) |
|---|---|---|
| Project | Weather Dashboard | Agent decides |
| Languages | Python FastAPI + TS/SolidJS | Agent decides (min. 2) |
| Goal prompt | Detailed, specific | Open-ended |
| Expected files | Known checklist | Dynamically discovered |
| Verification: functional | Specific checks (API, build) | Generic (starts? builds? tests?) |
| Verification: semantic | "Weather data? Charts? Filters?" | "Matches agent's proposal?" |
| Verification: cross-language | "Frontend calls FastAPI?" | "Languages communicate?" |
| Timeout | 120 min | 180 min |
| Difficulty | Medium (clear spec) | Hard (open-ended) |
| Best for | Showcase, regression | LLM comparison, capability eval |

---

## Known Limitations & Risks

| Risk | Mitigation |
|---|---|
| Playwright-MCP disconnects during long waits | Monitor via API as primary, reconnect browser for UI interactions |
| Local models (LM Studio) very slow | Extended timeouts (120-180 min), do not abort early |
| NATS message backlog from previous runs | Purge in Phase 0 (mandatory) |
| CodeForge agent may not understand multi-language requirement | Explicit in goal prompt, validate in GoalsPanel before proceeding |
| Frontend build may need specific Node version | Check `package.json` engines field |
| Weather API may require API key | Use keyless alternatives (wttr.in) or mock data |
| Agent has no roadmap creation tool | Expected: agent describes, Claude Code creates via UI |
| Goal proposals need manual approval clicks | Claude Code clicks Approve on each GoalProposalCard |
| policy_preset/execution_mode not in UI | API PATCH after project creation |
| Local models overwhelmed by long prompts | **Conversational strategy**: send multiple short messages instead of one wall-of-text. Build project description naturally through dialog (Run 1 learning) |
| Local models confuse React and SolidJS | Explicit negative instruction ("NOT React hooks") + framework detection in stack context (Bug #4+6 fix) |
| LiteLLM tag mismatch for local models | Ensure local model entries have all scenario tags: default, background, think, longContext, review, plan (Run 1 Bug #1 fix) |

---

## Learnings from Previous Runs

### Run 1 (2026-03-22, qwen3-30b, single-prompt)
- **57% score (16/28)** -- not showcase-worthy
- Agent confused React/SolidJS (Bug #4+6, fixed)
- goal_researcher skipped proposals (Bug #2, fixed)
- test_main.py had `</n` syntax error (Bug #5, mitigated with post-write lint)
- **Key learning:** Single large prompt overwhelms local models. Conversational approach recommended.

### Run 2 (2026-03-22, qwen3-30b, single-prompt with explicit SolidJS)
- Bug #2 fix confirmed: agent called `propose_goal` successfully
- Still slow with large prompt -- switched to conversational strategy mid-run

---

## Relationship to Existing Testplans

This testplan extends the existing `autonomous-goal-to-program-testplan.md` (S1-S4 scenarios) with:

- **Multi-language requirement** (S1-S4 are single-language)
- **Goal conversation** (S1-S4 use API to set goals directly)
- **4-tier verification** (S1-S4 only do functional checks)
- **Cross-language integration testing** (entirely new)
- **Bug documentation protocol** (every agent assist = documented bug)
- **Interactive mode selection** (A vs D, Claude Code asks user)
- **Mode switching** (`/mode goal_researcher` then `/mode coder`)
- **Explicit model selection** (`/model` command)
- **GoalProposalCard approval** (click Approve/Reject in UI)
- **Roadmap via UI** (agent describes, Claude Code creates)

The existing S1-S4 testplan remains valid for single-language scenarios.
