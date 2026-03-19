# TaskForge Evaluation Bugfixes — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix 5 bugs discovered during the CodeForge TaskForge evaluation test so the full project lifecycle (create → goals → roadmap → auto-execute → deliver) works end-to-end.

**Architecture:** All fixes are isolated, atomic changes in existing files. No new tables, no new services, no new NATS subjects. Each task is independently deployable and testable.

**Tech Stack:** Go 1.25 (backend), TypeScript/SolidJS (frontend), PostgreSQL (DB)

**Test patterns:** All Go tests use `package service_test`, `extRuntimeMockStore` / `runtimeMockStore` from `runtime_coverage_test.go`, and the public `HandleToolCallRequest` API. Decisions are verified via NATS queue responses (`queue.lastMessage(messagequeue.SubjectRunToolCallResponse)`).

---

## File Map

| Task | File | Action | Responsibility |
|------|------|--------|----------------|
| 1 | `internal/service/runtime.go` | Modify (lines 684-709) | Auto-persist goals on `agent.goal_proposed` event |
| 1 | `internal/service/runtime.go` | Modify (struct, line 31) | Add `goalSvc` field to RuntimeService |
| 1 | `internal/service/runtime.go` | Modify (after line 89) | Add `SetGoalService` setter |
| 1 | `cmd/codeforge/main.go` or wiring file | Modify | Wire goalSvc into RuntimeService |
| 1 | `internal/service/runtime_coverage_test.go` | Modify | Test auto-persist via goal store mock |
| 2 | `internal/service/runtime_execution.go` | Modify (lines 194-204) | Read `config["policy_preset"]` as fallback |
| 2 | `internal/service/runtime_coverage_test.go` | Modify | Test config fallback |
| 3 | `internal/service/runtime_execution.go` | Modify (lines 96-105, 231-240) | Auto-allow when profile mode is AcceptEdits/Delegate |
| 3 | `internal/service/runtime_coverage_test.go` | Modify | Test HITL bypass |
| 4 | `internal/service/project.go` | Modify (lines 484-491) | Persist `detected_languages` to project config |
| 4 | `internal/service/project_test.go` | Modify | Test stack persistence |
| 5 | `frontend/src/i18n/en.ts` | Modify (line 1607) | Fix misleading text |
| 5 | `frontend/src/i18n/locales/de.ts` | Modify (line 1626) | Fix German translation |
| 5 | `frontend/src/features/project/FilePanel.tsx` | Modify (lines 723-735) | Conditional text based on workspace existence |

---

## Task 1: Auto-Persist Goals on `agent.goal_proposed` Event

**Problem:** `propose_goal` tool emits an AG-UI WebSocket event but never writes to the DB. Goals only persist when a user clicks "Approve" in the frontend. In headless/full-auto mode, no user is present → goals are lost.

**Fix:** When `RuntimeService` receives `agent.goal_proposed` via NATS trajectory event, it calls `GoalDiscoveryService.Create()` to persist the goal immediately. The WebSocket broadcast stays (UI can still show the goal card).

**Files:**
- Modify: `internal/service/runtime.go:31` (add field)
- Modify: `internal/service/runtime.go:89` (add setter)
- Modify: `internal/service/runtime.go:684-709` (add persistence)
- Modify: wiring file (inject dependency)
- Test: `internal/service/runtime_coverage_test.go`

- [ ] **Step 1: Add goalSvc field and setter to RuntimeService**

```go
// internal/service/runtime.go — add to struct (after line 55, before closing brace)
goalSvc *GoalDiscoveryService

// internal/service/runtime.go — add setter (after SetOnRunComplete at line 89)
// SetGoalService sets the goal discovery service for auto-persisting agent-proposed goals.
func (s *RuntimeService) SetGoalService(svc *GoalDiscoveryService) {
    s.goalSvc = svc
}
```

- [ ] **Step 2: Add auto-persist logic in trajectory event handler**

Replace the block at `internal/service/runtime.go:684-709`:

```go
// Goal proposal events get a dedicated AG-UI broadcast + auto-persist.
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
        // Broadcast to frontend (UI can display/edit the goal).
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

        // Auto-persist: the agent recognized the goal — save it immediately.
        if s.goalSvc != nil && proposal.Data.Action == "create" {
            req := &goal.CreateRequest{
                Kind:     goal.GoalKind(proposal.Data.Kind),
                Title:    proposal.Data.Title,
                Content:  proposal.Data.Content,
                Priority: proposal.Data.Priority,
                Source:   "agent",
            }
            if _, createErr := s.goalSvc.Create(msgCtx, payload.ProjectID, req); createErr != nil {
                slog.Warn("auto-persist goal failed",
                    "project_id", payload.ProjectID,
                    "title", proposal.Data.Title,
                    "error", createErr,
                )
            } else {
                slog.Info("goal auto-persisted",
                    "project_id", payload.ProjectID,
                    "title", proposal.Data.Title,
                    "kind", proposal.Data.Kind,
                )
            }
        }
    }
}
```

Add `"github.com/Strob0t/CodeForge/internal/domain/goal"` to the import block.

- [ ] **Step 3: Wire GoalDiscoveryService into RuntimeService**

Search for `NewRuntimeService(` in `cmd/codeforge/main.go`. The `goalSvc` (GoalDiscoveryService) is created around line 612. Add the setter call near the other `SetXxx` calls:

```go
runtimeSvc.SetGoalService(goalDiscoverySvc)
```

- [ ] **Step 4: Write the test**

Add to `internal/service/runtime_coverage_test.go`. The test verifies that after the fix, creating a goal-proposing conversation triggers persistence. Since the trajectory handler is internal to the NATS subscriber, test at the integration boundary by extending `extRuntimeMockStore` to track `CreateProjectGoal` calls:

```go
// Add to extRuntimeMockStore:
goalCreated []goalPkg.ProjectGoal // add import alias: goalPkg "github.com/Strob0t/CodeForge/internal/domain/goal"

func (m *extRuntimeMockStore) CreateProjectGoal(_ context.Context, g *goalPkg.ProjectGoal) error {
    m.mu2.Lock()
    defer m.mu2.Unlock()
    g.ID = fmt.Sprintf("goal-%d", len(m.goalCreated)+1)
    m.goalCreated = append(m.goalCreated, *g)
    return nil
}
```

Since the trajectory handler is inside an anonymous NATS subscriber, write a focused unit test by extracting the persist logic into a testable exported method:

```go
// Add to runtime.go:
// PersistGoalProposal is called when an agent proposes a goal. It creates the goal in the DB.
func (s *RuntimeService) PersistGoalProposal(ctx context.Context, projectID, kind, title, content string, priority int) error {
    if s.goalSvc == nil {
        return nil
    }
    req := &goal.CreateRequest{
        Kind:     goal.GoalKind(kind),
        Title:    title,
        Content:  content,
        Priority: priority,
        Source:   "agent",
    }
    _, err := s.goalSvc.Create(ctx, projectID, req)
    return err
}
```

Then the trajectory handler calls `s.PersistGoalProposal(...)` instead of inlining the logic.

Test:

```go
func TestPersistGoalProposal(t *testing.T) {
    store := &extRuntimeMockStore{
        runtimeMockStore: runtimeMockStore{
            projects: []project.Project{
                {ID: "proj-1", Name: "test", WorkspacePath: "/tmp/test"},
            },
        },
    }
    queue := &runtimeMockQueue{}
    bc := &runtimeMockBroadcaster{}
    es := &runtimeMockEventStore{}
    policySvc := service.NewPolicyService("headless-safe-sandbox", nil)
    svc := service.NewRuntimeService(store, queue, bc, es, policySvc, &config.Runtime{})

    goalSvc := service.NewGoalDiscoveryService(store)
    svc.SetGoalService(goalSvc)

    ctx := context.Background()
    err := svc.PersistGoalProposal(ctx, "proj-1", "requirement", "Go REST API", "CRUD endpoints", 90)
    if err != nil {
        t.Fatalf("PersistGoalProposal failed: %v", err)
    }

    store.mu2.Lock()
    defer store.mu2.Unlock()
    if len(store.goalCreated) != 1 {
        t.Fatalf("expected 1 goal created, got %d", len(store.goalCreated))
    }
    if store.goalCreated[0].Title != "Go REST API" {
        t.Fatalf("expected title 'Go REST API', got %q", store.goalCreated[0].Title)
    }
    if store.goalCreated[0].Source != "agent" {
        t.Fatalf("expected source 'agent', got %q", store.goalCreated[0].Source)
    }
}
```

- [ ] **Step 5: Run test**

Run: `cd /workspaces/CodeForge && go test ./internal/service/ -run TestPersistGoalProposal -v`
Expected: PASS

- [ ] **Step 6: Run full service tests**

Run: `cd /workspaces/CodeForge && go test ./internal/service/ -count=1 -timeout 120s`
Expected: all existing tests still pass

- [ ] **Step 7: Commit**

```bash
git add internal/service/runtime.go internal/service/runtime_coverage_test.go cmd/codeforge/main.go
git commit -m "fix(goals): auto-persist agent-proposed goals to DB on trajectory event"
```

---

## Task 2: Map `config.policy_preset` to PolicyProfile for Conversations

**Problem:** The API stores `autonomy_level` / `policy_preset` in `project.Config` (generic JSON map), but `handleConversationToolCall()` reads from `project.PolicyProfile` (a dedicated DB column). Setting config values via the API has no effect on policy evaluation.

**Fix:** In `handleConversationToolCall()`, after checking `proj.PolicyProfile`, fall back to `proj.Config["policy_preset"]` if the dedicated field is empty.

**Files:**
- Modify: `internal/service/runtime_execution.go:194-204`
- Test: `internal/service/runtime_coverage_test.go`

- [ ] **Step 1: Write the failing test**

Following the pattern from `TestHandleConversationToolCall_DenyByPolicy` (line 499 of `runtime_coverage_test.go`):

```go
// Add to runtime_coverage_test.go:
func TestConversationToolCall_ConfigPolicyPresetFallback(t *testing.T) {
    store := &extRuntimeMockStore{
        runtimeMockStore: runtimeMockStore{
            projects: []project.Project{
                {
                    ID:            "proj-config",
                    Name:          "config-test",
                    WorkspacePath: "/tmp/cfg",
                    PolicyProfile: "", // empty — should fall back to config
                    Config:        map[string]string{"policy_preset": "trusted-mount-autonomous"},
                },
            },
            agents: []agent.Agent{
                {ID: "agent-1", ProjectID: "proj-config", Name: "a", Backend: "aider", Status: agent.StatusIdle, Config: map[string]string{}},
            },
            tasks: []task.Task{
                {ID: "task-1", ProjectID: "proj-config", Title: "t", Prompt: "p", Status: task.StatusPending},
            },
        },
        conversations: []conversation.Conversation{
            {ID: "conv-config", ProjectID: "proj-config", Title: "Config test"},
        },
    }
    queue := &runtimeMockQueue{}
    bc := &runtimeMockBroadcaster{}
    es := &runtimeMockEventStore{}
    // Default profile is supervised (ask for everything) — but config says trusted-mount (allow all)
    policySvc := service.NewPolicyService("supervised-ask-all", nil)
    svc := service.NewRuntimeService(store, queue, bc, es, policySvc, &config.Runtime{})

    ctx := context.Background()
    req := messagequeue.ToolCallRequestPayload{
        RunID:  "conv-config", // conversation ID, not a run → falls into handleConversationToolCall
        CallID: "call-cfg-1",
        Tool:   "Write",
        Path:   "/workspace/main.go",
    }
    if err := svc.HandleToolCallRequest(ctx, &req); err != nil {
        t.Fatalf("HandleToolCallRequest: %v", err)
    }

    // Verify: decision should be "allow" (from trusted-mount-autonomous, ModeAcceptEdits)
    // NOT "ask" or timeout-deny (from supervised-ask-all default)
    msg, ok := queue.lastMessage(messagequeue.SubjectRunToolCallResponse)
    if !ok {
        t.Fatal("expected tool call response on NATS")
    }
    var resp messagequeue.ToolCallResponsePayload
    _ = json.Unmarshal(msg.Data, &resp)
    if resp.Decision != "allow" {
        t.Fatalf("expected 'allow' from config fallback, got %q", resp.Decision)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /workspaces/CodeForge && go test ./internal/service/ -run TestConversationToolCall_ConfigPolicyPresetFallback -v -timeout 120s`
Expected: FAIL — decision is "deny" (HITL timeout on supervised-ask-all, ignoring config)

- [ ] **Step 3: Implement the config fallback**

In `internal/service/runtime_execution.go`, replace lines 194-204:

```go
// Resolve policy profile from the conversation's project.
policyProfile := ""
proj, projErr := s.store.GetProject(ctx, conv.ProjectID)
if projErr == nil {
    policyProfile = proj.PolicyProfile
    // Fall back to config["policy_preset"] if dedicated field is empty.
    if policyProfile == "" {
        if preset, ok := proj.Config["policy_preset"]; ok && preset != "" {
            policyProfile = preset
        }
    }
}

// If no policy profile is set, use the service default.
if policyProfile == "" {
    policyProfile = s.policy.DefaultProfile()
}
```

`DefaultProfile()` already exists on `PolicyService` (line 112 of `policy.go`). No changes needed there.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /workspaces/CodeForge && go test ./internal/service/ -run TestConversationToolCall_ConfigPolicyPresetFallback -v -timeout 120s`
Expected: PASS

- [ ] **Step 5: Run full service tests**

Run: `cd /workspaces/CodeForge && go test ./internal/service/ -count=1 -timeout 120s`
Expected: all tests pass

- [ ] **Step 6: Commit**

```bash
git add internal/service/runtime_execution.go internal/service/runtime_coverage_test.go
git commit -m "fix(policy): read config.policy_preset as fallback for conversation tool calls"
```

---

## Task 3: Auto-Allow DecisionAsk in Full-Auto Profiles (Skip HITL)

**Problem:** When tool call policy evaluates to `DecisionAsk`, it calls `waitForApproval()` which blocks 60s for a user response. In full-auto mode there is no user → always times out → deny → run dies.

**Fix:** Before calling `waitForApproval()`, check the resolved profile's mode. If mode is `ModeAcceptEdits` or `ModeDelegate`, auto-resolve to `DecisionAllow` instead of waiting.

**Files:**
- Modify: `internal/service/runtime_execution.go:96-105` (run-based) and `231-240` (conversation-based)
- Test: `internal/service/runtime_coverage_test.go`

**Important:** Apply Task 2 first since it changes lines 194-204, which shifts subsequent line numbers by ~1 line.

- [ ] **Step 1: Write the failing test**

```go
func TestConversationToolCall_HITLBypassForFullAutoProfile(t *testing.T) {
    store := &extRuntimeMockStore{
        runtimeMockStore: runtimeMockStore{
            projects: []project.Project{
                {
                    ID:            "proj-auto",
                    Name:          "auto-project",
                    WorkspacePath: "/tmp/auto",
                    // headless-safe-sandbox has ModeDefault → Bash "ask" by default
                    // But we set trusted-mount-autonomous which has ModeAcceptEdits
                    PolicyProfile: "trusted-mount-autonomous",
                },
            },
            agents: []agent.Agent{
                {ID: "agent-1", ProjectID: "proj-auto", Name: "a", Backend: "aider", Status: agent.StatusIdle, Config: map[string]string{}},
            },
            tasks: []task.Task{
                {ID: "task-1", ProjectID: "proj-auto", Title: "t", Prompt: "p", Status: task.StatusPending},
            },
        },
        conversations: []conversation.Conversation{
            {ID: "conv-auto", ProjectID: "proj-auto", Title: "Auto conv"},
        },
    }
    queue := &runtimeMockQueue{}
    bc := &runtimeMockBroadcaster{}
    es := &runtimeMockEventStore{}
    policySvc := service.NewPolicyService("supervised-ask-all", nil)
    runtimeCfg := config.Runtime{ApprovalTimeoutSeconds: 2} // short timeout to detect if HITL fires
    svc := service.NewRuntimeService(store, queue, bc, es, policySvc, &runtimeCfg)

    // Use a tight context — if HITL blocks, context will expire first
    ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
    defer cancel()

    req := messagequeue.ToolCallRequestPayload{
        RunID:  "conv-auto",
        CallID: "call-auto-1",
        Tool:   "Bash",
        Command: "go build ./...",
    }
    if err := svc.HandleToolCallRequest(ctx, &req); err != nil {
        t.Fatalf("HandleToolCallRequest: %v", err)
    }

    msg, ok := queue.lastMessage(messagequeue.SubjectRunToolCallResponse)
    if !ok {
        t.Fatal("expected tool call response")
    }
    var resp messagequeue.ToolCallResponsePayload
    _ = json.Unmarshal(msg.Data, &resp)
    if resp.Decision != "allow" {
        t.Fatalf("expected 'allow' (HITL bypass for full-auto), got %q", resp.Decision)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /workspaces/CodeForge && go test ./internal/service/ -run TestConversationToolCall_HITLBypassForFullAutoProfile -v -timeout 10s`
Expected: FAIL — blocks for 2s then returns "deny" (HITL timeout)

- [ ] **Step 3: Implement HITL bypass for conversation-based tool calls**

In `internal/service/runtime_execution.go`, replace the conversation HITL block (around lines 231-240 after Task 2 shifts):

```go
// HITL: when policy says "ask", check if the profile mode allows auto-approval.
if decision == policy.DecisionAsk {
    profile, profileOK := s.policy.GetProfile(policyProfile)
    if profileOK && (profile.Mode == policy.ModeAcceptEdits || profile.Mode == policy.ModeDelegate) {
        // Full-auto profile: auto-allow instead of waiting for HITL.
        decision = policy.DecisionAllow
        slog.Info("conversation HITL auto-approved (full-auto profile)",
            "conversation_id", req.RunID,
            "call_id", req.CallID,
            "tool", req.Tool,
            "profile", policyProfile,
        )
    } else {
        decision = s.waitForApproval(ctx, req.RunID, req.CallID, req.Tool, req.Command, req.Path)
        slog.Info("conversation HITL resolved",
            "conversation_id", req.RunID,
            "call_id", req.CallID,
            "tool", req.Tool,
            "decision", decision,
        )
    }
}
```

- [ ] **Step 4: Apply the same pattern to run-based tool calls**

In `internal/service/runtime_execution.go`, replace lines 96-105 (the run-based HITL block):

```go
// HITL: when policy says "ask", check if the profile mode allows auto-approval.
if decision == policy.DecisionAsk {
    if profile.Mode == policy.ModeAcceptEdits || profile.Mode == policy.ModeDelegate {
        decision = policy.DecisionAllow
        slog.Info("HITL auto-approved (full-auto profile)",
            "run_id", r.ID,
            "call_id", req.CallID,
            "tool", req.Tool,
            "profile", r.PolicyProfile,
        )
    } else {
        decision = s.waitForApproval(ctx, r.ID, req.CallID, req.Tool, req.Command, req.Path)
        slog.Info("HITL approval resolved",
            "run_id", r.ID,
            "call_id", req.CallID,
            "tool", req.Tool,
            "decision", decision,
        )
    }
}
```

Note: `profile` is already in scope from line 47 (`s.policy.GetProfile(r.PolicyProfile)`).

- [ ] **Step 5: Run test to verify it passes**

Run: `cd /workspaces/CodeForge && go test ./internal/service/ -run TestConversationToolCall_HITLBypassForFullAutoProfile -v -timeout 10s`
Expected: PASS (completes in <1s, not waiting for HITL)

- [ ] **Step 6: Run full service tests**

Run: `cd /workspaces/CodeForge && go test ./internal/service/ -count=1 -timeout 120s`
Expected: all tests pass

- [ ] **Step 7: Commit**

```bash
git add internal/service/runtime_execution.go internal/service/runtime_coverage_test.go
git commit -m "fix(policy): auto-allow DecisionAsk for full-auto profiles, skip HITL wait"
```

---

## Task 4: Persist Detected Languages to Project Config After Stack Detection

**Problem:** `SetupProject()` runs stack detection and returns the result, but never saves `detected_languages` to `project.Config`. The frontend checks `config?.detected_languages` for the onboarding pipeline step "Stack detected" → always `undefined` → always shows ○.

**Fix:** After successful stack detection in `SetupProject()`, marshal the detected languages and persist them in the project config.

**Files:**
- Modify: `internal/service/project.go:484-491`
- Test: `internal/service/project_test.go`

- [ ] **Step 1: Write the failing test**

Add a test that creates a workspace with a `go.mod` file, runs `SetupProject()`, and verifies `detected_languages` is persisted. Use the existing project test mock patterns:

```go
func TestSetupProject_PersistsDetectedLanguages(t *testing.T) {
    tmpDir := t.TempDir()
    // Create a go.mod so ScanWorkspace detects Go
    os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module test\ngo 1.21\n"), 0644)

    // Use the existing project test mock store (check project_test.go for pattern)
    // The mock must track UpdateProject calls to verify config persistence.
    // Key assertion: after SetupProject(), the project's Config["detected_languages"]
    // contains "go" (lowercase, as defined in stackmap.go manifestMap).
}
```

The exact mock structure depends on the existing patterns in `project_test.go`. The implementer should:
1. Read `internal/service/project_test.go` to find the mock store pattern
2. Extend it to track `UpdateProject` calls if not already tracked
3. Assert `Config["detected_languages"]` contains `"go"` (lowercase — `stackmap.go` maps `go.mod` → `"go"`)

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /workspaces/CodeForge && go test ./internal/service/ -run TestSetupProject_PersistsDetectedLanguages -v`
Expected: FAIL — `detected_languages` not in config

- [ ] **Step 3: Implement persistence after stack detection**

In `internal/service/project.go`, replace the success block at lines 484-491:

```go
} else {
    result.StackDetected = true
    result.Stack = stack
    result.Steps = append(result.Steps, project.SetupStep{
        Name:   "detect_stack",
        Status: "completed",
    })

    // Persist detected languages to project config for onboarding pipeline.
    if len(stack.Languages) > 0 {
        langJSON, marshalErr := json.Marshal(stack.Languages)
        if marshalErr == nil {
            if p.Config == nil {
                p.Config = make(map[string]string)
            }
            p.Config["detected_languages"] = string(langJSON)
            if updateErr := s.store.UpdateProject(ctx, p); updateErr != nil {
                slog.Warn("setup: failed to persist detected languages",
                    "project_id", id, "error", updateErr)
            }
        }
    }
}
```

`encoding/json` is already imported in `project.go`.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /workspaces/CodeForge && go test ./internal/service/ -run TestSetupProject_PersistsDetectedLanguages -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/service/project.go internal/service/project_test.go
git commit -m "fix(onboarding): persist detected_languages to project config after stack scan"
```

---

## Task 5: Fix Misleading "No workspace linked" Text in FilePanel

**Problem:** When a workspace IS linked but no file tab is open, the FilePanel shows "No workspace linked. Clone a repo or adopt a local directory." This is confusing — the workspace exists, just no file is selected. The same fallback text is shown whether a workspace exists or not.

**Fix:** Two changes: (a) Update i18n strings for the "has workspace, no file selected" case. (b) Add a conditional in `FilePanel.tsx` to show different text based on workspace existence.

**Files:**
- Modify: `frontend/src/i18n/en.ts:1607-1608`
- Modify: `frontend/src/i18n/locales/de.ts:1626-1628`
- Modify: `frontend/src/features/project/FilePanel.tsx:723-735`

- [ ] **Step 1: Add new i18n keys and update existing ones**

In `frontend/src/i18n/en.ts`, change lines 1607-1608 and add new keys:

```typescript
"empty.files": "No workspace linked. Clone a repo or adopt a local directory.",
"empty.files.action": "Setup Workspace",
"empty.files.select": "Select a file from the tree to view or edit.",
```

In `frontend/src/i18n/locales/de.ts`, add corresponding key:

```typescript
"empty.files.select": "Datei im Dateibaum auswaehlen zum Anzeigen oder Bearbeiten.",
```

- [ ] **Step 2: Update FilePanel.tsx to show conditional text**

In `frontend/src/features/project/FilePanel.tsx`, replace lines 723-735. The component needs to receive a `hasWorkspace` prop (or derive it). Replace the fallback:

```tsx
fallback={
  <div class="flex flex-col items-center justify-center gap-3 py-16 text-center">
    <p class="text-sm text-cf-text-muted">
      {props.hasWorkspace ? t("empty.files.select") : t("empty.files")}
    </p>
    <Show when={!props.hasWorkspace}>
      <button
        class="text-sm text-cf-accent hover:underline"
        onClick={() => props.onNavigate?.("setup")}
      >
        {t("empty.files.action")}
      </button>
    </Show>
  </div>
}
```

The `hasWorkspace` prop must be threaded from `ProjectDetailPage.tsx` where `p().workspace_path` is available. Check the existing FilePanel props interface and add `hasWorkspace?: boolean` if not present.

- [ ] **Step 3: Verify in browser**

1. Open CodeForge → navigate to a project WITH a workspace → no file selected
2. Verify: text says "Select a file from the tree to view or edit." (no "Setup Workspace" button)
3. Navigate to a project WITHOUT a workspace
4. Verify: text says "No workspace linked..." with "Setup Workspace" button

- [ ] **Step 4: Commit**

```bash
git add frontend/src/i18n/en.ts frontend/src/i18n/locales/de.ts frontend/src/features/project/FilePanel.tsx frontend/src/features/project/ProjectDetailPage.tsx
git commit -m "fix(ui): show context-appropriate text in FilePanel based on workspace state"
```

---

## Verification Checklist

After all 5 tasks are complete, run the full integration check:

- [ ] `cd /workspaces/CodeForge && go test ./internal/service/ -v -count=1 -timeout 120s` — all Go service tests pass
- [ ] `cd /workspaces/CodeForge && go build ./cmd/codeforge/` — binary compiles
- [ ] `cd /workspaces/CodeForge/frontend && npx tsc --noEmit` — TypeScript compiles
- [ ] Manual test: create project → send chat message → verify goals appear in Goals panel
- [ ] Manual test: set `config.policy_preset` = `"trusted-mount-autonomous"` → verify write_file is allowed
- [ ] Manual test: verify "Stack detected" shows ✓ after project setup
- [ ] Manual test: verify FilePanel shows context-appropriate text

---

## Task Dependency Graph

```
Task 1 (Goal Persist)  ──┐
Task 4 (Stack Detect)  ───┤── fully independent, can run in parallel
Task 5 (UI Text)       ───┘

Task 2 (Policy Config) ──→ Task 3 (HITL Bypass)
                            (apply sequentially — same file, Task 2 shifts line numbers)
```

Tasks 1, 4, 5 are fully independent of each other and of Tasks 2/3. Tasks 2 and 3 modify the same file (`runtime_execution.go`) — apply Task 2 first, then Task 3 (line numbers shift after Task 2).
