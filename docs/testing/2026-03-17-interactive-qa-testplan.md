# Interactive AI QA Test Plan: Claude Code + Playwright-MCP Tests CodeForge Chat

**Date:** 2026-03-17
**Type:** QA Test Runbook
**Tool:** Claude Code with `playwright-mcp` tools
**Target:** CodeForge Chat System — all implemented features

---

## Overview

This document is a structured test plan that Claude Code follows using `playwright-mcp` to interactively test all CodeForge chat features through the browser. It covers 16 phases from environment discovery through full-feature validation.

The plan uses **Decision Trees** for resilience — when a step fails, Claude Code follows fallback paths instead of aborting.

### Prerequisites

- CodeForge full stack running in development mode:
  ```bash
  # 1. Docker services
  docker compose up -d postgres nats litellm

  # 2. Go backend (APP_ENV=development required)
  APP_ENV=development go run ./cmd/codeforge/

  # 3. Frontend dev server
  cd frontend && npm run dev

  # 4. Python worker
  HF_TOKEN=... NATS_URL=nats://<container-ip>:4222 APP_ENV=development \
    .venv/bin/python -m codeforge.consumer
  ```
- At least one LLM model configured in LiteLLM
- Admin credentials: `admin@localhost` / `Changeme123`

---

## Architecture

```
FOUNDATION (sequential, gate for everything)
├─ Phase 0:  Environment Discovery & Readiness
├─ Phase 1:  Project Setup
├─ Phase 2:  Chat UI Navigation
└─ Phase 3:  Simple Message & Response

CORE CHAT (sequential, builds on each other)
├─ Phase 4:  Streaming Observation
├─ Phase 5:  Agentic Tool-Use
├─ Phase 6:  HITL Permissions
└─ Phase 7:  Full Project Creation

FEATURES (parallel, only need Phase 0-3)
├─ Phase 8:  Cost Tracking
├─ Phase 9:  Slash Commands
├─ Phase 10: Conversation Search
├─ Phase 11: Conversation Management
├─ Phase 12: Smart References
├─ Phase 13: Notifications
└─ Phase 14: Canvas Integration

WRAP-UP
└─ Phase 15: Report, Screenshots & Cleanup
```

### Dependency Rules

- **Phase 0-3:** Sequential, gate for all subsequent phases
- **Phase 4-7:** Sequential among themselves, require tool-capable model (skip 5-7 if unavailable)
- **Phase 8-14:** Parallel, only require Phase 0-3 passed
- **Phase 15:** Collects results from all phases

### Per-Phase Structure

Each phase follows this template:
1. **Goal** — what this phase validates
2. **Prerequisites** — which prior phases must pass
3. **Steps** — specific `playwright-mcp` tool calls
4. **Validation** — success criteria
5. **Decision Tree** — fallback paths when steps fail

---

## Phase 0: Environment Discovery & Readiness

**Goal:** Verify all services running, discover available models, classify capabilities.

**Prerequisites:** None (entry point).

### Steps

1. `browser_navigate` -> `http://localhost:3000` — frontend reachable?
2. `browser_snapshot` -> login page visible?
3. API check via `browser_evaluate`:
   ```javascript
   fetch('/api/v1/health').then(r => r.json())
   ```
   Expected: `{ status: "ok", dev_mode: true }`
4. Model discovery via `browser_evaluate`:
   ```javascript
   fetch('/api/v1/llm/discover').then(r => r.json())
   ```
   -> list of available models
5. Model classification: tool-capable? streaming? vision?
6. Store results mentally: `ENV = { models: [...], bestModel, toolCapable, visionCapable }`

### Validation

- Frontend renders login page
- Backend responds with `dev_mode: true`
- At least 1 model available

### Decision Tree

```
Frontend unreachable?
├─ Port 3000 timeout -> ABORT: "Frontend dev server not started (npm run dev)"
├─ Error page -> screenshot + ABORT: "Frontend error"
└─ Blank page -> browser_console_messages for JS errors

Backend /health failed?
├─ 502/503 -> ABORT: "Go backend not started"
├─ dev_mode: false -> WARN: "APP_ENV!=development, some features missing"
└─ Connection error -> ABORT: "Backend unreachable on port 8080"

No models found?
├─ LiteLLM reachable? -> fetch('http://localhost:4000/health')
│  ├─ No -> ABORT: "LiteLLM proxy not started"
│  └─ Yes but 0 models -> WARN: "No API keys configured"
├─ Worker status? -> check /health for worker info
│  └─ Worker offline -> ABORT: "Python worker not started"
└─ Models exist but none tool-capable ->
   FLAG: "Simple chat only, Phase 5-7 will be skipped"
```

---

## Phase 1: Project Setup

**Goal:** Log in and create a new project in CodeForge.

**Prerequisites:** Phase 0 passed.

### Steps

1. Login: `browser_fill_form` email=`admin@localhost`, password=`Changeme123` -> submit
2. Dashboard visible? `browser_snapshot` -> project list
3. "New Project" button click -> `browser_click`
4. Fill form: Name=`E2E-Test-Project`, Type=`local` -> `browser_fill_form`
5. Submit -> `browser_click`
6. Project appears in list? -> `browser_snapshot`

### Validation

- After login: dashboard with project list visible
- New project in list with correct name
- Project detail page opens

### Decision Tree

```
Login failed?
├─ Wrong password -> seeded admin password changed?
│  └─ API login attempt: POST /auth/login with admin@localhost/Changeme123
│     ├─ 200 -> password change required, do via API
│     └─ 401 -> ABORT: "Admin credentials invalid"
├─ Login page not visible -> cookie/session problem
│  └─ Clear cookies, reload
└─ 2FA/MFA active -> ABORT: "MFA not in test plan"

Project creation failed?
├─ Button not found -> UI selector changed?
│  └─ browser_snapshot + selector search via browser_evaluate
├─ Form validation error -> screenshot, read error message
├─ API 500 -> browser_console_messages
└─ Project already exists (409) ->
   Use existing project, don't create new
```

---

## Phase 2: Chat UI Navigation

**Goal:** Open chat panel in project and verify UI elements.

**Prerequisites:** Phase 1 passed.

### Steps

1. Open project -> `browser_click` on project in list
2. Find chat tab/panel -> `browser_snapshot`
3. Open chat panel -> `browser_click`
4. Verify UI elements: input field, send button, conversation list
5. `browser_snapshot` -> screenshot for report

### Validation

- Chat input visible and focusable
- Send button present (disabled when empty)
- Conversation list (empty or with default)

### Decision Tree

```
Chat panel not visible?
├─ Project has no workspace_path -> chat needs project with path
│  └─ Delete project, recreate with workspace_path
├─ Tab/navigation different -> browser_snapshot, search all links/buttons
└─ Feature flag disabled? -> /health check, dev_mode

Input field not interactive?
├─ Overlay/modal blocking -> browser_snapshot, close modal
├─ JavaScript error -> browser_console_messages
└─ WebSocket not connected ->
   Wait for reconnect (3s), then browser_snapshot again
```

---

## Phase 3: Simple Message & Response

**Goal:** Send a simple message and receive a response.

**Prerequisites:** Phase 2 passed.

### Steps

1. Click chat input -> `browser_click`
2. Type message: `Hello, what model are you?` -> `browser_type`
3. Click send button -> `browser_click`
4. Wait for response: `browser_wait_for` (assistant message appears)
5. `browser_snapshot` -> response visible?
6. Read and validate response content

### Validation

- User message appears in chat
- Typing/streaming indicator appears
- Assistant response appears with text
- No error state in UI

### Decision Tree

```
Message not sent?
├─ Send button disabled -> input empty? WebSocket disconnected?
│  └─ browser_evaluate: check WebSocket state
├─ Click registered but nothing happens ->
│  browser_console_messages for errors
└─ Try Enter key instead -> browser_press_key("Enter")

No response after 30s?
├─ Streaming indicator visible but no text?
│  ├─ Worker hanging -> console logs
│  └─ Model timeout -> try different model (Phase 0 fallback list)
├─ Error message in chat? -> screenshot + read error
│  ├─ "Model not found" -> model name wrong, redo discovery
│  ├─ "Rate limit" -> wait 10s, retry
│  └─ "Worker unavailable" -> ABORT: "Python worker down"
├─ No reaction at all -> WebSocket dead?
│  └─ Reload page, resend
└─ Response comes but is empty ->
   Model problem, next model from fallback list
```

---

## Phase 4: Streaming Observation

**Goal:** Verify AG-UI streaming events — text appears progressively, not as a block.

**Prerequisites:** Phase 3 passed.

### Steps

1. Send new message: `Explain in 3 paragraphs what a compiler does` -> `browser_type` + `browser_click`
2. Immediately `browser_snapshot` -> typing indicator visible?
3. Wait 2s -> `browser_snapshot` -> text growing?
4. Wait 5s -> `browser_snapshot` -> more text?
5. `browser_wait_for` (run finished) -> final state
6. `browser_console_messages` -> check AG-UI events (`agui.text_message`, `agui.run_started`, `agui.run_finished`)
7. Final `browser_snapshot` -> complete answer, no indicator

### Validation

- Streaming indicator appears during generation
- Text grows progressively (snapshot 2 has more text than snapshot 1)
- `agui.run_started` event before text
- `agui.run_finished` event at end
- Indicator disappears after completion

### Decision Tree

```
No streaming, text appears as block?
├─ Model doesn't support streaming ->
│  WARN: "Non-streaming model, text appears en bloc"
│  └─ Not an error, phase PARTIAL PASS
├─ WebSocket buffering -> console check for agui.text_message events
│  └─ Events arrive but UI buffers -> UI bug, screenshot + WARN
└─ SSE instead of WebSocket? -> architecture check, should be WS

Typing indicator doesn't disappear?
├─ agui.run_finished never received ->
│  ├─ Worker crashed -> console logs
│  └─ NATS message lost -> wait 30s, then reload page
├─ UI bug: indicator stuck ->
│  Screenshot, reload page, check if response in DB
└─ Run still active (slow model) ->
   Wait up to 120s, then timeout diagnosis

Console shows errors?
├─ "WebSocket disconnected" -> wait for auto-reconnect
├─ "JSON parse error" -> malformed AG-UI event, screenshot + WARN
└─ No agui events at all ->
   WebSocket connection check, /ws endpoint reachable?
```

---

## Phase 5: Agentic Tool-Use

**Goal:** LLM uses tools (Read, Write, Bash etc.) — ToolCallCards appear in chat.

**Prerequisites:** Phase 3 passed + Phase 0 identified a tool-capable model. If no tool-capable model -> **SKIP Phase 5-7**.

### Steps

1. Send message: `Create a file called hello.py with a simple Hello World script` -> `browser_type` + `browser_click`
2. `browser_wait_for` -> ToolCallCard appears
3. `browser_snapshot` -> tool name visible? (e.g. `write_file` or `Write`)
4. Tool arguments visible? (filename, content)
5. Tool result visible? (success/error)
6. Wait for run completion -> `browser_wait_for`
7. Final `browser_snapshot` -> assistant message after tool use
8. Verify: file exists (second message: `Read the file hello.py` -> should show content)

### Validation

- ToolCallCard in chat visible with tool name and arguments
- Tool result shows success
- Assistant summarizes result
- File was actually created (verified via Read tool)

### Decision Tree

```
Model doesn't make tool call?
├─ Model not tool-capable ->
│  Phase 0 classification was wrong?
│  └─ Try different model from fallback list
│     └─ None tool-capable -> SKIP Phase 5-7 with diagnosis
├─ Prompt too vague -> more explicit prompt:
│  "Use the write_file tool to create hello.py with print('Hello')"
├─ Model responds only with text ("Here's how you would...") ->
│  Stronger prompt: "Do NOT explain. Actually create the file using your tools."
└─ Model hallucinates tool use (says it wrote, but didn't) ->
   Verify via Read message, if file missing -> WARN

ToolCallCard not visible?
├─ Tool call happens but UI doesn't show ->
│  Console: check agui.tool_call event
│  ├─ Event present but UI doesn't render -> UI bug, screenshot
│  └─ Event missing -> Python worker not sending events
├─ HITL approval blocking (permission request) ->
│  Continue to Phase 6 (expected!)
└─ Tool call failed (error in result) ->
   Read error message, screenshot
   ├─ "Permission denied" -> autonomy level too restrictive
   ├─ "File not found" / "Path error" -> workspace path not set
   └─ "Command blocked" -> policy blocks tool

Run stuck (>60s no activity)?
├─ Pending permission request not visible? ->
│  Scroll page/reload, permission might be off-screen
├─ Worker stall -> console logs, NATS status
└─ Max iterations reached -> expected for complex task, WARN
```

---

## Phase 6: HITL Permissions

**Goal:** Test permission request system — approve, deny, allow-always.

**Prerequisites:** Phase 5 passed (tool use works).

### Steps

1. Set autonomy level to `supervised` (via UI or `/mode supervised`)
2. Send agentic message: `Run the command 'echo hello' in the terminal` -> `browser_type` + `browser_click`
3. `browser_wait_for` -> PermissionRequestCard appears
4. `browser_snapshot` -> permission details visible (tool, command, countdown)
5. **Test Deny:** `browser_click` on "Deny" button
6. `browser_snapshot` -> agent reacts to denial
7. New message: `Run 'echo test' in the terminal`
8. `browser_wait_for` -> new PermissionRequestCard
9. **Test Approve:** `browser_click` on "Approve"
10. `browser_wait_for` -> tool executes, result visible
11. New message: `Run 'echo again' in the terminal`
12. `browser_wait_for` -> PermissionRequestCard
13. **Test Allow-Always:** `browser_click` on "Allow Always"
14. New message: `Run 'echo final' in the terminal` -> should run WITHOUT permission

### Validation

- PermissionRequestCard shows tool name, command, countdown timer
- Deny -> agent receives denial, adapts behavior
- Approve -> tool executes, result appears
- Allow-Always -> subsequent same tool calls need no approval

### Decision Tree

```
PermissionRequestCard doesn't appear?
├─ Autonomy level not "supervised" ->
│  /mode command not available? -> set via API:
│  browser_evaluate: fetch('/api/v1/...', {method: 'PUT', ...})
├─ Policy preset allows everything ->
│  Check default preset, set to "supervised-ask-all"
├─ Tool call doesn't happen -> back to Phase 5 decision tree
└─ Permission UI exists but off-screen ->
   browser_evaluate: scrollIntoView on permission element

Countdown expires before action possible?
├─ Default timeout too short ->
│  WARN: "Approval timeout < 10s, barely testable interactively"
├─ UI doesn't respond to click ->
│  browser_snapshot, check button selector
└─ Multiple permissions simultaneously ->
   Answer first, next should follow

Allow-Always doesn't work?
├─ POST /policies/allow-always failed ->
│  Console logs, API response
├─ Rule saved but not effective ->
│  Policy cache, reload page + retest
└─ Next call still needs permission ->
   Tool name different? (bash vs Bash etc.)
```

---

## Phase 7: Full Project Creation

**Goal:** LLM creates a complete project in chat — multiple files, tests, structure.

**Prerequisites:** Phase 5+6 passed.

### Steps

1. Set allow-always for all tools (or autonomy to `auto-edit`)
2. Send comprehensive message:
   ```
   Create a complete Python CLI todo app with:
   - main.py with argparse for add/list/done/delete commands
   - todo.py with a TodoList class using JSON file storage
   - test_todo.py with pytest tests for all operations
   - requirements.txt with dependencies
   - README.md with usage instructions
   ```
3. Observe multi-step execution:
   - `browser_snapshot` every 10s -> document progress
   - Count tool calls (write calls for each file)
   - Observe step indicators (`agui.step_started`/`step_finished`)
4. Wait for run completion (up to 180s) -> `browser_wait_for`
5. Verify created files: `List all files you created` -> second message
6. Final `browser_snapshot` -> overall view
7. Optional: `Run the tests` -> observe Bash tool execution

### Validation

- At least 3 files created (write tool calls)
- No error states during execution
- Assistant delivers summary at the end
- Progress was visible in UI (multiple ToolCallCards)
- Cost badge shows costs > $0

### Decision Tree

```
Model stops after 1-2 tool calls?
├─ Max iterations too low ->
│  WARN: "MaxLoopIterations possibly < 10"
├─ Context window full ->
│  Try smaller task: "Create just main.py and test_main.py"
├─ Model "gives up" (says "I've created the files" without doing it) ->
│  Try stronger model from fallback list
└─ Budget limit reached -> WARN: "Budget exhausted"

Individual files missing?
├─ Write tool failed for one file ->
│  Check tool result in chat for errors
├─ Model forgot a file ->
│  Follow-up: "You forgot to create requirements.txt, please create it"
└─ Path problem (wrong directories) ->
   Check workspace path configuration

Run takes >180s?
├─ Still active (new tool calls coming) ->
│  Continue waiting, up to 300s, then timeout
├─ Stall (no activity >30s) ->
│  Permission request blocking? -> scroll, check
│  Worker stall -> console logs
└─ Infinite loop (same tool call repeated) ->
   WARN: "Agent loop detected", screenshot
```

---

## Phase 8: Cost Tracking

**Goal:** Verify per-message and total costs.

**Prerequisites:** Phase 3 passed (at least 1 conversation with responses).

### Steps

1. Open existing conversation from Phase 3+
2. `browser_snapshot` -> look for MessageBadge on assistant messages
3. Click MessageBadge -> `browser_click` -> CostBreakdown expands?
4. `browser_snapshot` -> token count (in/out), model name, cost in USD visible?
5. Send new message: `What is 2+2?` (cheap request)
6. `browser_wait_for` -> response arrives
7. `browser_snapshot` -> new MessageBadge with costs
8. Look for total cost display (conversation header or summary)

### Validation

- MessageBadge shows model name + token count + cost
- CostBreakdown expands with details
- Cost > $0.00 (not zero)
- Token counts plausible (input > 0, output > 0)

### Decision Tree

```
No MessageBadge visible?
├─ AG-UI state_delta events missing ->
│  Console: check agui.state_delta events?
│  ├─ Events present but UI doesn't render -> UI bug, screenshot
│  └─ Events missing -> Python worker not sending cost data
├─ Cost = $0.00 ->
│  LiteLLM cost tracking disabled? Local model without pricing?
│  └─ WARN: "Zero cost — possibly local model without pricing"
└─ Badge present but CostBreakdown doesn't expand ->
   Click handler missing? -> browser_evaluate: DOM event check

Token counts implausible?
├─ Input = 0 -> tracking bug, WARN
├─ Output = 0 but text present -> streaming token count missing
└─ Astronomically high -> model pricing table wrong in LiteLLM
```

---

## Phase 9: Slash Commands

**Goal:** Test all registered slash commands.

**Prerequisites:** Phase 2 passed.

### Steps

1. Type `/help` -> `browser_type` + `browser_press_key("Enter")`
2. `browser_snapshot` -> help text or command list appears?
3. Type `/model` -> Enter
4. `browser_snapshot` -> current model displayed?
5. Type `/mode` -> Enter
6. `browser_snapshot` -> current mode displayed?
7. Send several messages (for compact/rewind test)
8. `/compact` -> Enter
9. `browser_snapshot` -> compaction visible?
10. `/clear` -> Enter
11. `browser_snapshot` -> chat cleared?

### Validation

- `/help` shows available commands
- `/model` shows current model
- `/mode` shows current mode
- `/compact` compacts conversation
- `/clear` clears the chat
- Invalid commands (`/doesnotexist`) show error message

### Decision Tree

```
Slash command sent as normal message?
├─ CommandRegistry not loaded ->
│  Console logs, frontend error?
├─ "/" not recognized as command trigger ->
│  Input handler check: is "/" prefix intercepted?
└─ Command exists but sent to LLM ->
   Client-side vs server-side command handling check

/compact failed?
├─ API 404 -> endpoint not implemented?
│  └─ browser_evaluate: fetch('/api/v1/conversations/{id}/compact', {method:'POST'})
├─ API 500 -> worker problem during compaction
└─ UI doesn't change -> compact is server-side, UI refresh needed?

/rewind not testable?
├─ Needs checkpoints (agui.step_finished events) ->
│  Only available after agentic runs
├─ RewindTimeline doesn't appear ->
│  No checkpoints, SKIP with explanation
└─ Rewind executed but no visible change ->
   Count messages before/after
```

---

## Phase 10: Conversation Search

**Goal:** Test PostgreSQL full-text search over conversations.

**Prerequisites:** Phase 3 passed + at least 2 conversations with different content.

### Steps

1. Ensure at least 2 conversations exist with known content:
   - Conversation A: contains "compiler" (from Phase 4)
   - Conversation B: contains "hello.py" (from Phase 5)
2. Find search UI -> `browser_snapshot` (search field in sidebar or header?)
3. Enter search term: `compiler` -> `browser_type`
4. `browser_snapshot` -> results displayed?
5. Conversation A in results? Conversation B not?
6. New search: `hello.py`
7. `browser_snapshot` -> Conversation B in results?
8. Nonsense search: `xyznonexistent123`
9. `browser_snapshot` -> "No results" or empty list?

### Validation

- Search finds relevant conversations
- Search filters irrelevant ones
- Empty search shows "no results"
- Results are clickable and open the conversation

### Decision Tree

```
Search field not visible?
├─ Feature not exposed in UI ->
│  Test API directly: browser_evaluate:
│  fetch('/api/v1/search/conversations', {method:'POST',
│    headers:{'Content-Type':'application/json'},
│    body: JSON.stringify({query:'compiler'})})
│  ├─ API works -> UI bug, search exists but not visible
│  └─ API 404 -> feature not deployed -> SKIP
├─ Search field in different tab/view ->
│  Navigate through UI
└─ Search field present but inactive -> JavaScript error?

Search finds nothing despite content existing?
├─ FTS index not built ->
│  Migration 069 run? PostgreSQL GIN index?
│  └─ Only checkable via API, WARN
├─ Search term too specific ->
│  Try simpler term ("hello")
└─ Tenant isolation -> wrong tenant ID in query?
```

---

## Phase 11: Conversation Management

**Goal:** Test rewind, fork, multi-tab.

**Prerequisites:** Phase 3 passed + agentic conversation with checkpoints (Phase 5+).

### Steps

1. Open conversation with multiple messages
2. Find rewind feature -> `browser_snapshot`
3. Open rewind timeline (if available) -> `browser_click`
4. `browser_snapshot` -> checkpoints visible?
5. Click on earlier checkpoint -> `browser_click`
6. DiffSummaryModal appears? -> `browser_snapshot`
7. Confirm rewind -> `browser_click`
8. `browser_snapshot` -> conversation reset to earlier state?
9. Create new conversation (second tab) -> "New Conversation" button
10. `browser_snapshot` -> second conversation visible, first still there?
11. Switch between conversations -> tab click

### Validation

- Rewind timeline shows checkpoints
- Rewind resets conversation (fewer messages)
- DiffSummaryModal shows what will be lost
- Multiple conversations possible in parallel
- Tab switching works without data loss

### Decision Tree

```
Rewind timeline not available?
├─ No checkpoints present ->
│  Only after agentic runs (Phase 5+) ->
│  If Phase 5 skipped: SKIP rewind test
├─ UI element not visible ->
│  Try slash command /rewind
└─ Feature disabled -> API check:
   POST /conversations/{id}/rewind

Rewind changes nothing?
├─ API call successful but UI doesn't update ->
│  Reload page, count messages
├─ API failed -> console logs
└─ DiffSummaryModal shows no changes ->
   Already at earliest checkpoint

Multi-tab doesn't work?
├─ "New Conversation" button missing -> search UI
├─ New conversation overwrites old ->
│  State management bug, screenshot
└─ Tab switching loses messages ->
   Messages not stored per-conversation?
```

---

## Phase 12: Smart References

**Goal:** Test @mentions, #files, //commands with autocomplete.

**Prerequisites:** Phase 2 passed + project with files (Phase 5+).

### Steps

1. Focus chat input -> `browser_click`
2. Type `@` -> `browser_type("@")`
3. `browser_snapshot` -> AutocompletePopover appears?
4. Popover shows suggestions? (e.g. @project, @file)
5. Escape -> popover closes
6. Type `#` -> `browser_type("#")`
7. `browser_snapshot` -> file autocomplete appears?
8. Start typing filename: `#hello` -> `browser_type("hello")`
9. `browser_snapshot` -> filtered results (hello.py)?
10. Select entry -> `browser_click` or `browser_press_key("Enter")`
11. `browser_snapshot` -> TokenBadge in input visible?
12. Type `//` -> `browser_type("//")`
13. `browser_snapshot` -> command autocomplete?

### Validation

- `@` triggers mention autocomplete
- `#` triggers file autocomplete
- `//` triggers command autocomplete
- Typing filters results
- Selection inserts TokenBadge
- Popover closes on Escape

### Decision Tree

```
Autocomplete popover doesn't appear?
├─ Trigger character treated as text ->
│  AutocompletePopover component not loaded?
│  └─ Console logs
├─ Popover appears but empty ->
│  No data (no files, no mentions) ->
│  Files from Phase 5 present? If Phase 5 skipped:
│  └─ SKIP #file test, only test @ and //
└─ Popover appears at wrong position ->
   WARN: "Popover positioning issue", screenshot

TokenBadge doesn't appear after selection?
├─ Entry inserted as plain text ->
│  TokenBadge rendering missing
├─ Selection handler not connected ->
│  Console check
└─ Keyboard navigation doesn't work ->
   Try mouse click instead of Enter
```

---

## Phase 13: Notifications

**Goal:** Test notification system — bell, in-app, tab badge.

**Prerequisites:** Phase 3 passed.

### Steps

1. Find notification bell in header -> `browser_snapshot`
2. Click bell -> `browser_click` -> NotificationCenter opens?
3. `browser_snapshot` -> notification list (empty or with entries)
4. If empty: send a message that generates notification (e.g. agentic run completion)
5. Switch tab (open another browser tab) -> `browser_evaluate`: `document.hidden`
6. Send message (via API in background, or in other tab)
7. Switch back -> tab badge visible? (unread count in tab title)
8. Click bell again -> new notification?
9. Click notification -> navigates to relevant conversation?

### Validation

- Notification bell visible in header
- NotificationCenter opens on click
- New events generate notifications
- Tab badge shows unread count
- Notification click navigates correctly

### Decision Tree

```
Notification bell not visible?
├─ Header layout changed -> browser_snapshot, search all header elements
├─ Bell in different container (sidebar?) -> search navigation
└─ Feature not implemented ->
   API check: GET /notifications
   └─ 404 -> SKIP phase

No notifications despite activity?
├─ AG-UI events not wired ->
│  notificationStore not active?
├─ Browser permissions for push not granted ->
│  WARN: "Browser push notifications blocked, only in-app testable"
└─ Events arrive but store doesn't process ->
   Console logs, notification store debug

Tab badge doesn't work?
├─ document.title not updated ->
│  WARN: "Tab badge not updating"
├─ Web Audio sound missing ->
│  Audio autoplay policy blocked -> WARN
└─ Badge shows wrong number ->
   Count logic check
```

---

## Phase 14: Canvas Integration

**Goal:** Test visual design canvas — draw, export, multimodal chat.

**Prerequisites:** Phase 3 passed + model with vision support (optional).

### Steps

1. Open canvas -> `browser_snapshot`, find canvas tab/button
2. `browser_click` -> canvas area opens
3. `browser_snapshot` -> 7 tools visible? (Select, Rect, Ellipse, Freehand, Text, Annotate, Image)
4. Select rect tool -> `browser_click`
5. Draw rectangle -> `browser_drag` (from point A to point B)
6. `browser_snapshot` -> rectangle visible on canvas?
7. Select text tool -> `browser_click`
8. Enter text -> `browser_click` on canvas + `browser_type("Hello Canvas")`
9. `browser_snapshot` -> text visible on canvas?
10. Open export panel -> `browser_click`
11. `browser_snapshot` -> export options (PNG, ASCII, JSON)?
12. "Send to Chat" button -> `browser_click`
13. `browser_snapshot` -> canvas export in chat input/message?
14. Send chat message with canvas reference: `Describe what you see in this design`
15. `browser_wait_for` -> LLM response (vision model: describes image; else: JSON/ASCII)

### Validation

- Canvas renders SVG-based drawing area
- Tools work (rect, text minimum)
- Export produces output (PNG/ASCII/JSON depending on config)
- Canvas-to-chat pipeline works
- LLM reacts to canvas input

### Decision Tree

```
Canvas not visible/reachable?
├─ Feature tab missing -> Phase 32 not deployed?
│  └─ browser_evaluate: check feature flags
├─ Canvas renders but is empty/black ->
│  SVG rendering problem, console logs
└─ Canvas route doesn't exist -> SKIP phase

Drawing doesn't work?
├─ Drag event not recognized ->
│  browser_drag coordinates check, canvas bounds
├─ Tool selection doesn't work ->
│  Toolbar buttons check, active tool state
└─ Shapes appear but disappear ->
   State persistence bug

Export/send-to-chat failed?
├─ Export panel doesn't open -> button selector
├─ PNG export empty -> offscreen canvas problem
├─ Send-to-chat button missing -> feature not implemented
└─ LLM ignores canvas input ->
   Model has no vision -> WARN: "Non-vision model, canvas prompt degraded to JSON/ASCII"
```

---

## Phase 15: Report, Screenshots & Cleanup

**Goal:** Summary of all phase results, screenshots, cleanup.

**Prerequisites:** All other phases completed (or skipped).

### Steps

1. Collect results from all phases: PASS / PARTIAL / SKIP / FAIL
2. `browser_take_screenshot` -> final state of dashboard
3. Generate summary:
   - Which phases passed
   - Which phases skipped (with reason)
   - Which decision tree paths were activated
   - Discovered bugs/warnings
   - Total cost incurred
4. Delete test project (if desired)
5. Clean up conversations

### Result Format

```markdown
## QA Test Report — [Date]

| Phase | Status | Notes |
|-------|--------|-------|
| 0: Environment | PASS | 3 models found, gpt-4o-mini tool-capable |
| 1: Project Setup | PASS | E2E-Test-Project created |
| 2: Chat UI | PASS | All elements present |
| 3: Simple Message | PASS | Response in 2.3s |
| 4: Streaming | PASS | Progressive text confirmed |
| 5: Tool-Use | PASS | ToolCallCard rendered |
| 6: HITL | PASS | Approve/Deny/Allow-Always all work |
| 7: Full Project | PARTIAL | 4/5 files created, model skipped README |
| 8: Cost Tracking | PASS | $0.003 per message |
| 9: Slash Commands | PASS | 5/5 commands work |
| 10: Search | PASS | FTS returns correct results |
| 11: Management | PARTIAL | Rewind works, fork not found |
| 12: Smart Refs | PASS | @/# autocomplete works |
| 13: Notifications | SKIP | Bell not visible in header |
| 14: Canvas | PASS | Draw + export + chat pipeline works |

### Decision Tree Activations
- Phase 3: "No response after 30s" -> model timeout -> switched to gpt-4o-mini -> RESOLVED
- Phase 6: "PermissionRequestCard doesn't appear" -> autonomy not supervised -> set via API -> RESOLVED

### Bugs Found
- [BUG-001] MessageBadge CostBreakdown doesn't expand on click
- [WARN-001] Tab badge count incorrect after rewind

### Total Cost: $0.47
```

---

## Key Files Reference

| File | Role |
|------|------|
| `frontend/src/features/project/ChatPanel.tsx` | Main chat component |
| `frontend/src/features/chat/ChatInput.tsx` | Chat input with autocomplete triggers |
| `frontend/src/features/chat/AutocompletePopover.tsx` | @/#// autocomplete popover |
| `frontend/src/features/canvas/` | Visual design canvas (7 tools) |
| `frontend/src/features/notifications/NotificationCenter.tsx` | Notification UI |
| `frontend/e2e/helpers/api-helpers.ts` | API login/CRUD helpers |
| `frontend/e2e/helpers/ws-helpers.ts` | WebSocket test helpers |
| `internal/service/conversation.go` | Conversation dispatch logic |
| `internal/service/conversation_agent.go` | Agentic dispatch + HITL |
| `workers/codeforge/agent_loop.py` | Python agent loop executor |
| `frontend/playwright.config.ts` | Playwright browser test config |

## Usage

To run this test plan:

1. Ensure the full stack is running (see Prerequisites above)
2. Start a Claude Code session with `playwright-mcp` tools available
3. Tell Claude Code: "Follow the test plan at `docs/testing/2026-03-17-interactive-qa-testplan.md`"
4. Claude Code will execute phases sequentially (0-3), then core chat (4-7), then features (8-14) in parallel where possible
5. Phase 15 generates the final report
