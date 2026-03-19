# Frontend UI Fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix 4 independent frontend issues: context menu root-click bug, chat message width for tool calls, notification click navigation, notification sound default.

**Architecture:** Pure frontend changes across 5 files. No backend changes. All tasks are independent and can be implemented in any order.

**Tech Stack:** SolidJS, Tailwind CSS, @solidjs/router

**Spec:** `docs/superpowers/specs/2026-03-19-frontend-ui-fixes-design.md`

---

## File Map

| File | Action | Task |
|------|--------|------|
| `frontend/src/features/project/FileTree.tsx` | Modify (line 353) | 1 |
| `frontend/src/features/project/ChatPanel.tsx` | Modify (lines 814, 910) | 2 |
| `frontend/src/features/notifications/NotificationItem.tsx` | Modify (lines 1-111) | 3 |
| `frontend/src/features/notifications/NotificationCenter.tsx` | Modify (line 127) | 3 |
| `frontend/src/App.tsx` | Modify (lines 88-106) | 3 |
| `frontend/src/features/notifications/notificationSettings.ts` | Modify (line 17) | 4 |

---

### Task 1: Fix Context Menu Root Right-Click

**Files:**
- Modify: `frontend/src/features/project/FileTree.tsx:353`

- [ ] **Step 1: Add `min-h-full` to FileTree container**

In `FileTree.tsx` line 353, the outer `div` has `class="overflow-y-auto text-sm"`. Add `min-h-full` so it fills the parent container, creating clickable empty space below files.

```tsx
// Before (line 353):
<div
  class="overflow-y-auto text-sm"
  onContextMenu={(e: MouseEvent) => {

// After:
<div
  class="overflow-y-auto text-sm min-h-full"
  onContextMenu={(e: MouseEvent) => {
```

- [ ] **Step 2: Manual test**

1. Start frontend dev server: `cd frontend && npm run dev`
2. Open a project with files in the file browser
3. Right-click in the empty area below the last file
4. Verify the root context menu appears with "New File" and "New Folder"
5. Right-click on a file — verify the file context menu still works
6. Right-click on a folder — verify the folder context menu still works

- [ ] **Step 3: Commit**

```bash
git add frontend/src/features/project/FileTree.tsx
git commit -m "fix(ui): enable root right-click context menu in file browser

Add min-h-full to FileTree container so empty space below files
is clickable for the root-level context menu."
```

---

### Task 2: Widen Agent Messages with Tool Calls

**Files:**
- Modify: `frontend/src/features/project/ChatPanel.tsx:814,910`

- [ ] **Step 1: Widen persisted agent messages with tool calls**

In `ChatPanel.tsx` line 812-818, conditionally apply a wider max-width when the assistant message has tool calls.

```tsx
// Before (lines 812-818):
<li class={`flex ${msg.role === "user" ? "justify-end" : "justify-start"}`}>
  <div
    class={`max-w-[90%] sm:max-w-[75%] rounded-cf-md px-4 py-2 text-sm ${
      msg.role === "user"
        ? "bg-cf-accent text-white whitespace-pre-wrap"
        : "bg-cf-bg-surface-alt text-cf-text-primary"
    }`}
  >

// After:
<li class={`flex ${msg.role === "user" ? "justify-end" : "justify-start"}`}>
  <div
    class={`rounded-cf-md px-4 py-2 text-sm ${
      msg.role === "user"
        ? "max-w-[90%] sm:max-w-[75%] bg-cf-accent text-white whitespace-pre-wrap"
        : msg.tool_calls && msg.tool_calls.length > 0
          ? "max-w-[95%] sm:max-w-[90%] bg-cf-bg-surface-alt text-cf-text-primary"
          : "max-w-[90%] sm:max-w-[75%] bg-cf-bg-surface-alt text-cf-text-primary"
    }`}
  >
```

- [ ] **Step 2: Widen streaming tool calls container**

In `ChatPanel.tsx` line 910, apply the same wider width to the active tool calls container.

```tsx
// Before (line 910):
<div class="max-w-[90%] sm:max-w-[75%] w-full border-l-2 border-cf-accent/40 pl-3 ml-2">

// After:
<div class="max-w-[95%] sm:max-w-[90%] w-full border-l-2 border-cf-accent/40 pl-3 ml-2">
```

- [ ] **Step 3: Manual test**

1. Open a conversation with an agent that uses tool calls
2. Verify agent messages with tool calls are visibly wider than plain text messages
3. Verify user messages remain right-aligned and unchanged
4. Expand a tool call card — verify arguments and results have adequate width
5. Resize the browser window — verify responsive behavior still works
6. Check the streaming tool calls section appears with the wider width during active runs

- [ ] **Step 4: Commit**

```bash
git add frontend/src/features/project/ChatPanel.tsx
git commit -m "fix(ui): widen agent messages containing tool calls

Agent messages with tool calls now use max-w-[95%]/sm:max-w-[90%]
instead of max-w-[90%]/sm:max-w-[75%], giving tool call arguments
and results more horizontal space. Also applies to streaming tool calls."
```

---

### Task 3: Notification Click Navigation

**Files:**
- Modify: `frontend/src/App.tsx:88-106`
- Modify: `frontend/src/features/notifications/NotificationItem.tsx:1-111`

- [ ] **Step 1: Add `actionUrl` to notifications in App.tsx**

In `AppShell` (App.tsx), derive `projectId` from the current URL and pass it as `actionUrl` to all non-info notifications. Note: `useLocation` is already imported at line 2 but NOT yet called in `AppShell` — add the call there.

```tsx
// Insert after line 87 (`const { onAGUIEvent } = useWebSocket();`):
const location = useLocation();
const currentProjectId = (): string | undefined => {
  const match = location.pathname.match(/^\/projects\/([^/]+)/);
  return match ? match[1] : undefined;
};

// Then update the permission_request handler (lines 90-97):
const offPermission = onAGUIEvent("agui.permission_request", (ev) => {
  const projectId = currentProjectId();
  addNotification({
    type: "permission_request",
    title: "Approval Required",
    message: `Agent requests permission for: ${ev.tool || "action"}`,
    metadata: { run_id: ev.run_id, call_id: ev.call_id },
    actionUrl: projectId ? `/projects/${projectId}?tab=chat` : undefined,
  });
});

// And the run_finished handler (lines 99-106):
const offRunFinished = onAGUIEvent("agui.run_finished", (ev) => {
  const failed = ev.status === "failed" || ev.status === "error";
  const projectId = currentProjectId();
  addNotification({
    type: failed ? "run_failed" : "run_complete",
    title: failed ? "Run Failed" : "Run Complete",
    message: ev.error ? `Run ${ev.run_id}: ${ev.error}` : `Run ${ev.run_id} ${ev.status}`,
    actionUrl: projectId ? `/projects/${projectId}?tab=chat` : undefined,
  });
});
```

- [ ] **Step 2: Add click-to-navigate in NotificationItem.tsx**

Replace the click handler to navigate when `actionUrl` is present. Remove the separate "View" `<a>` link — the entire item becomes clickable for navigation. Also close the notification panel on navigate.

```tsx
// Add import at top of NotificationItem.tsx:
import { useNavigate } from "@solidjs/router";

// Add onClose to props interface (line 15):
export interface NotificationItemProps {
  notification: Notification;
  onMarkRead: (id: string) => void;
  onArchive: (id: string) => void;
  onClose?: () => void;
}

// Inside the component, replace handleClick (lines 46-49):
export default function NotificationItem(props: NotificationItemProps): JSX.Element {
  const navigate = useNavigate();

  function handleClick() {
    if (!props.notification.read) {
      props.onMarkRead(props.notification.id);
    }
    if (props.notification.actionUrl) {
      props.onClose?.();
      navigate(props.notification.actionUrl);
    }
  }

// Remove the "View" <a> link block (lines 87-97):
// DELETE this entire <Show> block:
//   <Show when={props.notification.type === "permission_request" && props.notification.actionUrl}>
//     <a href={...} ...>View</a>
//   </Show>
```

- [ ] **Step 3: Pass `onClose` from NotificationCenter to NotificationItem**

In `NotificationCenter.tsx` line 127-131, add the `onClose` prop:

```tsx
// Before:
<NotificationItem
  notification={notification}
  onMarkRead={markRead}
  onArchive={archiveNotification}
/>

// After:
<NotificationItem
  notification={notification}
  onMarkRead={markRead}
  onArchive={archiveNotification}
  onClose={props.onClose}
/>
```

- [ ] **Step 4: Manual test**

1. Navigate to a project page and trigger an agent run
2. Wait for a notification (permission request or run complete)
3. Click the notification bell to open the notification center
4. Click the notification — verify it navigates to `/projects/{id}?tab=chat`
5. Verify the notification panel closes after clicking
6. Verify the notification is also marked as read
7. Navigate away from the project page, verify an `info`-type notification (if any) does NOT navigate

- [ ] **Step 5: Commit**

```bash
git add frontend/src/App.tsx frontend/src/features/notifications/NotificationItem.tsx frontend/src/features/notifications/NotificationCenter.tsx
git commit -m "feat(ui): navigate to project chat when clicking notifications

Notifications now set actionUrl based on current project context.
Clicking a notification marks it as read AND navigates to the
project's chat tab. Uses useLocation() to derive projectId."
```

---

### Task 4: Notification Sound Default Off

**Files:**
- Modify: `frontend/src/features/notifications/notificationSettings.ts:17`

- [ ] **Step 1: Change default**

In `notificationSettings.ts` line 17, change `enableSound: true` to `enableSound: false`.

```typescript
// Before (line 17):
enableSound: true,

// After:
enableSound: false,
```

- [ ] **Step 2: Manual test**

1. Clear localStorage: `localStorage.removeItem("codeforge_notification_settings")`
2. Reload the page
3. Trigger a notification
4. Verify no sound plays
5. Open notification settings, enable sound, trigger notification — verify sound plays

- [ ] **Step 3: Commit**

```bash
git add frontend/src/features/notifications/notificationSettings.ts
git commit -m "fix(ui): disable notification sound by default

Change enableSound default from true to false. Users can still
opt-in via notification settings."
```
