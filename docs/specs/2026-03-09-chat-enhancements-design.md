# Chat Enhancements — Design Document

**Date:** 2026-03-09
**Status:** Approved
**Scope:** 10 features across 3 groups (Chat Interaction, Chat Input, Infrastructure)

## Overview

This design document covers 10 chat enhancement features inspired by Open WebUI and Claude Code, plus a future-scoped Voice & Video feature. The goal is to transform CodeForge's chat from a basic agent interface into a full-featured, interactive development workspace.

### Feature Summary

| # | Feature | Group | Priority |
|---|---|---|---|
| 1 | HITL Permission UI + Autonomy Mapping | Chat Interaction | High |
| 2 | Inline Diff View + Accept/Reject | Chat Interaction | High |
| 3 | Action Buttons (Hybrid) | Chat Interaction | High |
| 4 | Model-Badge + Cost + Context Gauge | Chat Interaction | Medium |
| 5 | `@`/`#`/`/` References (Dynamic) | Chat Input | Medium |
| 6 | `/compact` + `/rewind` + Chat Commands | Chat Input | Medium |
| 7 | Chat History Search + Agent Tool | Infrastructure | Medium |
| 8 | Browser Notifications + Notification Center | Infrastructure | Medium |
| 9 | Channels (Project + Bot) | Infrastructure | High (large) |
| 10 | Voice & Video | Future | Low |

---

## 1. HITL Permission UI + Autonomy Mapping

### Problem

The HITL backend is fully implemented (NATS request/response, `agui.permission_request` event, `POST /approve/{callId}` endpoint), but the frontend has no UI to display permission requests or let users approve/deny tool calls. Additionally, the 5 autonomy levels (1-5) are not automatically mapped to policy profiles.

### Autonomy-to-Policy Auto-Mapping

When a run starts without an explicit PolicyProfile, the Go Core derives it from the Mode's autonomy level:

| Autonomy Level | Policy Preset | Default Decision |
|---|---|---|
| 1 (supervised) | `supervised-ask-all` (new) | All tools = "ask" |
| 2 (semi-auto) | `headless-safe-sandbox` | Destructive = "ask", read/edit = "allow" |
| 3 (auto-edit) | `headless-safe-sandbox` | Terminal/deploy = "ask", rest = "allow" |
| 4 (full-auto) | `trusted-mount-autonomous` | All = "allow" (safety rules apply) |
| 5 (headless) | `trusted-mount-autonomous` | All = "allow" |

A new preset `supervised-ask-all` is needed (all tools default to "ask").

At autonomy levels 4+5 the `agui.permission_request` event is never emitted, so the PermissionRequestCard is never rendered. No special frontend logic required.

### Frontend: PermissionRequestCard

New event listener in ChatPanel for `agui.permission_request`. Renders inline:

```
+-- Permission Request ----------------------------------+
|                                                        |
|  Tool: bash                                            |
|  Command: rm -rf /tmp/test                             |
|  Path: /workspaces/CodeForge                           |
|                                                        |
|  ############-------- 42s remaining                    |
|                                                        |
|  [Allow]  [Allow Always]  [Deny]                       |
+--------------------------------------------------------+
```

- **Allow** -> `POST /api/v1/runs/{id}/approve/{callId}` with `{decision: "allow"}`
- **Allow Always** -> same call + persists a policy rule for this tool
- **Deny** -> same call with `{decision: "deny"}`
- Timeout countdown (60s), auto-deny on expiry

### Files Affected

- `internal/service/conversation_agent.go` — Autonomy-to-Policy mapping at run start
- `internal/domain/policy/presets.go` — New preset `supervised-ask-all`
- `frontend/src/api/websocket.ts` — Add `agui.permission_request` event type
- `frontend/src/features/project/ChatPanel.tsx` — Event handler
- `frontend/src/features/project/PermissionRequestCard.tsx` — New component
- `frontend/src/api/client.ts` — `approveToolCall()` API method

---

## 2. Inline Diff View + Accept/Reject

### Problem

When the agent edits files, the ToolCallCard shows "Result: success" with no visibility into what changed. Users need to see diffs and be able to accept or reject changes.

### Data Flow

Python Worker `edit` and `write` tools return structured diff data alongside the result text:

```json
{
  "result": "File edited successfully",
  "diff": {
    "path": "internal/auth.go",
    "hunks": [
      {
        "old_start": 40, "old_lines": 5,
        "new_start": 40, "new_lines": 6,
        "old_content": "func validateToken(t string) bool {\n  ...",
        "new_content": "func validateToken(t string) error {\n  ..."
      }
    ]
  }
}
```

Transported via the existing `agui.tool_result` event payload. No new event types needed.

### Display Modes

**Unified Diff (default, inline in chat):**
- Single block with red (deleted) and green (added) lines
- Compact, fits in chat bubble
- Buttons: `[Accept]` `[Reject]` `[Side-by-Side]` `[Full File]`

**Side-by-Side (toggle):**
- Click `[Side-by-Side]` opens a modal/slide-over panel with two columns
- Syntax highlighting via existing Markdown component

### Accept/Reject Flow

- **Accept:** No action needed (file is already written). Button turns green, card minimizes.
- **Reject:** `POST /api/v1/runs/{runId}/revert/{callId}` — new endpoint. Go Core restores the file from checkpoint. Agent receives "User rejected edit" as feedback.
- **Full File:** Opens complete file in read-only panel with change highlighted.

### Checkpoint System

Go Core stores pre-edit file content for every Edit/Write operation (checkpoint pattern like Claude Code). On revert, the old content is restored and an `agui.tool_reverted` event is sent.

### Files Affected

- `workers/codeforge/tools/edit.py` / `write.py` — Include diff data in result
- `workers/codeforge/tools/read.py` — Cache pre-edit content for diff calculation
- `internal/adapter/http/handlers_conversation.go` — New revert endpoint
- `internal/service/checkpoint.go` — New service: file checkpoint store/restore
- `frontend/src/features/project/ToolCallCard.tsx` — Integrate diff rendering
- `frontend/src/features/project/DiffView.tsx` — New component (unified)
- `frontend/src/features/project/DiffModal.tsx` — New component (side-by-side modal)

---

## 3. Action Buttons (Hybrid)

### Concept

Two sources for buttons: **fixed rules per tool type** (frontend) + **agent-suggested custom actions** (backend).

### Fixed Tool-Type Actions (Frontend Rules)

Mapping in `actionRules.ts`:

| Tool Result | Buttons |
|---|---|
| `edit`/`write` success | `[Accept]` `[Reject]` `[Show Diff]` |
| `bash` exit code != 0 | `[Retry]` `[Fix]` `[Show Full Output]` |
| `bash` with test output | `[Re-run Tests]` `[Show Coverage]` |
| `search`/`grep` | `[Open File]` `[Search Again]` |
| `read` | `[Edit File]` `[Open in Editor]` |
| Run completed (all edits) | `[Commit All]` `[Revert All]` `[Show All Diffs]` |
| Run failed | `[Retry Run]` `[Show Logs]` |

Rules match on tool name + result pattern (exit code, keyword "FAIL" in output).

### Agent Custom Actions (New AG-UI Event)

New event type: `agui.action_suggestion`

```json
{
  "type": "agui.action_suggestion",
  "actions": [
    {
      "id": "deploy-staging",
      "label": "Deploy to Staging",
      "icon": "rocket",
      "style": "primary",
      "action": { "type": "send_message", "message": "Deploy the changes to staging environment" }
    }
  ]
}
```

### Action Types

| Action Type | Behavior |
|---|---|
| `send_message` | Sends a predefined message into chat (new agent turn) |
| `api_call` | Calls an API endpoint (e.g., Revert, Approve) |
| `navigate` | Navigates to a route (e.g., Diff panel, File view) |
| `clipboard` | Copies content to clipboard |

### Rendering

Tool-type buttons appear first, agent suggestions below (visually separated with dashed border).

### HITL Integration

Permission Requests and Action Buttons are complementary:
- Permission Request: "May I do this?" -> before execution
- Action Buttons: "What should happen next?" -> after execution

### Files Affected

- `frontend/src/features/project/ActionBar.tsx` — New component for button bar
- `frontend/src/features/project/actionRules.ts` — Tool-type to button mapping
- `frontend/src/api/websocket.ts` — `agui.action_suggestion` event type
- `frontend/src/features/project/ChatPanel.tsx` — Integrate ActionBar
- `internal/adapter/ws/agui_events.go` — Event definition
- `workers/codeforge/agent_loop.py` — Emit action suggestions after tool calls

---

## 4. Model-Badge + Cost + Context Gauge

### Model-Badge per Message

Every assistant message gets a footer line:

```
model-name . provider . $cost . latency
```

Clicking the cost value expands a breakdown:

```
+-- Cost Breakdown ------+
| Input:   1,247 tok $0.0001 |
| Output:    683 tok $0.0019 |
| Cache:     512 tok (hit)   |
| Tools:  3 calls    $0.0010 |
| Total:             $0.0030 |
+----------------------------+
```

### Data Sources

| Data | Source | Status |
|---|---|---|
| Model name | `run_started` event / LiteLLM response | Available, not displayed |
| Provider | Derived from model name (mapping table) | Frontend mapping |
| Cost per message | `cost_usd` in `tool_result` events | Available, not aggregated |
| Latency | Delta `run_started` -> first `text_message` | Calculable in frontend |
| Token counts | LiteLLM response `usage` field | Needs transport via AG-UI event |

### Change Required

The `agui.run_finished` event gets a `usage` field:

```json
{
  "type": "agui.run_finished",
  "status": "completed",
  "usage": {
    "input_tokens": 1247,
    "output_tokens": 683,
    "cache_read_tokens": 512,
    "total_cost_usd": 0.003
  }
}
```

Python Worker reads `usage` from LiteLLM response and forwards via NATS to Go Core.

### Session Footer

Permanent footer below chat input:

```
Model: gemini-2.5-flash | Steps: 12/50
Cost: $0.042 total | Context: ########-- 78%
```

### Context Gauge Colors

| Utilization | Color | Behavior |
|---|---|---|
| 0-60% | Green | Normal |
| 60-80% | Yellow | Notice |
| 80-95% | Orange | "Consider /compact" tooltip |
| 95-100% | Red | Auto-compact warning |

### Files Affected

- `frontend/src/features/project/MessageBadge.tsx` — Model + Provider + Cost + Latency
- `frontend/src/features/project/CostBreakdown.tsx` — Expandable cost detail
- `frontend/src/features/project/SessionFooter.tsx` — Permanent footer
- `frontend/src/features/project/ContextGauge.tsx` — Progress bar component
- `frontend/src/utils/providerMap.ts` — Model name to provider mapping
- `internal/adapter/ws/agui_events.go` — `usage` field in `run_finished`
- `workers/codeforge/agent_loop.py` — Extract + forward usage data from LiteLLM response

---

## 5. `@`/`#`/`/` References with Fuzzy Search + Frequency Ranking

### Trigger Mechanics

Chat input recognizes 3 trigger characters. Autocomplete opens when:
- Character is at line start, or
- A space precedes it (prevents false triggers in e.g. email addresses)

### Convention (Industry Standard)

| Symbol | Meaning | Examples |
|---|---|---|
| `@` | Address agents/people (notification) | `@architect`, `@gemini-2.5-flash`, `@steffen` |
| `#` | Reference resources (link) | `#codeforge`, `#src/auth.go`, `#TASK-42` |
| `/` | Execute commands (action) | `/compact`, `/cost`, `/rewind` |

### Autocomplete Dropdown

Unified `AutocompletePopover` component for all three triggers:
- Fuzzy search with frequency ranking
- Grouped by category (Built-in, Skills, MCP Tools, etc.)
- Frequency counter in `localStorage` per user
- Top 5 by frequency when input is empty, then alphabetical
- Keyboard navigation: arrow keys, Enter to select, Esc to close, Tab to complete

### `/` Commands — Dynamic Registry System

Instead of a static frontend list, a Command Registry fed from multiple sources:

| Source | Type | Examples | Origin |
|---|---|---|---|
| Built-in Commands | Static (frontend) | `/compact`, `/clear`, `/help` | Hardcoded, always available |
| Agent Modes | Dynamic (API) | `/mode architect` | `GET /api/v1/modes` |
| Models | Dynamic (API) | `/model gemini-2.5-flash` | `GET /api/v1/models` |
| Skills | Dynamic (API) | `/skill deploy` | `GET /api/v1/skills` |
| MCP Tools | Dynamic (API) | `/tool github-search` | `GET /api/v1/mcp/tools` |
| User Custom Commands | Dynamic (project config) | `/deploy-staging` | `.codeforge/commands/*.yaml` |

**Aggregated endpoint:** `GET /api/v1/commands` — Go Core aggregates all sources.

**User Custom Commands** (`.codeforge/commands/deploy.yaml`):

```yaml
name: deploy-staging
label: Deploy to staging
icon: rocket
description: Build and deploy to staging environment
action:
  type: send_message
  message: "Run the deployment pipeline for staging: build, test, deploy"
```

### `@` Agents — Data Sources

| Source | API | Examples |
|---|---|---|
| Agent Modes | `GET /api/v1/modes` | `@architect`, `@coder` |
| Models | `GET /api/v1/models` | `@gemini-2.5-flash` |
| Team Members | `GET /api/v1/users` (when channels active) | `@steffen` |

### `#` Resources — Data Sources

| Source | API | Examples |
|---|---|---|
| Projects | `GET /api/v1/projects` | `#codeforge` |
| Files | `GET /api/v1/projects/{id}/files?query=...` | `#src/auth.go` |
| Knowledge Bases | `GET /api/v1/knowledge` (new endpoint) | `#api-docs` |
| Issues/Tasks | `GET /api/v1/projects/{id}/tasks` | `#TASK-42` |

Projects are cached. Files are lazy-loaded on-type (after 2+ characters after `#`).

### Fuzzy Search Algorithm

Frontend-only, no server call:

1. Exact prefix first (`/co` -> `/compact` before `/cost`)
2. Substring match next (`/act` -> `/compact`)
3. Fuzzy match fallback (Levenshtein <= 2, `/compct` -> `/compact`)
4. Sort: frequency descending, then alphabetical at equal frequency

### Selection Behavior

| Trigger | Behavior |
|---|---|
| `/compact` | Executed as command (no message sent) |
| `/mode architect` | Opens sub-dropdown, switches mode on selection |
| `@architect` | Inserted as token badge, sets agent mode for this message |
| `@gemini-2.5-flash` | Inserted as token badge, overrides model for this message |
| `#codeforge` | Inserted as token badge, project context appended to message |
| `#src/auth.go` | Inserted as token badge, file content injected |

### Token Badges

Inserted `@`/`#` references displayed as colored chips in the input (like Slack), not as plaintext. Deletable via Backspace.

### Files Affected

- `frontend/src/features/chat/AutocompletePopover.tsx` — Central autocomplete component
- `frontend/src/features/chat/TokenBadge.tsx` — Colored chips for inserted references
- `frontend/src/features/chat/ChatInput.tsx` — New chat input with trigger detection
- `frontend/src/features/chat/fuzzySearch.ts` — Fuzzy match + frequency ranking logic
- `frontend/src/features/chat/commandStore.ts` — Cached commands from API, merged with frequency
- `frontend/src/hooks/useFrequencyTracker.ts` — localStorage frequency counter
- `internal/adapter/http/handlers_commands.go` — New aggregated commands endpoint
- `internal/service/commands.go` — Command aggregation from Modes, Skills, MCP, Custom YAML
- `internal/domain/command/command.go` — Command domain model
- `internal/adapter/http/handlers_knowledge.go` — New knowledge list endpoint
- `internal/adapter/http/handlers_project.go` — File listing endpoint (for `#` files)

---

## 6. `/compact` + `/rewind` + Chat Commands

### Command Implementations

**`/compact` — Compress context**

1. Frontend sends `POST /api/v1/conversations/{id}/compact`
2. Go Core publishes NATS `conversation.compact.request`
3. Python Worker summarizes conversation via LLM
4. Worker replaces message history: system prompt + summary + last 3 messages
5. Go Core sends `agui.state_delta` event with new context gauge value
6. Frontend shows: "Context compressed: 78% -> 32%" as system message

**`/cost` — Show costs**

Frontend-only. Reads tracked cost sum from session state, renders as system message.

**`/rewind` — Rewind to previous step**

1. Frontend opens a timeline overlay with all agent steps
2. Each step shows: timestamp, summary, tool calls
3. User clicks a step -> confirmation dialog: "Code + Conversation / Code only / Conversation only"
4. Frontend sends `POST /api/v1/conversations/{id}/rewind` with `{step_id, mode}`
5. Go Core restores checkpoints (code) and/or trims messages (conversation)

**`/clear` — Clear chat**

Frontend clears local message list + `POST /api/v1/conversations/{id}/clear` marks conversation as completed. New conversation starts.

**`/mode <name>` — Switch agent mode**

Opens sub-dropdown with available modes. On selection: `POST /api/v1/conversations/{id}/mode`.

**`/model <name>` — Switch model**

Same as `/mode` but for models. `POST /api/v1/conversations/{id}/model`.

**`/help` — Show commands**

Frontend-only. Renders all available commands from command store as system message.

**`/explain` — Explain last result**

Sends automatic user message: "Explain the last tool result in detail". Normal agent turn.

**`/fix` — Fix last error**

Detects last failed tool call, sends: "Fix the error from the last tool call: {error_message}".

**`/diff` — Show all changes**

Frontend collects all `edit`/`write` tool results with diff data, opens modal with aggregated diff view.

### Command Type Overview

| Command | Type | Backend Call |
|---|---|---|
| `/compact` | Backend action | `POST /conversations/{id}/compact` |
| `/cost` | Frontend-only | No |
| `/rewind` | Backend action | `POST /conversations/{id}/rewind` |
| `/clear` | Backend action | `POST /conversations/{id}/clear` |
| `/mode` | Backend action | `POST /conversations/{id}/mode` |
| `/model` | Backend action | `POST /conversations/{id}/model` |
| `/help` | Frontend-only | No |
| `/explain` | Message shortcut | No (sends user message) |
| `/fix` | Message shortcut | No (sends user message) |
| `/diff` | Frontend-only | No |
| Custom Skills | Message shortcut | No (sends skill prompt) |

### Files Affected

- `frontend/src/features/chat/commandExecutor.ts` — Command dispatch: type-based execution
- `frontend/src/features/chat/RewindTimeline.tsx` — Timeline overlay component
- `frontend/src/features/chat/DiffSummaryModal.tsx` — Aggregated diff of all session changes
- `internal/adapter/http/handlers_conversation.go` — New endpoints: compact, rewind, clear, mode, model
- `internal/service/conversation.go` — Compact + Rewind + Mode-switch logic
- `workers/codeforge/consumer/_conversation.py` — Compact handler (LLM summarization)

---

## 7. Chat History Search + Agent Tool

### Global Search — New "Conversations" Tab

The existing SearchPage gets a new tab. Backend `POST /api/v1/search` gets a new scope value: `"conversations"`.

### Search Filters

- Project (optional)
- Model (optional)
- Role: user / assistant / all
- Date range: from / to

### SQL Implementation

PostgreSQL Full-Text Search (`tsvector`/`tsquery`) instead of `ILIKE`:

```sql
SELECT m.id, m.conversation_id, m.role, m.content,
       c.title, c.project_id, c.model, c.created_at,
       ts_rank(to_tsvector('english', m.content), plainto_tsquery($1)) AS rank
FROM conversation_messages m
JOIN conversations c ON c.id = m.conversation_id
WHERE c.tenant_id = $2
  AND to_tsvector('english', m.content) @@ plainto_tsquery($1)
  AND ($3 = '' OR c.project_id = $3)
  AND ($4 = '' OR m.role = $4)
  AND c.created_at BETWEEN $5 AND $6
ORDER BY rank DESC
LIMIT $7 OFFSET $8
```

Migration adds GIN index:

```sql
CREATE INDEX idx_conversation_messages_fts
ON conversation_messages USING GIN(to_tsvector('english', content));
```

### Agent Tool: `search_conversations`

New built-in tool for autonomous conversation history search:

```python
TOOL_DEFINITION = {
    "name": "search_conversations",
    "description": "Search through past conversation history to find relevant context, solutions, or decisions.",
    "parameters": {
        "query": {"type": "string", "description": "Search query"},
        "project_id": {"type": "string", "description": "Optional: limit to specific project"},
        "limit": {"type": "integer", "default": 5}
    }
}
```

Tool calls `POST /api/v1/search` with scope `conversations` internally.

### Files Affected

- `internal/adapter/http/handlers_search.go` — Add "conversations" scope
- `internal/service/search.go` — Conversation search logic
- `internal/adapter/store/store_search.go` — SQL query with full-text search
- `migrations/0XX_add_conversation_fts_index.sql` — GIN index
- `frontend/src/features/search/ConversationResults.tsx` — New tab content
- `frontend/src/features/search/SearchPage.tsx` — Add tab
- `workers/codeforge/tools/search_conversations.py` — New agent tool
- `workers/codeforge/tools/__init__.py` — Register tool

---

## 8. Browser Notifications + Notification Center

### Browser Push Notifications

Request `Notification.requestPermission()` on first chat start.

**Events that trigger notifications (only when tab is not focused):**

| Event | Text | Priority |
|---|---|---|
| `agui.run_finished` (completed) | "Agent finished: {summary}" | Normal |
| `agui.run_finished` (failed) | "Agent failed: {error}" | High |
| `agui.permission_request` | "Approval needed: {tool}" | High |
| Channel message | "{user}: {message} in #{channel}" | Normal |
| Permission timeout warning | "Approval expires in 15s" | Urgent |

### Sound + Tab Badge

- Configurable notification sound (Default / Chime / Bell / None)
- Tab title changes: `(3) CodeForge` when notifications pending
- Favicon switches to badge variant (red dot)
- Settings stored in `localStorage`

### Notification Center

Bell icon in top navigation with badge counter. Dropdown panel:

- Tabs: All / Unread / Archive
- Each notification is actionable (Permission Requests can be approved/denied inline)
- Max 50 notifications, oldest auto-removed
- Frontend-only state (SolidJS store), no DB model

### Files Affected

- `frontend/src/features/notifications/NotificationCenter.tsx` — Dropdown panel
- `frontend/src/features/notifications/NotificationBell.tsx` — Bell icon with badge
- `frontend/src/features/notifications/NotificationItem.tsx` — Single notification
- `frontend/src/features/notifications/notificationStore.ts` — SolidJS store + sound + browser push
- `frontend/src/features/notifications/notificationSettings.ts` — localStorage config
- `frontend/src/layouts/AppLayout.tsx` — Bell in navigation
- `frontend/src/utils/tabBadge.ts` — Tab title + favicon badge helper

---

## 9. Channels (Project + Bot)

### Concept

Two channel types, no free channel management:

| Type | Creation | Purpose |
|---|---|---|
| Project Channel | Automatic on project creation | Persistent team chat per project |
| Bot Channel | Manual by admin/user | CI/CD notifications, agent reports, webhooks |

### Database Schema

```sql
CREATE TABLE channels (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id),
    project_id  UUID REFERENCES projects(id),
    name        VARCHAR(100) NOT NULL,
    type        VARCHAR(20) NOT NULL CHECK (type IN ('project', 'bot')),
    description TEXT DEFAULT '',
    created_by  UUID REFERENCES users(id),
    created_at  TIMESTAMPTZ DEFAULT now(),
    UNIQUE(tenant_id, name)
);

CREATE TABLE channel_messages (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    channel_id  UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    sender_id   UUID REFERENCES users(id),
    sender_type VARCHAR(20) NOT NULL CHECK (sender_type IN ('user', 'agent', 'bot', 'webhook')),
    sender_name VARCHAR(100) NOT NULL,
    content     TEXT NOT NULL,
    metadata    JSONB DEFAULT '{}',
    parent_id   UUID REFERENCES channel_messages(id),
    created_at  TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE channel_members (
    channel_id  UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL REFERENCES users(id),
    role        VARCHAR(20) DEFAULT 'member' CHECK (role IN ('owner', 'admin', 'member')),
    notify      VARCHAR(20) DEFAULT 'all' CHECK (notify IN ('all', 'mentions', 'nothing')),
    joined_at   TIMESTAMPTZ DEFAULT now(),
    PRIMARY KEY (channel_id, user_id)
);

CREATE INDEX idx_channel_messages_channel ON channel_messages(channel_id, created_at DESC);
CREATE INDEX idx_channel_messages_thread ON channel_messages(parent_id) WHERE parent_id IS NOT NULL;
CREATE INDEX idx_channel_messages_fts ON channel_messages USING GIN(to_tsvector('english', content));
```

### API Endpoints

```
GET    /api/v1/channels                              — List all tenant channels
POST   /api/v1/channels                              — Create bot channel
GET    /api/v1/channels/{id}                         — Channel details
DELETE /api/v1/channels/{id}                         — Delete bot channel
GET    /api/v1/channels/{id}/messages                — Messages (cursor-paginated)
POST   /api/v1/channels/{id}/messages                — Send message
POST   /api/v1/channels/{id}/messages/{mid}/thread   — Thread reply
PUT    /api/v1/channels/{id}/members/{uid}           — Member settings (notify)
POST   /api/v1/channels/{id}/webhook                 — Webhook message (API key auth)
```

### WebSocket Real-Time Events

```json
{"type": "channel.message", "channel_id": "...", "message": {...}}
{"type": "channel.typing",  "channel_id": "...", "user": "steffen", "typing": true}
{"type": "channel.read",    "channel_id": "...", "user": "steffen", "last_read": "msg-id"}
```

Typing indicator sent via debounce (every 3s while user types).

### Agent Integration

1. **Automatic:** When an agent run completes in a project, posts a summary to the project channel (configurable: always / on_error / never)
2. **Explicit:** User `@`-mentions an agent in channel -> starts agent run whose output flows into the channel

### Bot Channel Webhooks

External systems post via webhook:

```
POST /api/v1/channels/{id}/webhook
Header: X-Webhook-Key: {channel-specific-key}
Body: { "sender_name": "GitHub Actions", "content": "Pipeline failed", "metadata": {...} }
```

Each bot channel gets a webhook key on creation.

### Relationship to War Room

| | War Room | Channels |
|---|---|---|
| Focus | Observe agent activity | Team communication |
| Content | Events, logs, tool calls | Messages, discussions |
| Interaction | Passive (read-only) | Active (write, react) |
| Persistence | Session-based | Permanent |

War Room remains as "Agent Dashboard". Channels are the "Team Chat".

### Files Affected

- `internal/domain/channel/channel.go` — Domain model
- `internal/adapter/store/store_channel.go` — PostgreSQL queries
- `internal/service/channel.go` — Channel service
- `internal/adapter/http/handlers_channel.go` — REST endpoints
- `internal/adapter/ws/channel_events.go` — WebSocket event handling
- `migrations/0XX_create_channels.sql` — DB schema
- `frontend/src/features/channels/ChannelList.tsx` — Sidebar channel list
- `frontend/src/features/channels/ChannelView.tsx` — Channel chat view
- `frontend/src/features/channels/ChannelMessage.tsx` — Single message
- `frontend/src/features/channels/ThreadPanel.tsx` — Thread view (slide-over)
- `frontend/src/features/channels/ChannelInput.tsx` — Input with `@`-mention support
- `frontend/src/layouts/AppLayout.tsx` — Channels in sidebar

---

## 10. Voice & Video (Future Feature — Scope Only)

**Status:** Documented for future implementation. No detailed design.

### Planned Scope

**Phase 1 — Voice Input/Output:**
- Mic button in chat input -> STT -> text as normal message
- TTS button on assistant messages -> read aloud
- Providers: Browser Web Speech API (free), Whisper (Ollama), Deepgram, OpenAI STT/TTS, ElevenLabs

**Phase 2 — Real-Time Voice Chat:**
- WebRTC bidirectional audio stream
- Streaming TTS for agent responses (token-by-token -> voice)
- Interruption handling
- Push-to-talk + Voice Activity Detection modes

**Phase 3 — Video + Screen Sharing:**
- User shares screen via WebRTC
- Screenshots periodically sent to multimodal LLM
- Agent "sees" what user sees and can react
- Optional: webcam feed for multimodal interaction

### Technology Assessment

| Component | Technology | Effort |
|---|---|---|
| STT | Web Speech API / Whisper | Small |
| TTS | SpeechSynthesis API / ElevenLabs | Small |
| Real-time Voice | WebRTC + Streaming TTS | Large |
| Screen Sharing | WebRTC getDisplayMedia + Vision LLM | Very Large |

No further design details. Separate design doc when prioritized.

---

## Cross-Cutting Concerns

### Tenant Isolation

All new database tables include `tenant_id`. All queries include `AND tenant_id = $N` with `tenantFromCtx(ctx)`. Channel webhooks validate tenant scope via API key.

### NATS Subjects

New subjects needed:

| Subject | Direction | Purpose |
|---|---|---|
| `conversation.compact.request` | Go -> Python | Compact conversation |
| `conversation.compact.complete` | Python -> Go | Compact result |
| `channel.agent.run` | Go -> Python | Agent run triggered from channel |

Must be added to JetStream stream config in `internal/port/messagequeue/jetstream.go`.

### Migration Sequence

New migrations required:
- `0XX_add_conversation_fts_index.sql` — GIN index for conversation search
- `0XX_create_channels.sql` — Channels schema (3 tables + indexes)

### No New Dependencies

All features use existing frontend stack (SolidJS signals/stores, Tailwind CSS, native fetch). No new npm packages. Fuzzy search is custom (~50 LOC). Diff rendering uses inline styles with Tailwind classes.
