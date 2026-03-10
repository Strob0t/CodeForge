# Feature 5: Chat Enhancements

> **Status:** Implemented (2026-03-10)
> **Branch:** `feature/chat-enhancements`
> **Plan:** [docs/plans/2026-03-09-chat-enhancements-plan.md](../plans/2026-03-09-chat-enhancements-plan.md)
> **Design:** [docs/plans/2026-03-09-chat-enhancements-design.md](../plans/2026-03-09-chat-enhancements-design.md)

## Overview

10 features transforming CodeForge's chat from a basic message interface into a full-featured, interactive development workspace. Built on the existing AG-UI event protocol, Go hexagonal architecture, and SolidJS reactive frontend.

## Features

### 1. HITL Permission UI + Autonomy Mapping (Phase 1)

**What:** Visual approve/deny cards for agent permission requests with countdown timer.

- `supervised-ask-all` policy preset (blocks all tool calls, requires explicit approval)
- Auto-mapping of autonomy levels 1-5 to policy presets via `AutonomyToPreset()`
- `PermissionRequestCard` component with approve/deny buttons, countdown bar, tool name display
- WebSocket `permission_request` / `permission_response` events

**Files:** `internal/domain/policy/presets.go`, `frontend/src/features/project/PermissionRequestCard.tsx`

### 2. Inline Diff Review (Phase 2)

**What:** Side-by-side diff preview for file-modifying tool calls before approval.

- `DiffPreview` component with split-pane old/new view, syntax-highlighted deletions/additions
- Integrated into `PermissionRequestCard` for write/edit tool calls
- `GET /api/v1/projects/{id}/files/content` endpoint for fetching current file content

**Files:** `frontend/src/components/DiffPreview.tsx`, `internal/adapter/http/handlers_files.go`

### 3. Action Buttons (Phase 3)

**What:** Quick-action buttons on agent messages for common follow-up operations.

- `MessageActions` component with context-sensitive buttons (Copy, Retry, Apply, View Diff)
- Copy-to-clipboard for code blocks, retry failed messages, apply suggested changes
- Buttons appear on hover/focus for each message bubble

**Files:** `frontend/src/features/project/MessageActions.tsx`

### 4. Cost Tracking per Message (Phase 4)

**What:** Per-message cost display with token breakdown.

- `MessageBadge` component showing cost, input/output tokens, and model name
- `CostBreakdown` expandable panel with detailed token counts
- Wired to AG-UI `state_delta` events carrying cost metadata

**Files:** `frontend/src/features/project/MessageBadge.tsx`, `frontend/src/features/project/CostBreakdown.tsx`

### 5. Smart References with Autocomplete (Phase 5)

**What:** `@mention`, `#file`, and `//command` triggers with fuzzy autocomplete popover.

- `AutocompletePopover` with keyboard navigation (arrow keys, Enter, Escape)
- Three trigger types: `@` for agents/users, `#` for files/projects, `//` for commands
- `useFrequencyTracker` hook for sorting suggestions by usage frequency
- `TokenBadge` for rendering resolved references inline

**Files:** `frontend/src/features/project/AutocompletePopover.tsx`, `frontend/src/features/project/ChatInput.tsx`

### 6. Slash Commands (Phase 6)

**What:** `/command` system for chat operations like `/compact`, `/rewind`, `/clear`.

- `CommandRegistry` with built-in commands: `/compact`, `/rewind`, `/clear`, `/help`, `/mode`, `/model`
- `POST /api/v1/conversations/{id}/compact` endpoint for context compaction
- `POST /api/v1/conversations/{id}/rewind` endpoint with event timeline picker
- `DiffModal` for reviewing changes before applying rewind
- Rewind timeline picker showing conversation checkpoints

**Files:** `frontend/src/features/project/CommandRegistry.ts`, `internal/adapter/http/handlers_conversation_control.go`

### 7. Conversation Search (Phase 7)

**What:** Full-text search across conversation messages with PostgreSQL FTS.

- Migration 069: GIN index on `conversation_messages.content` for `to_tsvector('english', content)`
- `POST /api/v1/search/conversations` endpoint with `plainto_tsquery` and `ts_rank` ordering
- `ConversationResults` component with role-colored badges and content truncation
- Tabs UI in SearchPage: Code | Conversations
- `search_conversations` agent tool for programmatic search

**Files:** `internal/adapter/postgres/migrations/069_add_conversation_fts_index.sql`, `internal/adapter/http/handlers_search.go`, `frontend/src/features/search/ConversationResults.tsx`

### 8. Notification Center (Phase 8)

**What:** In-app notification system with browser push, sound alerts, and tab badge.

- `notificationStore` module-level SolidJS store (max 50 notifications, 5 types)
- `NotificationBell` with unread count badge in sidebar header
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

- Migration 070: `channels`, `channel_messages`, `channel_members` tables with FTS
- Domain model: `Channel` (project/bot types), `Message` (user/agent/bot/webhook senders), `Member` with roles
- Channel service with validation, bot-only deletion, webhook key generation (`crypto/rand`)
- 9 HTTP endpoints: list/create/get/delete channels, list/send messages, thread replies, member notify settings, webhook ingress
- WebSocket events: `channel_message`, `channel_typing`, `channel_read`
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
| POST | `/api/v1/search/conversations` | Full-text search across conversation messages |
| POST | `/api/v1/conversations/{id}/compact` | Compact conversation context |
| POST | `/api/v1/conversations/{id}/rewind` | Rewind conversation to checkpoint |
| GET | `/api/v1/channels` | List channels |
| POST | `/api/v1/channels` | Create channel |
| GET | `/api/v1/channels/{id}` | Get channel |
| DELETE | `/api/v1/channels/{id}` | Delete channel (bot-only) |
| GET | `/api/v1/channels/{id}/messages` | List channel messages |
| POST | `/api/v1/channels/{id}/messages` | Send channel message |
| POST | `/api/v1/channels/{id}/messages/{mid}/replies` | Send thread reply |
| PUT | `/api/v1/channels/{id}/members/me/notify` | Update notification setting |
| POST | `/api/v1/channels/{id}/webhook/{key}` | Webhook message ingress |

## Database Migrations

- **069**: GIN index for conversation message full-text search
- **070**: channels, channel_messages, channel_members tables

## WebSocket Events Added

- `channel_message` — new message in a channel
- `channel_typing` — user typing indicator
- `channel_read` — read receipt / cursor update
