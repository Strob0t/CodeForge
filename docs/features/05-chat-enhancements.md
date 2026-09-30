# Feature 5: Chat Enhancements

> **Status:** Implemented (2026-03-10), except Feature 3 (Action Buttons, planned) and Feature 10 (future scope)
> **Branch:** `feature/chat-enhancements`
> **Plan:** [docs/plans/2026-03-09-chat-enhancements-plan.md](../plans/2026-03-09-chat-enhancements-plan.md)
> **Design:** [docs/specs/2026-03-09-chat-enhancements-design.md](../specs/2026-03-09-chat-enhancements-design.md)

## Overview

10 features transforming CodeForge's chat from a basic message interface into a full-featured, interactive development workspace. Built on the existing AG-UI event protocol, Go hexagonal architecture, and SolidJS reactive frontend.

## Features

### 1. HITL Permission UI + Autonomy Mapping (Phase 1)

**What:** Visual approve/deny cards for agent permission requests with countdown timer.

- `supervised-ask-all` policy preset (blocks all tool calls, requires explicit approval)
- Auto-mapping of a mode's autonomy level (1-5) to a policy preset via `policyForAutonomy()` (`internal/service/conversation_dispatch.go`). The project-level `autonomy_level` saved by the compact settings popover has no effect (see [Known Issues](../todo.md#known-issues) KI-41)
- `PermissionRequestCard` component with approve/deny/allow-always buttons, countdown bar, tool name display and a display-only arguments preview (`arguments_preview`, never used for matching); a failed Allow-Always shows an error toast
- WebSocket `agui.permission_request` event; the decision is sent via `POST /api/v1/runs/{id}/approve/{callId}` and forwarded to the worker on NATS `runs.toolcall.response`
- **"Allow Always" persistence:** Clicking "Allow Always" approves the current call AND persists a permanent `allow` rule via `POST /api/v1/policies/allow-always` (admin). The card sends the `profile` that decided the call (from `agui.permission_request`); the rule is added to the per-project clone `{profile}-custom-{projectId}`, which replaces that profile only for this project (the project's profile selection is not changed). For Bash the rule is `command_allow` with every executable of the approved command (e.g. `cd frontend && npm test` -> `[cd, npm]`); commands that cannot be analysed are rejected (400). The rule never overrides a deny list. Idempotent (whole-rule comparison via `HasRule`). Persisted to `policy.custom_dir` (default `data/policies`); 404 for an unknown profile.

**Files:** `internal/domain/policy/presets.go`, `internal/domain/policy/policy.go`, `internal/service/policy.go`, `internal/service/project.go`, `internal/adapter/http/handlers_policy_crud.go`, `frontend/src/features/project/PermissionRequestCard.tsx`, `frontend/src/features/project/ChatPanel.tsx`

### 2. Inline Diff Review (Phase 2)

**What:** Side-by-side diff preview for file-modifying tool calls before approval.

- `DiffPreview` component: unified diff with old/new line-number columns, highlighted deletions/additions (split-pane view planned)
- Used in `TrajectoryPanel`; showing it inside `PermissionRequestCard` for write/edit tool calls before approval is planned
- `GET /api/v1/projects/{id}/files/content` endpoint for fetching current file content

**Files:** `frontend/src/components/DiffPreview.tsx`, `internal/adapter/http/handlers_files.go`

### 3. Action Buttons (Phase 3)

**What:** Quick-action buttons on agent messages for common follow-up operations.

> **Implementation status (2026-09-29):** Not implemented (planned). No `MessageActions` component exists and `ChatMessages.tsx` has no copy/retry/apply actions.

- `MessageActions` component with context-sensitive buttons (Copy, Retry, Apply, View Diff)
- Copy-to-clipboard for code blocks, retry failed messages, apply suggested changes
- Buttons appear on hover/focus for each message bubble

**Files (planned):** `frontend/src/features/project/MessageActions.tsx`

### 4. Cost Tracking per Message (Phase 4)

**What:** Per-message cost display with token breakdown.

- `MessageBadge` component showing cost, input/output tokens, and model name
- `CostBreakdown` expandable panel with detailed token counts
- Fed by cost data from `agui.tool_result` / `agui.run_finished` and the stored message tokens/model; `agui.state_delta` has no producer yet (see Gap below)

**Files:** `frontend/src/features/project/MessageBadge.tsx`, `frontend/src/features/project/CostBreakdown.tsx`

### 5. Smart References with Autocomplete (Phase 5)

**What:** `@mention`, `#file`, and `//command` triggers with fuzzy autocomplete popover.

- `AutocompletePopover` with keyboard navigation (arrow keys, Enter, Escape)
- Three trigger types: `@` for agents/users, `#` for files/projects, `//` for commands
- `useFrequencyTracker` hook for sorting suggestions by usage frequency
- `TokenBadge` for rendering resolved references inline

**Files:** `frontend/src/features/chat/AutocompletePopover.tsx`, `frontend/src/features/chat/ChatInput.tsx`

### 6. Slash Commands (Phase 6)

**What:** `/command` system for chat operations like `/compact`, `/rewind`, `/clear`.

- `CommandService` (`internal/service/commands.go`, `GET /api/v1/commands`) with built-in commands `/compact`, `/rewind`, `/clear`, `/diff`, `/cost`, `/help`, `/mode`, `/model`; the frontend `commandStore.ts` keeps a fallback list
- `POST /api/v1/conversations/{id}/compact` endpoint for context compaction
- `POST /api/v1/conversations/{id}/rewind` endpoint with event timeline picker
- `DiffSummaryModal` for `/diff` (session file changes). Reviewing a diff before applying a rewind is planned; today the timeline applies the rewind directly
- Rewind timeline picker showing conversation checkpoints

**Files:** `frontend/src/features/chat/commandStore.ts`, `frontend/src/features/chat/commandExecutor.ts`, compact handler in `internal/adapter/http/handlers_conversation.go`, rewind handler in `internal/adapter/http/handlers_session.go`

### 7. Conversation Search (Phase 7)

**What:** Full-text search across conversation messages with PostgreSQL FTS.

- Migration 069: GIN index on `conversation_messages.content` for `to_tsvector('english', content)`
- `POST /api/v1/search/conversations` endpoint with `plainto_tsquery` and `ts_rank` ordering
- `ConversationResults` component with role-colored badges and content truncation
- Tabs UI in SearchPage: Code | Conversations. Not reachable: `SearchPage` has no route since the sidebar restructure (c6831971); the API and the agent tool work
- `search_conversations` agent tool for programmatic search

**Files:** `internal/adapter/postgres/migrations/069_add_conversation_fts_index.sql`, `internal/adapter/http/handlers_search.go`, `frontend/src/features/search/ConversationResults.tsx`

### 8. Notification Center (Phase 8)

**What:** In-app notification system with browser push, sound alerts, and tab badge.

- `notificationStore` module-level SolidJS store (max 50 notifications, 5 types)
- `NotificationBell` with unread count badge in the top bar header
- `NotificationCenter` dropdown with All/Unread/Archived tabs and Mark All Read
- `NotificationItem` with type-colored left border, relative timestamp, archive on hover
- `notificationSettings` with localStorage persistence (push, sound, sound type)
- Browser Notification API integration (permission request, tab-hidden trigger)
- Web Audio API notification sounds (800Hz default, 440Hz subtle)
- `tabBadge` utility: `(N) CodeForge` title with auto-reset on window focus
- AG-UI event subscriptions: `permission_request` and `run_finished` auto-create notifications

**Files:** `frontend/src/features/notifications/notificationStore.ts`, `frontend/src/features/notifications/NotificationBell.tsx`, `frontend/src/features/notifications/NotificationCenter.tsx`, `frontend/src/utils/tabBadge.ts`

### 9. Real-Time Channels (Phase 9)

**What:** Slack-style messaging channels for project collaboration and bot integrations.

- Migration 071: `channels`, `channel_messages`, `channel_members` tables with FTS
- Domain model: `Channel` (project/bot types), `Message` (user/agent/bot/webhook senders), `Member` with roles
- Channel service with validation, bot-only deletion, webhook key generation (`crypto/rand`)
- 9 HTTP endpoints: list/create/get/delete channels, list/send messages, thread replies, member notify settings, webhook ingress
- WebSocket event types `channel.message`, `channel.typing`, `channel.read` are defined but never broadcast; `ChannelView` refetches only after the local user sends, so messages from others appear after a reload (see [Known Issues](../todo.md#known-issues) KI-42)
- `ChannelList` sidebar component with `#` (project) and `>` (bot) prefixes
- `ChannelView` with message list, auto-scroll, and input bar
- `ChannelMessage` with sender type badges and thread reply indicators
- `ThreadPanel` slide-over panel for threaded conversations
- Route: `/channels/:id`

**Files:** `internal/domain/channel/channel.go`, `internal/adapter/postgres/store_channel.go`, `internal/service/channel.go`, `internal/adapter/http/handlers_channel.go`, `frontend/src/features/channels/`

### 10. Voice & Video (Future Scope)

**What:** Real-time voice/video communication for pair programming with agents.

**Status:** Not implemented. Documented as future scope.

**Considerations:**
- WebRTC for peer-to-peer audio/video
- SFU (Selective Forwarding Unit) for multi-party calls
- Screen sharing for agent-assisted debugging
- Voice-to-text for hands-free agent interaction
- Integration with existing channel system for call initiation

## API Endpoints Added

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/policies/allow-always` | Persist "Allow Always" rule for a tool on a project |
| POST | `/api/v1/search/conversations` | Full-text search across conversation messages |
| POST | `/api/v1/conversations/{id}/compact` | Compact conversation context |
| POST | `/api/v1/conversations/{id}/rewind` | Rewind conversation to checkpoint |
| GET | `/api/v1/channels` | List channels |
| POST | `/api/v1/channels` | Create channel |
| GET | `/api/v1/channels/{id}` | Get channel |
| DELETE | `/api/v1/channels/{id}` | Delete channel (bot-only) |
| GET | `/api/v1/channels/{id}/messages` | List channel messages |
| POST | `/api/v1/channels/{id}/messages` | Send channel message |
| POST | `/api/v1/channels/{id}/messages/{mid}/thread` | Send thread reply |
| PUT | `/api/v1/channels/{id}/members/{uid}` | Update notification setting |
| POST | `/api/v1/channels/{id}/webhook` | Webhook message ingress |

## Database Migrations

- **069**: GIN index for conversation message full-text search
- **071**: channels, channel_messages, channel_members tables

## AG-UI Event Flow Architecture

The AG-UI (Agent-User Interaction) protocol streams 8 event types from agent execution to the frontend. Events do **not** originate in the Python worker as AG-UI messages. Instead, the Python worker publishes raw protocol messages to NATS, and the Go Core service translates them into AG-UI WebSocket events.

### Event Flow Path

```
Python Worker  --[NATS]--> Go Core Service --[WebSocket]--> Frontend
```

### Detailed Event Mapping

| AG-UI Event | Go Emitter | NATS Trigger | Source Files |
|---|---|---|---|
| `agui.run_started` | `ConversationService`, `RuntimeService` (direct) | Emitted before NATS publish | `conversation.go`, `conversation_dispatch.go`, `runtime.go` |
| `agui.run_finished` | `ConversationService`, `RuntimeService` | `conversation.run.complete`, `runs.complete` | `conversation_agent.go`, `conversation.go`, `conversation_dispatch.go`, `runtime_lifecycle.go` |
| `agui.text_message` | `RuntimeService` | `runs.output` | `runtime_subscribers.go` |
| `agui.tool_call` | `RuntimeService` | `runs.toolcall.request` | `runtime_execution.go` |
| `agui.tool_result` | `RuntimeService` | `runs.toolcall.result` | `runtime_execution.go` |
| `agui.step_started` | `OrchestratorService` | Direct (orchestration steps) | `orchestrator_consensus.go` |
| `agui.step_finished` | `OrchestratorService` | Direct (orchestration steps) | `orchestrator_consensus.go` |
| `agui.state_delta` | Not emitted | N/A | Defined in `agui.go` but no producer exists |

### Python Worker NATS Subjects

The Python `RuntimeClient` (`workers/codeforge/runtime.py`) publishes to these subjects:

- `runs.toolcall.request` -- permission request before each tool execution
- `runs.toolcall.result` -- outcome of each tool execution (success/error, cost, tokens)
- `runs.output` -- streaming LLM text chunks (per-token streaming via `send_output()`)
- `runs.trajectory.event` -- structured trajectory events (step_done, tool_called, stall_detected)
- `runs.heartbeat` -- periodic heartbeat for liveness detection

The Python `ConversationHandlerMixin` publishes `conversation.run.complete` with the full result.

### Go Core NATS Subscribers

Go subscribes to these subjects in `RuntimeService.StartSubscribers()` (`runtime.go`, handlers in `runtime_subscribers.go`); `conversation.run.complete` is subscribed by `ConversationService` (`conversation_agent.go`):

1. **`runs.output`** -- Broadcasts `EventTaskOutput` (native) + `AGUITextMessage` (AG-UI) for non-stderr, non-empty lines.
2. **`runs.toolcall.request`** -- Evaluates policy, stores run state, broadcasts `EventToolCallStatus` + `AGUIToolCall`, then publishes `runs.toolcall.response` back to Python.
3. **`runs.toolcall.result`** -- Updates run metrics, broadcasts `EventToolCallStatus` (phase=result) + `AGUIToolResult` with diff data.
4. **`runs.trajectory.event`** -- Persists to event store, broadcasts `EventTrajectoryEvent`, and additionally emits `AGUIActionSuggestion`, `AGUIGoalProposal` or `AGUIRoadmapProposal` for specific trajectory event types.
5. **`conversation.run.complete`** -- Handled by `ConversationService.HandleConversationRunComplete()` which stores messages in PostgreSQL and broadcasts `AGUIRunFinished`.

### Additional AG-UI Events

Beyond the NATS bridge, some AG-UI events are emitted directly by Go services:

- `agui.permission_request` -- Emitted by `RuntimeService` when a tool call requires HITL approval (`runtime_approval.go`)
- `agui.action_suggestion` -- Emitted via trajectory event bridge when agent suggests follow-up actions
- `agui.goal_proposal` -- Emitted via trajectory event bridge when agent proposes goals
- `agui.roadmap_proposal` -- Emitted via trajectory event bridge when the agent proposes a roadmap (`propose_roadmap`)

### Gap: `agui.state_delta`

The `state_delta` event type is defined in `internal/domain/event/agui.go` and typed in the frontend (`websocket.ts`, `types.ts`), but no producer exists in either Go or Python. This event is intended for partial state updates (JSON Patch / Merge Patch) and would be needed for features like real-time cost accumulation display or agent mode transitions. The per-message cost feature (Feature 4) currently uses `AGUIRunFinished` for final cost data rather than incremental `state_delta` updates.

## WebSocket Events Added

Defined in `internal/domain/event/broadcast.go` but not broadcast yet (KI-42):

- `channel.message` -- new message in a channel
- `channel.typing` -- user typing indicator
- `channel.read` -- read receipt / cursor update
