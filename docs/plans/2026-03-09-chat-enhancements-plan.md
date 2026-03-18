# Chat Enhancements Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Transform CodeForge's chat into a full-featured, interactive development workspace with 10 features across HITL, diff review, action buttons, cost tracking, smart references, chat commands, search, notifications, and channels.

**Architecture:** Frontend (SolidJS) + Go Core (chi, WebSocket, NATS) + Python Workers (LiteLLM, agent loop). All new features follow the existing AG-UI event pattern over WebSocket. New backend endpoints follow existing handler patterns. No new dependencies.

**Tech Stack:** SolidJS + Tailwind CSS (frontend), Go 1.25 + chi + pgx (backend), Python 3.12 + psycopg3 (workers), PostgreSQL 18, NATS JetStream.

**Design Doc:** `docs/specs/2026-03-09-chat-enhancements-design.md`

---

## Phase 1: HITL Permission UI + Autonomy Mapping

Dependencies: None (backend exists, frontend-only + small Go change)

### Task 1.1: New Policy Preset `supervised-ask-all`

**Files:**
- Modify: `internal/domain/policy/presets.go:141-170`
- Test: `internal/domain/policy/presets_test.go`

**Step 1: Write the failing test**

```go
func TestPresetSupervisedAskAll(t *testing.T) {
    p := PresetSupervisedAskAll()
    assert.Equal(t, "supervised-ask-all", p.Name)
    assert.Equal(t, ModeDefault, p.Mode)
    // All tools should default to "ask" via ModeDefault
    // No explicit allow rules
    assert.Empty(t, p.Rules)
}

func TestPresetByName_SupervisedAskAll(t *testing.T) {
    p, ok := PresetByName("supervised-ask-all")
    assert.True(t, ok)
    assert.Equal(t, "supervised-ask-all", p.Name)
}

func TestPresetNames_IncludesSupervisedAskAll(t *testing.T) {
    names := PresetNames()
    assert.Contains(t, names, "supervised-ask-all")
}
```

**Step 2: Run test to verify it fails**

Run: `cd /workspaces/CodeForge && go test ./internal/domain/policy/ -run TestPresetSupervisedAskAll -v`
Expected: FAIL — `PresetSupervisedAskAll` not defined

**Step 3: Write minimal implementation**

Add to `internal/domain/policy/presets.go` before `PresetNames()` (before line 141):

```go
// PresetSupervisedAskAll returns a profile where all tools require user approval.
// Used for autonomy level 1 (supervised).
func PresetSupervisedAskAll() Profile {
    return Profile{
        Name: "supervised-ask-all",
        Mode: ModeDefault, // defaults to "ask" for all unmatched tools
        Rules: []PermissionRule{
            {Specifier: ToolSpecifier{Tool: "Read"}, Decision: DecisionAllow},
        },
        QualityGate: QualityGate{},
        Termination: TerminationConfig{MaxSteps: 50},
    }
}
```

Update `PresetNames()` to include `"supervised-ask-all"`.
Update `PresetByName()` switch to include `case "supervised-ask-all": return PresetSupervisedAskAll(), true`.

**Step 4: Run test to verify it passes**

Run: `cd /workspaces/CodeForge && go test ./internal/domain/policy/ -run TestPreset -v`
Expected: All PASS

**Step 5: Commit**

```bash
git add internal/domain/policy/presets.go internal/domain/policy/presets_test.go
git commit -m "feat(policy): add supervised-ask-all preset for autonomy level 1"
```

---

### Task 1.2: Autonomy-to-Policy Auto-Mapping in Go Core

**Files:**
- Modify: `internal/service/conversation_agent.go:191-194,400-403`
- Test: `internal/service/conversation_agent_test.go`

**Step 1: Write the failing test**

```go
func TestAutonomyToPolicyMapping(t *testing.T) {
    tests := []struct {
        autonomy int
        expected string
    }{
        {1, "supervised-ask-all"},
        {2, "headless-safe-sandbox"},
        {3, "headless-safe-sandbox"},
        {4, "trusted-mount-autonomous"},
        {5, "trusted-mount-autonomous"},
        {0, "headless-safe-sandbox"}, // default fallback
    }
    for _, tc := range tests {
        t.Run(fmt.Sprintf("autonomy_%d", tc.autonomy), func(t *testing.T) {
            result := policyForAutonomy(tc.autonomy)
            assert.Equal(t, tc.expected, result)
        })
    }
}
```

**Step 2: Run test to verify it fails**

Run: `cd /workspaces/CodeForge && go test ./internal/service/ -run TestAutonomyToPolicyMapping -v`
Expected: FAIL — `policyForAutonomy` not defined

**Step 3: Write minimal implementation**

Add helper function in `internal/service/conversation_agent.go`:

```go
// policyForAutonomy maps an autonomy level (1-5) to a policy preset name.
func policyForAutonomy(autonomy int) string {
    switch autonomy {
    case 1:
        return "supervised-ask-all"
    case 4, 5:
        return "trusted-mount-autonomous"
    default: // 2, 3, 0, or unknown
        return "headless-safe-sandbox"
    }
}
```

Modify `SendMessageAgentic()` at line ~191 to use autonomy from mode:

```go
policyProfile := ""
if s.policySvc != nil {
    modeProfile := ""
    if activeMode != nil {
        modeProfile = policyForAutonomy(activeMode.Autonomy)
    }
    policyProfile = s.policySvc.ResolveProfile(modeProfile, proj.PolicyProfile)
}
```

Apply same change in `SendMessageAgenticWithMode()` at line ~400.

**Step 4: Run test to verify it passes**

Run: `cd /workspaces/CodeForge && go test ./internal/service/ -run TestAutonomyToPolicyMapping -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/service/conversation_agent.go internal/service/conversation_agent_test.go
git commit -m "feat(policy): auto-map autonomy levels to policy presets"
```

---

### Task 1.3: Frontend — Add `agui.permission_request` Event Type

**Files:**
- Modify: `frontend/src/api/websocket.ts:11-86`

**Step 1: Add event type and interface**

Add `"agui.permission_request"` to `AGUIEventType` union (line 20):

```typescript
export type AGUIEventType =
  | "agui.run_started"
  | "agui.run_finished"
  | "agui.text_message"
  | "agui.tool_call"
  | "agui.tool_result"
  | "agui.state_delta"
  | "agui.step_started"
  | "agui.step_finished"
  | "agui.goal_proposal"
  | "agui.permission_request";
```

Add interface after `AGUIGoalProposal` (after line 73):

```typescript
export interface AGUIPermissionRequest {
  run_id: string;
  call_id: string;
  tool: string;
  command?: string;
  path?: string;
}
```

Add to `AGUIEventMap` (after line 85):

```typescript
"agui.permission_request": AGUIPermissionRequest;
```

**Step 2: Verify frontend compiles**

Run: `cd /workspaces/CodeForge/frontend && npx tsc --noEmit`
Expected: No errors

**Step 3: Commit**

```bash
git add frontend/src/api/websocket.ts
git commit -m "feat(ws): add agui.permission_request event type to frontend"
```

---

### Task 1.4: Frontend — `PermissionRequestCard` Component

**Files:**
- Create: `frontend/src/features/project/PermissionRequestCard.tsx`

**Step 1: Create the component**

```typescript
import { createSignal, onCleanup, Show } from "solid-js";
import { api } from "../../api/client";

interface PermissionRequestCardProps {
  runId: string;
  callId: string;
  tool: string;
  command?: string;
  path?: string;
  timeoutSeconds?: number;
  onResolved?: (decision: "allow" | "deny") => void;
}

export default function PermissionRequestCard(props: PermissionRequestCardProps) {
  const timeout = props.timeoutSeconds ?? 60;
  const [remaining, setRemaining] = createSignal(timeout);
  const [resolved, setResolved] = createSignal<"allow" | "deny" | null>(null);
  const [loading, setLoading] = createSignal(false);

  const timer = setInterval(() => {
    setRemaining((r) => {
      if (r <= 1) {
        handleDecision("deny");
        return 0;
      }
      return r - 1;
    });
  }, 1000);

  onCleanup(() => clearInterval(timer));

  async function handleDecision(decision: "allow" | "deny") {
    if (resolved()) return;
    setLoading(true);
    clearInterval(timer);
    try {
      await api.runs.approve(props.runId, props.callId, decision);
      setResolved(decision);
      props.onResolved?.(decision);
    } catch {
      setResolved("deny");
    } finally {
      setLoading(false);
    }
  }

  async function handleAllowAlways() {
    await handleDecision("allow");
    // NOTE: Implemented — POST /policies/allow-always (preset cloning, rule prepend, idempotent)
  }

  const progressPercent = () => (remaining() / timeout) * 100;

  const toolIcon = () => {
    switch (props.tool) {
      case "bash": case "exec": case "shell": return "\u25B8";
      case "read": case "read_file": return "\u25A1";
      case "edit": case "edit_file": case "write": case "write_file": return "\u25A1";
      case "search": case "glob": case "grep": return "\u25C7";
      default: return "\u25CB";
    }
  };

  return (
    <div class={`rounded-cf-md border-2 p-4 my-2 ${
      resolved() === "allow" ? "border-green-500 bg-green-500/5" :
      resolved() === "deny" ? "border-red-500 bg-red-500/5" :
      "border-amber-500 bg-amber-500/5"
    }`}>
      <div class="flex items-center gap-2 mb-3">
        <span class="text-amber-500 font-bold text-lg">{"\u26A0"}</span>
        <span class="font-semibold text-cf-text-primary text-sm">Permission Request</span>
      </div>

      <div class="space-y-1 mb-3 text-sm">
        <div class="flex gap-2">
          <span class="text-cf-text-muted w-20">Tool:</span>
          <span class="font-mono text-cf-text-primary">{toolIcon()} {props.tool}</span>
        </div>
        <Show when={props.command}>
          <div class="flex gap-2">
            <span class="text-cf-text-muted w-20">Command:</span>
            <span class="font-mono text-cf-text-primary break-all">{props.command}</span>
          </div>
        </Show>
        <Show when={props.path}>
          <div class="flex gap-2">
            <span class="text-cf-text-muted w-20">Path:</span>
            <span class="font-mono text-cf-text-primary break-all">{props.path}</span>
          </div>
        </Show>
      </div>

      <Show when={!resolved()}>
        <div class="mb-3">
          <div class="w-full bg-cf-bg-inset rounded-full h-1.5">
            <div
              class={`h-1.5 rounded-full transition-all duration-1000 ${
                remaining() > 30 ? "bg-amber-500" :
                remaining() > 10 ? "bg-orange-500" : "bg-red-500"
              }`}
              style={{ width: `${progressPercent()}%` }}
            />
          </div>
          <span class="text-xs text-cf-text-muted mt-1">{remaining()}s remaining</span>
        </div>

        <div class="flex gap-2">
          <button
            class="px-3 py-1.5 rounded-cf-sm bg-green-600 text-white text-sm font-medium hover:bg-green-700 disabled:opacity-50"
            onClick={() => handleDecision("allow")}
            disabled={loading()}
          >
            Allow
          </button>
          <button
            class="px-3 py-1.5 rounded-cf-sm bg-cf-bg-surface border border-cf-border text-cf-text-primary text-sm font-medium hover:bg-cf-bg-inset disabled:opacity-50"
            onClick={handleAllowAlways}
            disabled={loading()}
          >
            Allow Always
          </button>
          <button
            class="px-3 py-1.5 rounded-cf-sm bg-red-600 text-white text-sm font-medium hover:bg-red-700 disabled:opacity-50"
            onClick={() => handleDecision("deny")}
            disabled={loading()}
          >
            Deny
          </button>
        </div>
      </Show>

      <Show when={resolved()}>
        <div class={`text-sm font-medium ${
          resolved() === "allow" ? "text-green-500" : "text-red-500"
        }`}>
          {resolved() === "allow" ? "\u2713 Allowed" : "\u2717 Denied"}
        </div>
      </Show>
    </div>
  );
}
```

**Step 2: Verify frontend compiles**

Run: `cd /workspaces/CodeForge/frontend && npx tsc --noEmit`

**Step 3: Commit**

```bash
git add frontend/src/features/project/PermissionRequestCard.tsx
git commit -m "feat(ui): add PermissionRequestCard component"
```

---

### Task 1.5: Frontend — Add `api.runs.approve()` API Method

**Files:**
- Modify: `frontend/src/api/client.ts:224+`

**Step 1: Add the method**

Add to the `api` object under a new `runs` namespace (or existing if present):

```typescript
runs: {
  approve: (runId: string, callId: string, decision: "allow" | "deny") =>
    request<{ status: string; run_id: string; call_id: string; decision: string }>(
      `${BASE}/runs/${runId}/approve/${callId}`,
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ decision }),
      },
    ),
},
```

**Step 2: Verify frontend compiles**

Run: `cd /workspaces/CodeForge/frontend && npx tsc --noEmit`

**Step 3: Commit**

```bash
git add frontend/src/api/client.ts
git commit -m "feat(api): add runs.approve() method for HITL decisions"
```

---

### Task 1.6: Frontend — Wire PermissionRequestCard into ChatPanel

**Files:**
- Modify: `frontend/src/features/project/ChatPanel.tsx:222-348,764`

**Step 1: Add permission request state and event handler**

Add signal after existing signals:

```typescript
const [permissionRequests, setPermissionRequests] = createSignal<AGUIPermissionRequest[]>([]);
```

Add event subscription after line 332 (after `cleanupGoalProposal`):

```typescript
const cleanupPermissionRequest = onAGUIEvent("agui.permission_request", (payload) => {
  setPermissionRequests((prev) => [...prev, payload]);
});
```

Add to cleanup block (line 339-348):

```typescript
cleanupPermissionRequest();
```

Clear on `run_finished`:

```typescript
// Inside the run_finished handler (line ~291), add:
setPermissionRequests([]);
```

**Step 2: Render PermissionRequestCards in the chat area**

Add after the tool call cards / goal proposal cards rendering, before the chat input:

```typescript
<For each={permissionRequests().filter((pr) => !resolvedPermissions().has(pr.call_id))}>
  {(pr) => (
    <PermissionRequestCard
      runId={pr.run_id}
      callId={pr.call_id}
      tool={pr.tool}
      command={pr.command}
      path={pr.path}
      onResolved={() => {
        setResolvedPermissions((prev) => new Set([...prev, pr.call_id]));
      }}
    />
  )}
</For>
```

Add a `resolvedPermissions` signal:

```typescript
const [resolvedPermissions, setResolvedPermissions] = createSignal<Set<string>>(new Set());
```

**Step 3: Verify frontend compiles and renders**

Run: `cd /workspaces/CodeForge/frontend && npx tsc --noEmit`

**Step 4: Commit**

```bash
git add frontend/src/features/project/ChatPanel.tsx
git commit -m "feat(ui): wire PermissionRequestCard into ChatPanel"
```

---

## Phase 2: Inline Diff View + Accept/Reject

Dependencies: Phase 1 (for Revert endpoint pattern)

### Task 2.1: Python — Edit/Write Tools Return Diff Data

**Files:**
- Modify: `workers/codeforge/tools/edit_file.py:56-83`
- Modify: `workers/codeforge/tools/write_file.py` (similar pattern)
- Test: `workers/codeforge/tools/test_edit_file.py`

**Step 1: Write the failing test**

```python
import pytest
from workers.codeforge.tools.edit_file import EditFileTool

@pytest.mark.asyncio
async def test_edit_file_returns_diff(tmp_path):
    f = tmp_path / "test.go"
    f.write_text("func hello() bool {\n  return false\n}\n")

    tool = EditFileTool()
    result = await tool.execute(
        {"file_path": str(f), "old_text": "return false", "new_text": "return true"},
        workspace_path=str(tmp_path),
    )
    assert result.diff is not None
    assert result.diff["path"] == str(f)
    assert len(result.diff["hunks"]) == 1
    assert "return false" in result.diff["hunks"][0]["old_content"]
    assert "return true" in result.diff["hunks"][0]["new_content"]
```

**Step 2: Run test to verify it fails**

Run: `cd /workspaces/CodeForge && python -m pytest workers/codeforge/tools/test_edit_file.py::test_edit_file_returns_diff -v`
Expected: FAIL — `ToolResult` has no `diff` attribute

**Step 3: Modify `ToolResult` model and `EditFileTool`**

In `workers/codeforge/tools/__init__.py`, add `diff` field to `ToolResult`:

```python
@dataclass
class ToolResult:
    output: str
    error: str = ""
    diff: dict[str, Any] | None = None
```

In `workers/codeforge/tools/edit_file.py`, capture old content before edit, compute diff:

```python
async def execute(self, arguments: dict[str, Any], workspace_path: str) -> ToolResult:
    file_path = arguments["file_path"]
    old_text = arguments["old_text"]
    new_text = arguments["new_text"]

    content = Path(file_path).read_text(encoding="utf-8")
    old_content = content  # snapshot before edit

    if old_text not in content:
        return ToolResult(output="", error=f"old_text not found in {file_path}")

    new_content = content.replace(old_text, new_text, 1)
    Path(file_path).write_text(new_content, encoding="utf-8")

    # Build diff
    old_lines = old_text.count("\n") + 1
    new_lines = new_text.count("\n") + 1
    start = content[:content.index(old_text)].count("\n") + 1

    diff_data = {
        "path": file_path,
        "hunks": [{
            "old_start": start,
            "old_lines": old_lines,
            "new_start": start,
            "new_lines": new_lines,
            "old_content": old_text,
            "new_content": new_text,
        }],
    }

    rel = os.path.relpath(file_path, workspace_path) if workspace_path else file_path
    return ToolResult(
        output=f"replaced {old_lines} line(s) with {new_lines} line(s) in {rel}",
        diff=diff_data,
    )
```

Apply similar pattern to `write_file.py` (diff shows full file as new content if file didn't exist, or unified diff if overwriting).

**Step 4: Run test to verify it passes**

Run: `cd /workspaces/CodeForge && python -m pytest workers/codeforge/tools/test_edit_file.py -v`
Expected: PASS

**Step 5: Commit**

```bash
git add workers/codeforge/tools/
git commit -m "feat(tools): edit/write tools return structured diff data"
```

---

### Task 2.2: Python — Include Diff in AG-UI `tool_result` Event

**Files:**
- Modify: `workers/codeforge/runtime.py:261-292`
- Modify: `workers/codeforge/agent_loop.py:448-531`

**Step 1: Extend `report_tool_result` to accept diff**

Add `diff: dict | None = None` parameter to `report_tool_result()` in `runtime.py`:

```python
async def report_tool_result(
    self,
    call_id: str,
    tool: str,
    success: bool,
    output: str = "",
    error: str = "",
    cost_usd: float = 0.0,
    tokens_in: int = 0,
    tokens_out: int = 0,
    model: str = "",
    diff: dict[str, Any] | None = None,
) -> None:
```

Include `"diff": diff` in the NATS payload dict.

**Step 2: Pass diff from agent loop tool execution**

In `agent_loop.py`, after executing a tool that returns a `ToolResult` with diff:

```python
await self._runtime.report_tool_result(
    call_id=tc.call_id,
    tool=tc.name,
    success=True,
    output=result.output,
    diff=result.diff,
)
```

**Step 3: Go Core — Forward diff in AG-UI WebSocket event**

In `internal/adapter/ws/agui_events.go`, add `Diff` field to `AGUIToolResultEvent`:

```go
type AGUIToolResultEvent struct {
    CallID  string          `json:"call_id"`
    Result  string          `json:"result"`
    Error   string          `json:"error,omitempty"`
    CostUSD float64         `json:"cost_usd,omitempty"`
    Diff    json.RawMessage `json:"diff,omitempty"`
}
```

In the handler that forwards NATS tool results to WebSocket, pass through the diff field.

**Step 4: Commit**

```bash
git add workers/codeforge/runtime.py workers/codeforge/agent_loop.py internal/adapter/ws/agui_events.go
git commit -m "feat(agui): include diff data in tool_result events"
```

---

### Task 2.3: Go Core — Checkpoint Service + Revert Endpoint

**Files:**
- Create: `internal/service/checkpoint.go`
- Modify: `internal/adapter/http/handlers_conversation.go:121-150`
- Test: `internal/service/checkpoint_test.go`

**Step 1: Write checkpoint service tests**

```go
func TestCheckpointService_StoreAndRevert(t *testing.T) {
    svc := NewCheckpointService()

    runID := "run-123"
    callID := "call-456"
    path := "/tmp/test-checkpoint.txt"

    // Write original file
    os.WriteFile(path, []byte("original"), 0644)

    // Store checkpoint
    err := svc.Store(runID, callID, path)
    require.NoError(t, err)

    // Modify file
    os.WriteFile(path, []byte("modified"), 0644)

    // Revert
    err = svc.Revert(runID, callID)
    require.NoError(t, err)

    // Verify restored
    content, _ := os.ReadFile(path)
    assert.Equal(t, "original", string(content))

    // Cleanup
    os.Remove(path)
}
```

**Step 2: Implement checkpoint service**

```go
package service

type CheckpointService struct {
    mu          sync.RWMutex
    checkpoints map[string]map[string]checkpoint // runID -> callID -> checkpoint
}

type checkpoint struct {
    Path    string
    Content []byte
}

func NewCheckpointService() *CheckpointService { ... }
func (s *CheckpointService) Store(runID, callID, path string) error { ... }
func (s *CheckpointService) Revert(runID, callID string) error { ... }
func (s *CheckpointService) ClearRun(runID string) { ... }
```

**Step 3: Add revert HTTP handler**

Add to `handlers_conversation.go`:

```go
// RevertToolCall reverts a file edit to its pre-change state.
// POST /api/v1/runs/{id}/revert/{callId}
func (h *Handlers) RevertToolCall(w http.ResponseWriter, r *http.Request) {
    runID := chi.URLParam(r, "id")
    callID := chi.URLParam(r, "callId")
    err := h.Checkpoint.Revert(runID, callID)
    if err != nil {
        writeError(w, http.StatusNotFound, err.Error())
        return
    }
    writeJSON(w, http.StatusOK, map[string]string{
        "status": "reverted",
        "run_id": runID,
        "call_id": callID,
    })
}
```

Register route in router.

**Step 4: Run tests**

Run: `cd /workspaces/CodeForge && go test ./internal/service/ -run TestCheckpoint -v`

**Step 5: Commit**

```bash
git add internal/service/checkpoint.go internal/service/checkpoint_test.go internal/adapter/http/handlers_conversation.go
git commit -m "feat(checkpoint): add file checkpoint service and revert endpoint"
```

---

### Task 2.4: Frontend — `DiffView` Component (Unified)

**Files:**
- Create: `frontend/src/features/project/DiffView.tsx`

**Step 1: Create the component**

Renders unified diff with red/green line coloring. Accepts `hunks` array from tool_result diff data. Includes syntax highlighting via `<pre>` with Tailwind classes.

Component props: `{ path: string; hunks: DiffHunk[] }` where `DiffHunk = { old_start, old_lines, new_start, new_lines, old_content, new_content }`.

Split `old_content` and `new_content` by newlines. Render old lines with red background (`bg-red-500/10 text-red-400`) prefixed with `-`, new lines with green (`bg-green-500/10 text-green-400`) prefixed with `+`.

**Step 2: Verify frontend compiles**

Run: `cd /workspaces/CodeForge/frontend && npx tsc --noEmit`

**Step 3: Commit**

```bash
git add frontend/src/features/project/DiffView.tsx
git commit -m "feat(ui): add DiffView component for unified diff rendering"
```

---

### Task 2.5: Frontend — `DiffModal` Component (Side-by-Side)

**Files:**
- Create: `frontend/src/features/project/DiffModal.tsx`

**Step 1: Create the modal component**

Modal overlay with two columns (CSS grid `grid-cols-2`). Left = old content, right = new content. Line numbers on both sides. Changed lines highlighted. Close button + `[Accept]` `[Reject]` buttons in footer.

**Step 2: Commit**

```bash
git add frontend/src/features/project/DiffModal.tsx
git commit -m "feat(ui): add DiffModal component for side-by-side diff view"
```

---

### Task 2.6: Frontend — Integrate Diff into ToolCallCard

**Files:**
- Modify: `frontend/src/features/project/ToolCallCard.tsx:152-195`
- Modify: `frontend/src/api/websocket.ts` (add diff to AGUIToolResult)

**Step 1: Extend AGUIToolResult interface**

```typescript
export interface AGUIToolResult {
  call_id: string;
  result: string;
  error?: string;
  cost_usd?: number;
  diff?: {
    path: string;
    hunks: Array<{
      old_start: number;
      old_lines: number;
      new_start: number;
      new_lines: number;
      old_content: string;
      new_content: string;
    }>;
  };
}
```

**Step 2: Modify ToolCallCard to show DiffView when diff data exists**

Replace the plain-text result section with conditional DiffView rendering:

```typescript
<Show when={props.diff} fallback={/* existing pre result rendering */}>
  <DiffView path={props.diff!.path} hunks={props.diff!.hunks} />
  <div class="flex gap-2 mt-2">
    <button class="..." onClick={handleAccept}>Accept</button>
    <button class="..." onClick={handleReject}>Reject</button>
    <button class="..." onClick={() => setShowSideBySide(true)}>Side-by-Side</button>
  </div>
</Show>
```

Accept calls nothing (file already written). Reject calls `api.runs.revert(runId, callId)`.

**Step 3: Verify and commit**

```bash
git add frontend/src/features/project/ToolCallCard.tsx frontend/src/api/websocket.ts
git commit -m "feat(ui): integrate inline diff view with accept/reject into ToolCallCard"
```

---

## Phase 3: Action Buttons (Hybrid)

Dependencies: Phase 2 (uses ToolCallCard integration pattern)

### Task 3.1: Go Core + Python — `agui.action_suggestion` Event

**Files:**
- Modify: `internal/adapter/ws/agui_events.go`
- Modify: `frontend/src/api/websocket.ts`
- Modify: `workers/codeforge/agent_loop.py`

Add event constant `AGUIActionSuggestion = "agui.action_suggestion"` and struct. Add to frontend event types. Python worker emits after successful run completion.

**Commit:** `feat(agui): add action_suggestion event type`

---

### Task 3.2: Frontend — `ActionBar` Component + Action Rules

**Files:**
- Create: `frontend/src/features/project/ActionBar.tsx`
- Create: `frontend/src/features/project/actionRules.ts`

`actionRules.ts` maps tool name + result pattern -> button definitions.
`ActionBar.tsx` renders buttons from both rules and agent suggestions.

**Commit:** `feat(ui): add ActionBar with tool-type rules and agent suggestions`

---

### Task 3.3: Frontend — Wire ActionBar into ChatPanel

**Files:**
- Modify: `frontend/src/features/project/ChatPanel.tsx`

Add `agui.action_suggestion` event handler. Render ActionBar below messages and after run completion.

**Commit:** `feat(ui): wire ActionBar into ChatPanel`

---

## Phase 4: Model-Badge + Cost + Context Gauge

Dependencies: None (can run parallel to Phase 3)

### Task 4.1: Go + Python — Add `usage` to `run_finished` Event

**Files:**
- Modify: `internal/adapter/ws/agui_events.go:26-30`
- Modify: `workers/codeforge/agent_loop.py`
- Modify: `workers/codeforge/runtime.py`

Add `Usage` struct to `AGUIRunFinishedEvent`. Python worker accumulates token counts across the run and includes in the finish payload.

**Commit:** `feat(agui): add usage data to run_finished event`

---

### Task 4.2: Frontend — `MessageBadge` + `CostBreakdown` Components

**Files:**
- Create: `frontend/src/features/project/MessageBadge.tsx`
- Create: `frontend/src/features/project/CostBreakdown.tsx`
- Create: `frontend/src/utils/providerMap.ts`

`providerMap.ts`: simple map from model prefix to provider name.
`MessageBadge`: renders `model . provider . $cost . latency`.
`CostBreakdown`: expandable detail on click.

**Commit:** `feat(ui): add MessageBadge and CostBreakdown components`

---

### Task 4.3: Frontend — `SessionFooter` + `ContextGauge`

**Files:**
- Create: `frontend/src/features/project/SessionFooter.tsx`
- Create: `frontend/src/features/project/ContextGauge.tsx`

SessionFooter: permanent bar below chat input showing model, steps, total cost, context gauge.
ContextGauge: colored progress bar (green/yellow/orange/red thresholds).

**Commit:** `feat(ui): add SessionFooter with context gauge and cost tracking`

---

### Task 4.4: Frontend — Wire MessageBadge + SessionFooter into ChatPanel

**Files:**
- Modify: `frontend/src/features/project/ChatPanel.tsx`

Add MessageBadge to each assistant message rendering. Add SessionFooter below chat input. Track cumulative cost and token usage in signals.

**Commit:** `feat(ui): integrate cost tracking and model badges into ChatPanel`

---

## Phase 5: `@`/`#`/`/` References with Fuzzy Search

Dependencies: None (standalone feature)

### Task 5.1: Frontend — Fuzzy Search Utility

**Files:**
- Create: `frontend/src/features/chat/fuzzySearch.ts`
- Test: `frontend/src/features/chat/fuzzySearch.test.ts`

Pure function: `fuzzyMatch(query: string, items: Item[], frequencyMap: Map<string, number>): Item[]`. Implements prefix > substring > Levenshtein fallback, sorted by frequency.

**Commit:** `feat(chat): add fuzzy search utility with frequency ranking`

---

### Task 5.2: Frontend — Frequency Tracker Hook

**Files:**
- Create: `frontend/src/hooks/useFrequencyTracker.ts`

Custom hook using `localStorage`. Methods: `track(key: string)`, `getFrequency(key: string): number`, `getAll(): Map<string, number>`.

**Commit:** `feat(chat): add useFrequencyTracker hook for localStorage frequency tracking`

---

### Task 5.3: Frontend — `AutocompletePopover` Component

**Files:**
- Create: `frontend/src/features/chat/AutocompletePopover.tsx`

Generic popover: triggers on `@`, `#`, `/`. Shows grouped results with category headers. Keyboard navigation (arrow, enter, esc, tab). Uses fuzzy search + frequency ranking.

**Commit:** `feat(chat): add AutocompletePopover component`

---

### Task 5.4: Frontend — `TokenBadge` Component

**Files:**
- Create: `frontend/src/features/chat/TokenBadge.tsx`

Colored chip for inserted `@`/`#` references. Props: `type: "@" | "#"`, `label: string`. Backspace-deletable. Colors: blue for `@`, purple for `#`.

**Commit:** `feat(chat): add TokenBadge component for inserted references`

---

### Task 5.5: Go Core — Dynamic Command Registry Endpoint

**Files:**
- Create: `internal/domain/command/command.go`
- Create: `internal/service/commands.go`
- Create: `internal/adapter/http/handlers_commands.go`
- Test: `internal/service/commands_test.go`

Domain model: `Command{ID, Label, Category, Icon, Description, Args}`.
Service: aggregates from modes, models, skills, MCP tools, custom YAML.
Handler: `GET /api/v1/commands` returns all commands.

**Commit:** `feat(commands): add dynamic command registry with aggregated endpoint`

---

### Task 5.6: Frontend — `ChatInput` with Trigger Detection

**Files:**
- Create: `frontend/src/features/chat/ChatInput.tsx`
- Create: `frontend/src/features/chat/commandStore.ts`

Replace existing `<textarea>` in ChatPanel with new `ChatInput` component. Detects `@`, `#`, `/` triggers. Opens AutocompletePopover. Renders TokenBadges for selected references. `commandStore.ts` fetches commands from API and caches.

**Commit:** `feat(chat): add ChatInput with @/#// trigger detection and autocomplete`

---

### Task 5.7: Frontend — Wire ChatInput into ChatPanel

**Files:**
- Modify: `frontend/src/features/project/ChatPanel.tsx:764-772`

Replace the existing `<textarea>` with `<ChatInput>`. Pass selected references as metadata with the message.

**Commit:** `feat(ui): replace chat textarea with enhanced ChatInput`

---

## Phase 6: Chat Commands (`/compact`, `/rewind`, etc.)

Dependencies: Phase 5 (uses command registry and ChatInput)

### Task 6.1: Frontend — Command Executor

**Files:**
- Create: `frontend/src/features/chat/commandExecutor.ts`

Dispatch function: routes command ID to handler (frontend-only, api-call, or message-shortcut). Implements `/cost`, `/help`, `/diff` (frontend-only). Calls API for `/compact`, `/rewind`, `/clear`, `/mode`, `/model`.

**Commit:** `feat(chat): add command executor for slash command dispatch`

---

### Task 6.2: Go Core — Compact, Rewind, Mode, Model Endpoints

**Files:**
- Modify: `internal/adapter/http/handlers_conversation.go`
- Modify: `internal/service/conversation.go`

New endpoints:
- `POST /api/v1/conversations/{id}/compact`
- `POST /api/v1/conversations/{id}/rewind` with `{step_id, mode}`
- `POST /api/v1/conversations/{id}/clear`
- `POST /api/v1/conversations/{id}/mode` with `{mode}`
- `POST /api/v1/conversations/{id}/model` with `{model}`

**Commit:** `feat(api): add conversation control endpoints (compact, rewind, mode, model)`

---

### Task 6.3: Python Worker — Compact Handler

**Files:**
- Modify: `workers/codeforge/consumer/_conversation.py`

Subscribe to `conversation.compact.request`. Takes message history, sends to LLM with summarization prompt. Publishes `conversation.compact.complete` with summarized history.

**Commit:** `feat(worker): add conversation compact handler with LLM summarization`

---

### Task 6.4: Frontend — Rewind Timeline Component

**Files:**
- Create: `frontend/src/features/chat/RewindTimeline.tsx`

Overlay component showing all agent steps as timeline entries. Each entry: timestamp, tool calls, summary. Click -> confirmation dialog (Code + Conversation / Code only / Conversation only). Calls rewind API on confirm.

**Commit:** `feat(ui): add RewindTimeline overlay component`

---

### Task 6.5: Frontend — Diff Summary Modal

**Files:**
- Create: `frontend/src/features/chat/DiffSummaryModal.tsx`

Collects all edit/write tool results from the session. Shows aggregated unified diff per file. Used by `/diff` command.

**Commit:** `feat(ui): add DiffSummaryModal for aggregated session changes`

---

## Phase 7: Chat History Search + Agent Tool

Dependencies: None (standalone)

### Task 7.1: PostgreSQL — FTS Index Migration

**Files:**
- Create: `migrations/0XX_add_conversation_fts_index.sql`

```sql
CREATE INDEX idx_conversation_messages_fts
ON conversation_messages USING GIN(to_tsvector('english', content));
```

**Commit:** `feat(db): add full-text search index on conversation messages`

---

### Task 7.2: Go Core — Conversation Search in Search Handler

**Files:**
- Modify: `internal/adapter/http/handlers_search.go:23-53`
- Create: `internal/adapter/store/store_search_conversations.go`
- Test: `internal/adapter/store/store_search_conversations_test.go`

Add `"conversations"` scope to `GlobalSearch`. New store method with PostgreSQL FTS query using `ts_rank`, `to_tsvector`, `plainto_tsquery`. Supports filters: project_id, role, date range.

**Commit:** `feat(search): add conversation full-text search with facet filters`

---

### Task 7.3: Frontend — Conversations Tab in SearchPage

**Files:**
- Create: `frontend/src/features/search/ConversationResults.tsx`
- Modify: `frontend/src/features/search/SearchPage.tsx:59-94`

New tab "Conversations" alongside existing tabs. Filter dropdowns for project, model, role, date range. Each result: timestamp, project badge, model, snippet with highlighted match, "Open" link.

**Commit:** `feat(ui): add Conversations tab to SearchPage with filters`

---

### Task 7.4: Python — `search_conversations` Agent Tool

**Files:**
- Create: `workers/codeforge/tools/search_conversations.py`
- Modify: `workers/codeforge/tools/__init__.py:112-136`

New tool: calls `POST /api/v1/search` with scope `conversations`. Returns top 5 matching conversation snippets. Register in `build_default_registry()`.

**Commit:** `feat(tools): add search_conversations agent tool`

---

## Phase 8: Browser Notifications + Notification Center

Dependencies: Phase 1 (uses permission_request events)

### Task 8.1: Frontend — Notification Store

**Files:**
- Create: `frontend/src/features/notifications/notificationStore.ts`
- Create: `frontend/src/features/notifications/notificationSettings.ts`

SolidJS store for notifications (max 50). Settings in localStorage (enable push, enable sound, sound type, notify-on filters). Push via `Notification` API when `document.hidden`. Sound via `Audio` element.

**Commit:** `feat(notifications): add notification store with push, sound, and settings`

---

### Task 8.2: Frontend — Tab Badge Utility

**Files:**
- Create: `frontend/src/utils/tabBadge.ts`

Updates document.title to `(N) CodeForge`. Swaps favicon to badge variant. Resets on window focus.

**Commit:** `feat(ui): add tab badge utility for unread notification count`

---

### Task 8.3: Frontend — Notification Center Components

**Files:**
- Create: `frontend/src/features/notifications/NotificationBell.tsx`
- Create: `frontend/src/features/notifications/NotificationCenter.tsx`
- Create: `frontend/src/features/notifications/NotificationItem.tsx`

Bell icon with badge counter. Dropdown panel with All/Unread/Archive tabs. Each item is actionable (permission requests have inline Approve/Deny). Mark All Read button.

**Commit:** `feat(ui): add NotificationCenter with bell icon and actionable items`

---

### Task 8.4: Frontend — Wire Notifications into App Layout

**Files:**
- Modify: `frontend/src/App.tsx:116-150`

Add `NotificationBell` to the navigation bar. Subscribe to AG-UI events in a top-level effect to feed the notification store.

**Commit:** `feat(ui): integrate notification bell into app navigation`

---

## Phase 9: Channels (Project + Bot)

Dependencies: Phase 8 (notification integration), Phase 5 (@ mentions)

### Task 9.1: PostgreSQL — Channels Schema Migration

**Files:**
- Create: `migrations/0XX_create_channels.sql`

Three tables: `channels`, `channel_messages`, `channel_members` with indexes. See design doc for full schema.

**Commit:** `feat(db): add channels schema (channels, messages, members)`

---

### Task 9.2: Go Core — Channel Domain Model

**Files:**
- Create: `internal/domain/channel/channel.go`

Domain types: `Channel`, `ChannelMessage`, `ChannelMember`, `ChannelType` (project/bot), `SenderType` (user/agent/bot/webhook), `NotifySetting` (all/mentions/nothing).

**Commit:** `feat(domain): add channel domain model`

---

### Task 9.3: Go Core — Channel Store (PostgreSQL)

**Files:**
- Create: `internal/adapter/store/store_channel.go`
- Test: `internal/adapter/store/store_channel_test.go`

CRUD operations: CreateChannel, GetChannel, ListChannels, DeleteChannel, CreateMessage, ListMessages (cursor-paginated), CreateThreadReply, AddMember, UpdateMemberSettings, CreateWebhookKey, ValidateWebhookKey.

All queries include `AND tenant_id = $N`.

**Commit:** `feat(store): add channel PostgreSQL store with tenant isolation`

---

### Task 9.4: Go Core — Channel Service

**Files:**
- Create: `internal/service/channel.go`
- Test: `internal/service/channel_test.go`

Business logic: auto-create project channel on project creation, webhook key generation (random 32-byte hex), agent post-run summary to project channel, message validation.

**Commit:** `feat(service): add channel service with auto-creation and agent integration`

---

### Task 9.5: Go Core — Channel HTTP Handlers

**Files:**
- Create: `internal/adapter/http/handlers_channel.go`

REST endpoints as defined in design doc. Register routes in router. Webhook endpoint validates `X-Webhook-Key` header.

**Commit:** `feat(api): add channel REST endpoints with webhook support`

---

### Task 9.6: Go Core — Channel WebSocket Events

**Files:**
- Create: `internal/adapter/ws/channel_events.go`

Event types: `channel.message`, `channel.typing`, `channel.read`. Hub subscribes users to channels they're members of. Typing indicator debounce on server side.

**Commit:** `feat(ws): add real-time channel events (message, typing, read)`

---

### Task 9.7: Frontend — Channel List + Sidebar Integration

**Files:**
- Create: `frontend/src/features/channels/ChannelList.tsx`
- Modify: `frontend/src/App.tsx:116-150`

Sidebar section "Channels" with list of project and bot channels. Unread badge per channel. Click navigates to channel view.

**Commit:** `feat(ui): add ChannelList sidebar with unread badges`

---

### Task 9.8: Frontend — Channel View + Message + Input

**Files:**
- Create: `frontend/src/features/channels/ChannelView.tsx`
- Create: `frontend/src/features/channels/ChannelMessage.tsx`
- Create: `frontend/src/features/channels/ChannelInput.tsx`

ChannelView: message list with infinite scroll (cursor pagination), real-time updates via WebSocket.
ChannelMessage: sender name + avatar/icon + timestamp + content + thread reply count.
ChannelInput: reuses ChatInput component with `@` mention support.

**Commit:** `feat(ui): add ChannelView with real-time messages and input`

---

### Task 9.9: Frontend — Thread Panel

**Files:**
- Create: `frontend/src/features/channels/ThreadPanel.tsx`

Slide-over panel from right. Shows parent message + thread replies. Input for reply. Closes on Esc or outside click.

**Commit:** `feat(ui): add ThreadPanel slide-over for channel message threads`

---

## Phase 10: Voice & Video (Stub Only)

Dependencies: None

### Task 10.1: Documentation — Future Feature Spec

**Files:**
- Create: `docs/features/05-chat-enhancements.md`

Document all 10 implemented features + Voice/Video future scope. Cross-reference in `docs/todo.md`.

**Commit:** `docs: add chat enhancements feature spec with voice/video future scope`

---

## Phase 11: Documentation + Cleanup

### Task 11.1: Update `docs/todo.md`

Mark all completed chat enhancement tasks. Add new items discovered during implementation.

### Task 11.2: Update `docs/project-status.md`

Add chat enhancements as completed phase.

### Task 11.3: Update `CLAUDE.md`

Add chat enhancements to the Architecture section (Action Buttons, Channels, Notification Center, etc.).

**Commit:** `docs: update todo, project-status, and CLAUDE.md for chat enhancements`

---

## Execution Order + Dependencies

```
Phase 1 (HITL)  ─────────────────────┐
Phase 2 (Diff)  ──── depends on 1 ───┤
Phase 3 (Actions) ── depends on 2 ───┤
Phase 4 (Cost) ── parallel to 2+3 ───┤
Phase 5 (Refs) ── parallel to all ───┤──> Phase 11 (Docs)
Phase 6 (Commands) ── depends on 5 ──┤
Phase 7 (Search) ── parallel to all ─┤
Phase 8 (Notifs) ── depends on 1 ────┤
Phase 9 (Channels) ── depends on 5+8─┤
Phase 10 (V&V stub) ── parallel ─────┘
```

**Parallelizable groups:**
- Group A: Phase 1 → 2 → 3 (sequential)
- Group B: Phase 4 + 5 → 6 (Phase 5 then 6)
- Group C: Phase 7 (independent)
- Group D: Phase 8 → 9 (sequential)
- All groups can run in parallel.
