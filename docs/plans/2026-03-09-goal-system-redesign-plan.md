# Goal System Redesign — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace the broken manage_goals HTTP-callback tool with a propose_goal AG-UI event tool, rewrite the goal-researcher mode with GSD questioning methodology, and add hybrid file+DB persistence.

**Architecture:** Python tool emits trajectory event → Go Core translates to AG-UI WebSocket event → Frontend renders GoalProposalCard with Approve/Edit/Reject → Approve triggers Frontend DB persist + Agent file write.

**Tech Stack:** Python (tool executor, runtime client), Go (AG-UI events, trajectory handler, mode preset), TypeScript/SolidJS (GoalProposalCard, ChatPanel handler)

**Design Doc:** `docs/specs/2026-03-09-goal-system-redesign-design.md`

---

## Task 1: Create `propose_goal` Python Tool

**Files:**
- Create: `workers/codeforge/tools/propose_goal.py`
- Reference: `workers/codeforge/tools/_base.py` (ToolDefinition, ToolResult, ToolExecutor protocol)
- Reference: `workers/codeforge/runtime.py:339` (publish_trajectory_event)
- Reference: `workers/codeforge/tools/manage_goals.py` (old tool, for comparison)

**Step 1: Write the failing test**

Create `workers/tests/tools/test_propose_goal.py`:

```python
"""Tests for the propose_goal tool."""

from __future__ import annotations

import json
from unittest.mock import AsyncMock

import pytest

from codeforge.tools.propose_goal import PROPOSE_GOAL_DEFINITION, ProposeGoalExecutor


class TestProposeGoalDefinition:
    def test_name(self):
        assert PROPOSE_GOAL_DEFINITION.name == "propose_goal"

    def test_required_fields(self):
        assert "required" in PROPOSE_GOAL_DEFINITION.parameters
        assert set(PROPOSE_GOAL_DEFINITION.parameters["required"]) == {"action", "kind", "title", "content"}

    def test_action_enum(self):
        props = PROPOSE_GOAL_DEFINITION.parameters["properties"]
        assert props["action"]["enum"] == ["create", "update", "delete"]

    def test_kind_enum(self):
        props = PROPOSE_GOAL_DEFINITION.parameters["properties"]
        assert props["kind"]["enum"] == ["vision", "requirement", "constraint", "state", "context"]


class TestProposeGoalExecutor:
    @pytest.fixture()
    def runtime(self):
        rt = AsyncMock()
        rt.publish_trajectory_event = AsyncMock()
        return rt

    @pytest.fixture()
    def executor(self, runtime):
        return ProposeGoalExecutor(runtime=runtime)

    @pytest.mark.asyncio()
    async def test_create_proposal_success(self, executor, runtime):
        args = {
            "action": "create",
            "kind": "requirement",
            "title": "User can search products",
            "content": "A search function that aggregates...",
            "priority": 90,
        }
        result = await executor.execute(args, "/tmp/workspace")

        assert result.success is True
        assert "User can search products" in result.output
        runtime.publish_trajectory_event.assert_called_once()
        event = runtime.publish_trajectory_event.call_args[0][0]
        assert event["event_type"] == "agent.goal_proposed"
        data = event["data"]
        assert data["action"] == "create"
        assert data["kind"] == "requirement"
        assert data["title"] == "User can search products"

    @pytest.mark.asyncio()
    async def test_create_missing_required_field(self, executor, runtime):
        args = {"action": "create", "kind": "requirement"}
        result = await executor.execute(args, "/tmp/workspace")
        assert result.success is False
        assert "title" in result.error or "content" in result.error
        runtime.publish_trajectory_event.assert_not_called()

    @pytest.mark.asyncio()
    async def test_update_requires_goal_id(self, executor, runtime):
        args = {"action": "update", "kind": "requirement", "title": "Updated", "content": "New content"}
        result = await executor.execute(args, "/tmp/workspace")
        assert result.success is False
        assert "goal_id" in result.error

    @pytest.mark.asyncio()
    async def test_delete_requires_goal_id(self, executor, runtime):
        args = {"action": "delete", "kind": "requirement", "title": "X", "content": "Y"}
        result = await executor.execute(args, "/tmp/workspace")
        assert result.success is False
        assert "goal_id" in result.error

    @pytest.mark.asyncio()
    async def test_default_priority(self, executor, runtime):
        args = {"action": "create", "kind": "vision", "title": "T", "content": "C"}
        await executor.execute(args, "/tmp/workspace")
        event = runtime.publish_trajectory_event.call_args[0][0]
        assert event["data"]["priority"] == 90

    @pytest.mark.asyncio()
    async def test_proposal_id_is_uuid(self, executor, runtime):
        args = {"action": "create", "kind": "vision", "title": "T", "content": "C"}
        await executor.execute(args, "/tmp/workspace")
        event = runtime.publish_trajectory_event.call_args[0][0]
        import uuid
        uuid.UUID(event["data"]["proposal_id"])  # raises if not valid UUID

    @pytest.mark.asyncio()
    async def test_unknown_action(self, executor, runtime):
        args = {"action": "invalid", "kind": "vision", "title": "T", "content": "C"}
        result = await executor.execute(args, "/tmp/workspace")
        assert result.success is False
```

**Step 2: Run test to verify it fails**

Run: `cd /workspaces/CodeForge && python -m pytest workers/tests/tools/test_propose_goal.py -v`
Expected: ImportError — module `codeforge.tools.propose_goal` does not exist.

**Step 3: Write the implementation**

Create `workers/codeforge/tools/propose_goal.py`:

```python
"""Agent tool for proposing project goals via AG-UI events."""

from __future__ import annotations

import logging
import uuid
from typing import Any

from ._base import ToolDefinition, ToolResult

logger = logging.getLogger(__name__)

PROPOSE_GOAL_DEFINITION = ToolDefinition(
    name="propose_goal",
    description=(
        "Propose a project goal for user review. The goal is NOT created "
        "until the user approves it in the UI. Use this after understanding "
        "the project through exploration and interview."
    ),
    parameters={
        "type": "object",
        "properties": {
            "action": {
                "type": "string",
                "enum": ["create", "update", "delete"],
                "description": "The proposal action.",
            },
            "kind": {
                "type": "string",
                "enum": ["vision", "requirement", "constraint", "state", "context"],
                "description": "Goal category.",
            },
            "title": {
                "type": "string",
                "description": "Goal title.",
            },
            "content": {
                "type": "string",
                "description": "Goal content in markdown.",
            },
            "priority": {
                "type": "integer",
                "description": "Priority 0-100, higher = more important (default 90).",
            },
            "goal_id": {
                "type": "string",
                "description": "Existing goal ID (required for update and delete).",
            },
        },
        "required": ["action", "kind", "title", "content"],
    },
    when_to_use=(
        "Use this tool to propose a project goal after exploring the codebase "
        "and interviewing the user. The user must approve before the goal is saved."
    ),
    output_format="Confirmation that the goal was proposed for user review.",
    common_mistakes=[
        "Proposing goals before understanding the project (skip Phase 1 and 2).",
        "Proposing all goals at once instead of one at a time.",
        "Missing goal_id when action is update or delete.",
    ],
)

_VALID_ACTIONS = {"create", "update", "delete"}


class ProposeGoalExecutor:
    """Emit a goal_proposal AG-UI event via the runtime trajectory stream."""

    def __init__(self, runtime: object) -> None:
        self._runtime = runtime

    async def execute(self, arguments: dict[str, Any], workspace_path: str) -> ToolResult:
        action = arguments.get("action", "")
        if action not in _VALID_ACTIONS:
            return ToolResult(output="", error=f"Unknown action: {action}. Use create, update, or delete.", success=False)

        title = arguments.get("title", "")
        content = arguments.get("content", "")

        if action == "create":
            if not title or not content:
                return ToolResult(output="", error="create requires title and content.", success=False)

        if action in ("update", "delete"):
            if not arguments.get("goal_id"):
                return ToolResult(output="", error=f"{action} requires goal_id.", success=False)

        proposal_id = str(uuid.uuid4())
        priority = arguments.get("priority", 90)

        event_data = {
            "event_type": "agent.goal_proposed",
            "data": {
                "proposal_id": proposal_id,
                "action": action,
                "kind": arguments.get("kind", ""),
                "title": title,
                "content": content,
                "priority": priority,
                "goal_id": arguments.get("goal_id"),
            },
        }

        await self._runtime.publish_trajectory_event(event_data)

        logger.info("goal proposed: %s (%s)", title, action)
        return ToolResult(output=f"Goal proposed for user review: {title}")
```

**Step 4: Run tests to verify they pass**

Run: `cd /workspaces/CodeForge && python -m pytest workers/tests/tools/test_propose_goal.py -v`
Expected: All 8 tests PASS.

**Step 5: Commit**

```bash
git add workers/codeforge/tools/propose_goal.py workers/tests/tools/test_propose_goal.py
git commit -m "feat(goals): add propose_goal tool with AG-UI trajectory event"
```

---

## Task 2: Wire `propose_goal` into Agent Loop

**Files:**
- Modify: `workers/codeforge/consumer/_conversation.py:104-105,749-753`
- Delete: `workers/codeforge/tools/manage_goals.py`

**Step 1: Write the failing test**

Create `workers/tests/consumer/test_propose_goal_registration.py`:

```python
"""Test that propose_goal is registered instead of manage_goals."""

from unittest.mock import AsyncMock, MagicMock

from codeforge.consumer._conversation import ConversationHandler


def test_register_propose_goal_tool():
    handler = ConversationHandler.__new__(ConversationHandler)
    handler._js = MagicMock()
    registry = MagicMock()
    runtime = AsyncMock()

    handler._register_propose_goal_tool(registry, runtime)

    registry.register.assert_called_once()
    defn = registry.register.call_args[0][0]
    assert defn.name == "propose_goal"
    executor = registry.register.call_args[0][1]
    assert hasattr(executor, "_runtime")
```

**Step 2: Run test to verify it fails**

Run: `cd /workspaces/CodeForge && python -m pytest workers/tests/consumer/test_propose_goal_registration.py -v`
Expected: FAIL — `ConversationHandler` has no `_register_propose_goal_tool`.

**Step 3: Modify `_conversation.py`**

In `_conversation.py`, replace:
- Line 105: `self._register_goals_tool(registry, run_msg.project_id)`
  → `self._register_propose_goal_tool(registry, runtime)`
- Lines 749-753: Replace `_register_goals_tool` method with:

```python
def _register_propose_goal_tool(self, registry: object, runtime: object) -> None:
    """Register the propose_goal tool for agent-driven goal proposals."""
    from codeforge.tools.propose_goal import PROPOSE_GOAL_DEFINITION, ProposeGoalExecutor

    registry.register(PROPOSE_GOAL_DEFINITION, ProposeGoalExecutor(runtime))
```

**Step 4: Delete `manage_goals.py`**

```bash
rm workers/codeforge/tools/manage_goals.py
```

Verify no other imports reference it:

```bash
grep -r "manage_goals" workers/
```

If any references remain in test files, delete those too.

**Step 5: Run tests to verify**

Run: `cd /workspaces/CodeForge && python -m pytest workers/tests/consumer/test_propose_goal_registration.py -v`
Expected: PASS.

Also run: `cd /workspaces/CodeForge && python -m pytest workers/ -v --timeout=30 -x`
Expected: No import errors for manage_goals.

**Step 6: Commit**

```bash
git add workers/codeforge/consumer/_conversation.py workers/tests/consumer/test_propose_goal_registration.py
git rm workers/codeforge/tools/manage_goals.py
git commit -m "feat(goals): wire propose_goal into agent loop, remove manage_goals"
```

---

## Task 3: Add `goal_proposal` AG-UI Event Type (Go)

**Files:**
- Modify: `internal/adapter/ws/agui_events.go` (add constant + struct)
- Modify: `internal/service/runtime.go:614-655` (handle `agent.goal_proposed` trajectory event)

**Step 1: Write the failing test**

Create `internal/adapter/ws/agui_events_test.go`:

```go
package ws_test

import (
    "encoding/json"
    "testing"

    "github.com/Strob0t/CodeForge/internal/adapter/ws"
)

func TestAGUIGoalProposalEventMarshal(t *testing.T) {
    ev := ws.AGUIGoalProposalEvent{
        RunID:      "run-123",
        ProposalID: "prop-456",
        Action:     "create",
        Kind:       "requirement",
        Title:      "User can search products",
        Content:    "A search function...",
        Priority:   90,
    }
    data, err := json.Marshal(ev)
    if err != nil {
        t.Fatalf("marshal: %v", err)
    }
    var got map[string]any
    if err := json.Unmarshal(data, &got); err != nil {
        t.Fatalf("unmarshal: %v", err)
    }
    if got["run_id"] != "run-123" {
        t.Errorf("run_id = %v, want run-123", got["run_id"])
    }
    if got["proposal_id"] != "prop-456" {
        t.Errorf("proposal_id = %v, want prop-456", got["proposal_id"])
    }
    if got["kind"] != "requirement" {
        t.Errorf("kind = %v, want requirement", got["kind"])
    }
}
```

**Step 2: Run test to verify it fails**

Run: `cd /workspaces/CodeForge && go test ./internal/adapter/ws/ -run TestAGUIGoalProposal -v`
Expected: FAIL — `ws.AGUIGoalProposalEvent` undefined.

**Step 3: Add event constant and struct to `agui_events.go`**

Add after the existing constants (around line 18):

```go
AGUIGoalProposal = "agui.goal_proposal"
```

Add struct after existing structs (around line 85):

```go
// AGUIGoalProposalEvent is sent when the agent proposes a goal for user review.
type AGUIGoalProposalEvent struct {
    RunID      string `json:"run_id"`
    ProposalID string `json:"proposal_id"`
    Action     string `json:"action"`
    Kind       string `json:"kind"`
    Title      string `json:"title"`
    Content    string `json:"content"`
    Priority   int    `json:"priority"`
    GoalID     string `json:"goal_id,omitempty"`
}
```

**Step 4: Add trajectory event handler in `runtime.go`**

In `runtime.go`, inside the trajectory event subscriber (around line 647, after `s.hub.BroadcastEvent(msgCtx, ws.EventTrajectoryEvent, ...)`), add:

```go
// Goal proposal events get a dedicated AG-UI broadcast.
if payload.EventType == "agent.goal_proposed" {
    var proposal struct {
        Data struct {
            ProposalID string `json:"proposal_id"`
            Action     string `json:"action"`
            Kind       string `json:"kind"`
            Title      string `json:"title"`
            Content    string `json:"content"`
            Priority   int    `json:"priority"`
            GoalID     string `json:"goal_id"`
        } `json:"data"`
    }
    if err := json.Unmarshal(data, &proposal); err == nil {
        s.hub.BroadcastEvent(msgCtx, ws.AGUIGoalProposal, ws.AGUIGoalProposalEvent{
            RunID:      payload.RunID,
            ProposalID: proposal.Data.ProposalID,
            Action:     proposal.Data.Action,
            Kind:       proposal.Data.Kind,
            Title:      proposal.Data.Title,
            Content:    proposal.Data.Content,
            Priority:   proposal.Data.Priority,
            GoalID:     proposal.Data.GoalID,
        })
    }
}
```

**Step 5: Run tests**

Run: `cd /workspaces/CodeForge && go test ./internal/adapter/ws/ -run TestAGUIGoalProposal -v`
Expected: PASS.

Run: `cd /workspaces/CodeForge && go test ./internal/service/ -run TestRuntime -v -count=1`
Expected: Existing tests still pass.

**Step 6: Commit**

```bash
git add internal/adapter/ws/agui_events.go internal/adapter/ws/agui_events_test.go internal/service/runtime.go
git commit -m "feat(goals): add agui.goal_proposal event type and trajectory handler"
```

---

## Task 4: Add `goal_proposal` AG-UI Event Type (TypeScript)

**Files:**
- Modify: `frontend/src/api/websocket.ts:10-74` (add type + interface + map entry)

**Step 1: Read current file to understand exact line structure**

Read: `frontend/src/api/websocket.ts`

**Step 2: Add to `AGUIEventType` union**

Add `| "agui.goal_proposal"` to the type union (around line 18).

**Step 3: Add interface**

Add after existing interfaces (around line 62):

```typescript
export interface AGUIGoalProposal {
  run_id: string;
  proposal_id: string;
  action: "create" | "update" | "delete";
  kind: "vision" | "requirement" | "constraint" | "state" | "context";
  title: string;
  content: string;
  priority: number;
  goal_id?: string;
}
```

**Step 4: Add to `AGUIEventMap`**

Add to the map (around line 74):

```typescript
"agui.goal_proposal": AGUIGoalProposal;
```

**Step 5: Commit**

```bash
git add frontend/src/api/websocket.ts
git commit -m "feat(goals): add agui.goal_proposal TypeScript types"
```

---

## Task 5: Create GoalProposalCard Frontend Component

**Files:**
- Create: `frontend/src/features/project/GoalProposalCard.tsx`
- Reference: `frontend/src/api/client.ts` (api.goals.create)
- Reference: `frontend/src/api/types.ts` (CreateGoalRequest, GoalKind)

**Step 1: Read API types for goals**

Read: `frontend/src/api/types.ts` — find `CreateGoalRequest` and `GoalKind` types.
Read: `frontend/src/api/client.ts` — find `api.goals.create` method signature.

**Step 2: Create the component**

Create `frontend/src/features/project/GoalProposalCard.tsx`:

```tsx
import { createSignal, Show } from "solid-js";

import { api } from "~/api/client";
import type { AGUIGoalProposal } from "~/api/websocket";
import type { GoalKind } from "~/api/types";
import { Button } from "~/ui";

interface Props {
  proposal: AGUIGoalProposal;
  projectId: string;
  onApprove: (title: string) => void;
  onReject: (title: string) => void;
}

const KIND_LABELS: Record<string, string> = {
  vision: "Vision",
  requirement: "Requirement",
  constraint: "Constraint",
  state: "Current State",
  context: "Context",
};

const KIND_COLORS: Record<string, string> = {
  vision: "text-green-400 bg-green-400/10 border-green-400/30",
  requirement: "text-blue-400 bg-blue-400/10 border-blue-400/30",
  constraint: "text-amber-400 bg-amber-400/10 border-amber-400/30",
  state: "text-cf-text-secondary bg-cf-bg-secondary border-cf-border",
  context: "text-cf-text-secondary bg-cf-bg-secondary border-cf-border",
};

export default function GoalProposalCard(props: Props) {
  const [status, setStatus] = createSignal<"pending" | "approved" | "rejected">("pending");
  const [saving, setSaving] = createSignal(false);

  const handleApprove = async () => {
    setSaving(true);
    try {
      await api.goals.create(props.projectId, {
        kind: props.proposal.kind as GoalKind,
        title: props.proposal.title,
        content: props.proposal.content,
        priority: props.proposal.priority,
        source: "agent",
      });
      setStatus("approved");
      props.onApprove(props.proposal.title);
    } catch {
      setSaving(false);
    }
  };

  const handleReject = () => {
    setStatus("rejected");
    props.onReject(props.proposal.title);
  };

  return (
    <div
      class={`rounded-cf-md border p-4 my-2 ${
        status() === "approved"
          ? "border-green-500/50 bg-green-500/5"
          : status() === "rejected"
            ? "border-red-500/30 bg-red-500/5 opacity-60"
            : "border-cf-border bg-cf-bg-secondary"
      }`}
    >
      <div class="flex items-center gap-2 mb-2">
        <span
          class={`text-xs font-semibold px-2 py-0.5 rounded-full border ${
            KIND_COLORS[props.proposal.kind] || KIND_COLORS.context
          }`}
        >
          {KIND_LABELS[props.proposal.kind] || props.proposal.kind}
        </span>
        <span class="text-xs text-cf-text-tertiary">
          {props.proposal.action === "create" ? "New Goal" : props.proposal.action}
        </span>
      </div>

      <h4 class="text-sm font-semibold text-cf-text-primary mb-1">{props.proposal.title}</h4>

      <Show when={props.proposal.content}>
        <p class="text-xs text-cf-text-secondary whitespace-pre-wrap mb-3">
          {props.proposal.content.length > 500
            ? props.proposal.content.slice(0, 500) + "..."
            : props.proposal.content}
        </p>
      </Show>

      <Show
        when={status() === "pending"}
        fallback={
          <span
            class={`text-xs font-medium ${
              status() === "approved" ? "text-green-400" : "text-red-400"
            }`}
          >
            {status() === "approved" ? "Approved" : "Rejected"}
          </span>
        }
      >
        <div class="flex items-center gap-2">
          <Button
            variant="primary"
            size="sm"
            onClick={handleApprove}
            disabled={saving()}
            loading={saving()}
          >
            Approve
          </Button>
          <Button variant="secondary" size="sm" onClick={handleReject}>
            Reject
          </Button>
        </div>
      </Show>
    </div>
  );
}
```

**Step 3: Commit**

```bash
git add frontend/src/features/project/GoalProposalCard.tsx
git commit -m "feat(goals): add GoalProposalCard component with Approve/Reject"
```

---

## Task 6: Wire GoalProposalCard into ChatPanel

**Files:**
- Modify: `frontend/src/features/project/ChatPanel.tsx:175-286`

**Step 1: Read the current ChatPanel**

Read: `frontend/src/features/project/ChatPanel.tsx` — understand where AG-UI event handlers are registered and where tool calls are rendered.

**Step 2: Add goal proposal state and handler**

In the ChatPanel component, add:

1. Import at top:
```typescript
import GoalProposalCard from "./GoalProposalCard";
import type { AGUIGoalProposal } from "~/api/websocket";
```

2. New signal (near other signals):
```typescript
const [goalProposals, setGoalProposals] = createSignal<AGUIGoalProposal[]>([]);
```

3. New AG-UI subscription (near other `onAGUIEvent` calls):
```typescript
const cleanupGoalProposal = onAGUIEvent("agui.goal_proposal", (payload) => {
  if (payload.run_id === activeRunId()) {
    setGoalProposals((prev) => [...prev, payload]);
  }
});
```

4. Add to cleanup:
```typescript
cleanupGoalProposal();
```

5. Clear proposals on run start (with other resets):
```typescript
setGoalProposals([]);
```

6. Render GoalProposalCard in the message stream. Find where tool calls are rendered and add nearby:
```tsx
<For each={goalProposals()}>
  {(proposal) => (
    <GoalProposalCard
      proposal={proposal}
      projectId={projectId()}
      onApprove={(title) => sendMessage(`[Goal approved: ${title}]`)}
      onReject={(title) => sendMessage(`[Goal rejected: ${title}]`)}
    />
  )}
</For>
```

Note: `sendMessage` is the existing function that sends a user message in the chat. Check the exact function name in ChatPanel. The Approve/Reject buttons send a synthetic message that becomes the next user turn.

**Step 3: Commit**

```bash
git add frontend/src/features/project/ChatPanel.tsx
git commit -m "feat(goals): wire GoalProposalCard into ChatPanel AG-UI stream"
```

---

## Task 7: Rewrite `goal-researcher` Mode Prompt (Go)

**Files:**
- Modify: `internal/domain/mode/presets.go:882-920`
- Test: `internal/domain/mode/mode_test.go`

**Step 1: Read the current mode definition**

Read: `internal/domain/mode/presets.go:878-923` — the current `goal-researcher` mode.

**Step 2: Replace the mode definition**

Replace the entire `goal-researcher` mode block (lines 882-920) with the GSD-style prompt from the design doc. Key changes:

- `Tools`: `["Read", "Glob", "Grep", "ListDir", "propose_goal", "Write"]`
- `DeniedTools`: `["Edit", "Bash"]` (Write allowed, Edit/Bash denied)
- `LLMScenario`: `"think"` (keep same)
- `Autonomy`: `2` (keep same)
- `PromptPrefix`: The full GSD-style prompt including `<questioning_guide>` XML block from the design doc (Section "Goal-Researcher Mode Prompt" → PromptPrefix Structure). Include the file templates inline.

The prompt is long (~3KB). Use Go raw string literals or string concatenation.

**Step 3: Run existing mode tests**

Run: `cd /workspaces/CodeForge && go test ./internal/domain/mode/ -v`
Expected: All tests pass. The existing test checks that `goal-researcher` exists and has correct fields.

**Step 4: Update test if needed**

If the existing test checks specific tools in the `goal-researcher` mode, update it to expect `propose_goal` instead of `manage_goals`.

**Step 5: Commit**

```bash
git add internal/domain/mode/presets.go internal/domain/mode/mode_test.go
git commit -m "feat(goals): rewrite goal-researcher mode with GSD questioning methodology"
```

---

## Task 8: Add Context Injection for `docs/*.md` Files

**Files:**
- Modify: `internal/adapter/http/handlers_goals.go:100-148` (AIDiscoverProjectGoals handler)
- Modify: `internal/service/conversation_agent.go` (SendMessageAgenticWithMode — add context entries)
- Reference: `internal/port/messagequeue/schemas.go` (ContextEntryPayload)

**Step 1: Read ContextEntryPayload structure**

Read: `internal/port/messagequeue/schemas.go` — find `ContextEntryPayload` struct.

**Step 2: Read the AIDiscoverProjectGoals handler**

Read: `internal/adapter/http/handlers_goals.go:97-148`.

**Step 3: Add docs file reading to AIDiscoverProjectGoals**

Before the `SendMessageAgenticWithMode` call (line 132), read the three GSD files from the workspace:

```go
// Inject existing goal files as context for the agent.
var contextEntries []messagequeue.ContextEntryPayload
for _, name := range []string{"docs/PROJECT.md", "docs/REQUIREMENTS.md", "docs/STATE.md"} {
    filePath := filepath.Join(proj.WorkspacePath, name)
    content, err := os.ReadFile(filePath)
    if err != nil {
        continue // File doesn't exist yet — that's fine
    }
    contextEntries = append(contextEntries, messagequeue.ContextEntryPayload{
        Source:  name,
        Content: string(content),
    })
}
```

Then pass `contextEntries` to `SendMessageAgenticWithMode`. Check if the method signature accepts context entries — if not, it needs a new parameter or the handler needs to call a lower-level method that does.

**Step 4: Verify the context entries reach the NATS payload**

Trace from `SendMessageAgenticWithMode` → the NATS publish call. Ensure `contextEntries` is set on `ConversationRunStartPayload.Context`.

**Step 5: Write a test**

Add a test in `internal/adapter/http/handlers_goals_test.go` that verifies:
- When `docs/PROJECT.md` exists in workspace, it appears in the NATS payload's context
- When `docs/PROJECT.md` doesn't exist, context is empty (no error)

**Step 6: Commit**

```bash
git add internal/adapter/http/handlers_goals.go internal/service/conversation_agent.go internal/adapter/http/handlers_goals_test.go
git commit -m "feat(goals): inject docs/*.md as context entries in goal discovery"
```

---

## Task 9: Clean Up and Integration Testing

**Files:**
- Verify: No remaining references to `manage_goals` anywhere
- Modify: `docs/todo.md` — mark goal system tasks complete, add new tasks if discovered

**Step 1: Verify no dangling references**

```bash
grep -r "manage_goals" --include="*.py" --include="*.go" --include="*.ts" --include="*.tsx" .
```

Expected: No results (except maybe in git history or docs).

**Step 2: Run full Python test suite**

Run: `cd /workspaces/CodeForge && python -m pytest workers/ -v --timeout=60`
Expected: All tests pass.

**Step 3: Run full Go test suite**

Run: `cd /workspaces/CodeForge && go test ./... -count=1 -timeout=120s`
Expected: All tests pass.

**Step 4: Run frontend build**

Run: `cd /workspaces/CodeForge/frontend && npm run build`
Expected: No TypeScript errors.

**Step 5: Run pre-commit**

Run: `cd /workspaces/CodeForge && pre-commit run --all-files`
Expected: All checks pass.

**Step 6: Update docs/todo.md**

Mark the goal system rework as complete. Reference the design doc.

**Step 7: Final commit**

```bash
git add docs/todo.md
git commit -m "docs: mark goal system redesign as complete"
```

---

## Task Summary

| # | Task | Files Changed | Depends On |
|---|------|---------------|------------|
| 1 | Create `propose_goal` Python tool | New: `propose_goal.py`, test | - |
| 2 | Wire into agent loop, delete `manage_goals` | `_conversation.py`, delete `manage_goals.py` | 1 |
| 3 | Go AG-UI event type + trajectory handler | `agui_events.go`, `runtime.go` | - |
| 4 | TypeScript AG-UI types | `websocket.ts` | - |
| 5 | GoalProposalCard component | New: `GoalProposalCard.tsx` | 4 |
| 6 | Wire into ChatPanel | `ChatPanel.tsx` | 4, 5 |
| 7 | Rewrite goal-researcher mode prompt | `presets.go` | - |
| 8 | Context injection for docs/*.md | `handlers_goals.go`, `conversation_agent.go` | - |
| 9 | Cleanup and integration test | Various | 1-8 |

**Parallelizable:** Tasks 1, 3, 4, 7, 8 are independent and can run in parallel.
**Sequential:** 1→2, 4→5→6, all→9.
