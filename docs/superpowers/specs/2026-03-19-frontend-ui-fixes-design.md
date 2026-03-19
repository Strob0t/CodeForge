# Frontend UI Fixes — Design Spec

**Date:** 2026-03-19
**Scope:** 4 independent frontend tasks (bug fix + UI improvements)

---

## Task 1: File Browser Context Menu — Root Right-Click Bug

**Problem:** Right-clicking in the empty area below files in the FileTree does not open the root context menu.

**Root Cause:** `FileTree.tsx:356` checks `e.target === e.currentTarget` — this only fires when clicking directly on the container `div`. The container has no minimum height, so it shrinks to its content and there is no clickable empty space below the file list.

**Fix:** Add `min-h-full` to the outer `div` in `FileTree.tsx:353` so the container fills all available vertical space. The existing `e.target === e.currentTarget` check then works correctly for the empty area below files.

**Files:** `frontend/src/features/project/FileTree.tsx`

---

## Task 2: Chat Message Width — Wider Agent Messages with Tool Calls

**Problem:** Both user and agent messages use `max-w-[90%] sm:max-w-[75%]`. For agent messages containing ToolCallCards, this is too narrow — arguments and results appear in cramped `<pre>` blocks.

**Fix:** Agent messages that contain tool calls get `max-w-[95%] sm:max-w-[90%]` instead of the default constraint. User messages remain unchanged. This gives tool call content significantly more room without breaking responsive layout.

**Implementation:** In `ChatPanel.tsx`, detect whether an agent message has tool calls and apply the wider max-width class conditionally. Also apply the same wider width to the streaming tool call container (line ~910) for visual consistency.

**Files:** `frontend/src/features/project/ChatPanel.tsx`

---

## Task 3: Notification Click Navigation

**Problem:** Clicking a notification only marks it as read. The `actionUrl` field exists in the notification type but is never set.

**ProjectId Resolution:** AG-UI events only carry `run_id`, not `project_id`. The `AppShell` component (where notifications are created) has no direct project context. Solution: use `useLocation()` from SolidJS Router to extract `projectId` from the current URL path (`/projects/:id/...`). If the user is not on a project page when the event fires, `actionUrl` is simply not set (graceful degradation to current behavior).

**Fix:**
1. In `AppShell` (App.tsx), derive `projectId` from `useLocation().pathname` via regex match on `/projects/([^/]+)`.
2. Set `actionUrl` on each `addNotification()` call:
   - `permission_request` -> `/projects/{projectId}?tab=chat` (when projectId available)
   - `run_complete` / `run_failed` -> `/projects/{projectId}?tab=chat` (when projectId available)
   - `agent_message` -> `/projects/{projectId}?tab=chat` (when projectId available)
   - `info` -> no navigation (no `actionUrl`)
3. In `NotificationItem.tsx`, replace the existing conditional "View" `<a>` link with a click-to-navigate behavior on the entire notification item. Use `useNavigate()` from SolidJS Router. When `actionUrl` is present, clicking the notification both marks it as read AND navigates.

**Files:**
- `frontend/src/features/notifications/NotificationItem.tsx`
- `frontend/src/App.tsx`

---

## Task 4: Notification Sound — Default Off

**Problem:** Notification sound plays by default via Web Audio API oscillator.

**Fix:** Change `enableSound` default from `true` to `false` in `notificationSettings.ts`. Keep all sound infrastructure (settings UI, `playNotificationSound()`, sound types) intact. Users can still opt-in via settings.

**Files:** `frontend/src/features/notifications/notificationSettings.ts`

---

## Non-Goals

- No changes to notification sound implementation (kept, just default off)
- No changes to user message styling
- No changes to FileContextMenu actions or appearance
- No backend changes required (projectId derived from current URL, not from AG-UI events)
