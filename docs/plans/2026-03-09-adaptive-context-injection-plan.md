# Adaptive Context Injection Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Enable agents to automatically receive dense codebase context (RepoMap, Retrieval, GraphRAG, LSP, Goals) with an adaptive token budget that shrinks as conversation history grows — zero config, no special cases.

**Architecture:** Three changes to the existing system: (1) flip `ContextEnabled` default to `true`, (2) add adaptive budget calculation in Go Core based on `len(history)` so early turns get full context and later turns get minimal/none, (3) auto-trigger all index builds (RepoMap + Retrieval + GraphRAG) after project clone/adopt/setup. The existing priority-based packing in `assembleAndPack()` handles the rest — no new special cases needed.

**Tech Stack:** Go (config, service layer, HTTP handlers), Python (no changes needed — budget arrives via NATS payload)

---

## Task 1: Adaptive Budget Calculation Function

**Files:**
- Create: `internal/service/context_budget.go`
- Create: `internal/service/context_budget_test.go`

**Step 1: Write the failing tests**

```go
// internal/service/context_budget_test.go
package service

import (
	"testing"

	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

func TestAdaptiveContextBudget(t *testing.T) {
	baseBudget := 2048

	tests := []struct {
		name         string
		historyCount int
		wantMin      int
		wantMax      int
	}{
		{
			name:         "first message gets full budget",
			historyCount: 0,
			wantMin:      baseBudget,
			wantMax:      baseBudget,
		},
		{
			name:         "single prior exchange still gets full budget",
			historyCount: 2, // 1 user + 1 assistant
			wantMin:      baseBudget - 1, // allow rounding
			wantMax:      baseBudget,
		},
		{
			name:         "moderate history reduces budget",
			historyCount: 20, // ~10 exchanges
			wantMin:      512,
			wantMax:      baseBudget - 1,
		},
		{
			name:         "long history gets minimal budget",
			historyCount: 40, // ~20 exchanges
			wantMin:      256,
			wantMax:      768,
		},
		{
			name:         "very long history gets zero",
			historyCount: 80,
			wantMin:      0,
			wantMax:      256,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			history := make([]messagequeue.ConversationMessagePayload, tc.historyCount)
			for i := range history {
				if i%2 == 0 {
					history[i] = messagequeue.ConversationMessagePayload{Role: "user", Content: "hello"}
				} else {
					history[i] = messagequeue.ConversationMessagePayload{Role: "assistant", Content: "hi there"}
				}
			}
			got := AdaptiveContextBudget(baseBudget, history)
			if got < tc.wantMin || got > tc.wantMax {
				t.Errorf("AdaptiveContextBudget(%d, %d msgs) = %d, want [%d, %d]",
					baseBudget, tc.historyCount, got, tc.wantMin, tc.wantMax)
			}
		})
	}
}

func TestAdaptiveContextBudget_NeverNegative(t *testing.T) {
	got := AdaptiveContextBudget(100, make([]messagequeue.ConversationMessagePayload, 1000))
	if got < 0 {
		t.Errorf("budget must never be negative, got %d", got)
	}
}

func TestAdaptiveContextBudget_ZeroBase(t *testing.T) {
	got := AdaptiveContextBudget(0, nil)
	if got != 0 {
		t.Errorf("zero base budget should return 0, got %d", got)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `cd /workspaces/CodeForge && go test ./internal/service/ -run TestAdaptiveContextBudget -v -count=1`
Expected: FAIL — `AdaptiveContextBudget` not defined

**Step 3: Implement the adaptive budget function**

The formula uses a linear decay based on message count. Key insight: each message in history means the agent has already gathered context through tool calls and conversation. After ~60 messages the agent has enough self-gathered context that pre-injection adds no value.

```go
// internal/service/context_budget.go
package service

import "github.com/Strob0t/CodeForge/internal/port/messagequeue"

// contextDecayThreshold is the number of history messages at which
// the adaptive budget reaches zero. ~30 exchanges (user+assistant pairs).
const contextDecayThreshold = 60

// AdaptiveContextBudget calculates the context injection token budget
// based on conversation history length. Early turns get the full budget;
// as history grows the budget shrinks linearly to zero.
//
// Rationale: on turn 1 the agent knows nothing about the codebase and
// benefits most from pre-injected context (RepoMap, Retrieval, etc.).
// By turn 15+ the agent has read files and built its own context through
// tool calls, so injecting more wastes tokens.
func AdaptiveContextBudget(baseBudget int, history []messagequeue.ConversationMessagePayload) int {
	if baseBudget <= 0 {
		return 0
	}
	n := len(history)
	if n >= contextDecayThreshold {
		return 0
	}
	// Linear decay: budget * (1 - n/threshold)
	scaled := baseBudget * (contextDecayThreshold - n) / contextDecayThreshold
	if scaled < 0 {
		return 0
	}
	return scaled
}
```

**Step 4: Run tests to verify they pass**

Run: `cd /workspaces/CodeForge && go test ./internal/service/ -run TestAdaptiveContextBudget -v -count=1`
Expected: PASS (all cases)

**Step 5: Commit**

```
feat: add AdaptiveContextBudget for history-aware context injection
```

---

## Task 2: Wire Adaptive Budget into Conversation Service

**Files:**
- Modify: `internal/service/conversation_agent.go:77-109` (buildConversationContextEntries)
- Modify: `internal/service/conversation_agent.go:137-141` (SendMessageAgentic — pass history)
- Modify: `internal/service/conversation_test.go:185-237` (update existing test)

**Step 1: Write the failing test**

Add a new test that verifies the budget adapts to history length. The existing test `TestSendMessageAgentic_ContextInjection` (line 174) uses `ContextEnabled: true` with an empty history — it should still pass. The new test verifies that a long history reduces the context entries.

```go
// Add to internal/service/conversation_test.go

func TestSendMessageAgentic_AdaptiveBudgetReducesContext(t *testing.T) {
	dir := t.TempDir()
	// Create enough files to fill a full budget.
	for i := 0; i < 20; i++ {
		name := fmt.Sprintf("file%d.go", i)
		content := fmt.Sprintf("package main\n\nfunc Handler%d() {}\n", i)
		_ = os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644)
	}

	store := &convMockStore{}
	store.projects = []project.Project{
		{ID: "proj-1", Name: "test", WorkspacePath: dir},
	}
	q := &captureQueue{}
	bc := &mockBroadcaster{}
	modes := service.NewModeService()
	svc := service.NewConversationService(store, bc, "gpt-4o", modes)
	svc.SetQueue(q)

	agentCfg := &config.Agent{
		MaxLoopIterations:    10,
		MaxContextTokens:     120000,
		ContextEnabled:       true,
		ContextBudget:        2048,
		ContextPromptReserve: 512,
	}
	svc.SetAgentConfig(agentCfg)

	orchCfg := &config.Orchestrator{DefaultContextBudget: 8192, PromptReserve: 1024}
	ctxOpt := service.NewContextOptimizerService(store, orchCfg, &config.Limits{
		MaxFiles: 50, MaxFileSize: 32768, SearchTimeout: 5 * time.Second,
	})
	svc.SetContextOptimizer(ctxOpt)

	ctx := context.Background()
	conv, err := svc.Create(ctx, conversation.CreateRequest{ProjectID: "proj-1", Title: "Adaptive Test"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Seed conversation with many messages to simulate long history.
	for i := 0; i < 40; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		store.messages[conv.ID] = append(store.messages[conv.ID], conversation.Message{
			ConversationID: conv.ID,
			Role:           role,
			Content:        fmt.Sprintf("Message %d with some content about handlers and authentication", i),
		})
	}

	err = svc.SendMessageAgentic(ctx, conv.ID, conversation.SendMessageRequest{Content: "Continue working"})
	if err != nil {
		t.Fatalf("SendMessageAgentic: %v", err)
	}

	_, data := q.snapshot()
	var payload messagequeue.ConversationRunStartPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// With 40+ history messages the adaptive budget should be very small,
	// resulting in fewer (or zero) context entries compared to an empty history.
	if len(payload.Context) > 3 {
		t.Errorf("expected reduced context with long history, got %d entries", len(payload.Context))
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /workspaces/CodeForge && go test ./internal/service/ -run TestSendMessageAgentic_AdaptiveBudget -v -count=1`
Expected: FAIL — budget is still static (full 2048), many context entries returned

**Step 3: Modify buildConversationContextEntries to accept history**

Update `internal/service/conversation_agent.go`:

Change the signature of `buildConversationContextEntries` to accept history, and use `AdaptiveContextBudget`:

```go
// buildConversationContextEntries assembles context entries for a conversation run
// when ContextEnabled is true and the context optimizer is wired.
// The budget adapts to conversation length: early turns get full context,
// later turns get progressively less as the agent builds its own context.
func (s *ConversationService) buildConversationContextEntries(
	ctx context.Context,
	projectID, userMessage, conversationID string,
	history []messagequeue.ConversationMessagePayload,
) []messagequeue.ContextEntryPayload {
	if s.contextOpt == nil || s.agentCfg == nil || !s.agentCfg.ContextEnabled {
		return nil
	}

	budget := AdaptiveContextBudget(s.agentCfg.ContextBudget, history)
	if budget <= 0 {
		slog.Debug("adaptive context budget is zero, skipping context injection",
			"conversation_id", conversationID,
			"history_messages", len(history),
		)
		return nil
	}

	entries, err := s.contextOpt.BuildConversationContext(ctx, projectID, userMessage, "",
		ConversationContextOpts{
			Budget:        budget,
			PromptReserve: s.agentCfg.ContextPromptReserve,
		})
	if err != nil {
		slog.Warn("conversation context build failed",
			"conversation_id", conversationID,
			"project_id", projectID,
			"error", err,
		)
		return nil
	}
	if len(entries) == 0 {
		return nil
	}

	slog.Info("conversation context entries built",
		"conversation_id", conversationID,
		"entries", len(entries),
		"budget", budget,
		"history_messages", len(history),
	)
	return toContextEntryPayloads(entries)
}
```

**Step 4: Update the call site in SendMessageAgentic**

In `internal/service/conversation_agent.go`, the call at line ~241:

Change from:
```go
contextEntries := s.buildConversationContextEntries(ctx, proj.ID, req.Content, conversationID)
```

To:
```go
contextEntries := s.buildConversationContextEntries(ctx, proj.ID, req.Content, conversationID, protoMessages)
```

Note: `protoMessages` is already computed at line 166 (`s.historyToPayload(history)`) and is the correct type (`[]messagequeue.ConversationMessagePayload`).

**Step 5: Update the call site in SendMessageAgenticWithMode (if it exists)**

Search for any other callers of `buildConversationContextEntries` and update them to pass the history parameter.

Run: `grep -n "buildConversationContextEntries" internal/service/conversation_agent.go`

Update all call sites to pass the `protoMessages` argument.

**Step 6: Run all conversation tests**

Run: `cd /workspaces/CodeForge && go test ./internal/service/ -run TestSendMessage -v -count=1`
Expected: ALL PASS (including existing `TestSendMessageAgentic_ContextInjection` and new adaptive test)

**Step 7: Commit**

```
feat: wire adaptive budget into conversation context injection
```

---

## Task 3: Change ContextEnabled Default to true

**Files:**
- Modify: `internal/config/config.go:550` (default value)
- Modify: `internal/service/conversation_test.go:324-328` (update ContextEmptyWhenDisabled test)

**Step 1: Verify the existing "disabled" test**

The test `TestSendMessageAgentic_ContextEmptyWhenDisabled` (line 310) explicitly sets `ContextEnabled: false`. This test should still pass after changing the default, since it explicitly overrides the default. Verify this.

Run: `cd /workspaces/CodeForge && go test ./internal/service/ -run TestSendMessageAgentic_ContextEmptyWhenDisabled -v -count=1`
Expected: PASS

**Step 2: Change the default**

In `internal/config/config.go:550`, change:
```go
ContextEnabled:       false,
```
to:
```go
ContextEnabled:       true,
```

Also update the comment on line 148:
```go
ContextEnabled       bool     `yaml:"context_enabled"`        // Enable context optimizer for conversations (default: true)
```

**Step 3: Run the full config and conversation test suites**

Run: `cd /workspaces/CodeForge && go test ./internal/config/ ./internal/service/ -v -count=1 2>&1 | tail -30`
Expected: ALL PASS

Note: The `ContextEmptyWhenDisabled` test explicitly sets `ContextEnabled: false`, so it should still pass. If any test relied on the default being `false` without explicitly setting it, it may fail — fix by explicitly setting the field in those tests.

**Step 4: Commit**

```
feat: enable context injection by default (ContextEnabled=true)
```

---

## Task 4: Auto-Index After Clone

**Files:**
- Modify: `internal/adapter/http/handlers.go:271-278` (CloneProject)

**Step 1: Write a test that verifies all three indexes are triggered**

This is best tested as an integration-level assertion. Since the existing auto-trigger for RepoMap is already untested (fire-and-forget goroutine), we add a helper method on Handlers that can be tested.

Create a focused test in the existing handlers test file:

```go
// Verify the auto-index helper triggers all three services
func TestAutoIndexProject_TriggersAllIndexes(t *testing.T) {
	repoMapCalled := false
	retrievalCalled := false
	graphCalled := false

	// Use mock services that track calls
	// (adapt to existing test infrastructure in handlers_test.go)
	h := &Handlers{
		RepoMap:   &mockRepoMap{onGenerate: func() { repoMapCalled = true }},
		Retrieval: &mockRetrieval{onIndex: func() { retrievalCalled = true }},
		Graph:     &mockGraph{onBuild: func() { graphCalled = true }},
	}

	h.autoIndexProject(context.Background(), "proj-1", "/tmp/workspace")

	if !repoMapCalled {
		t.Error("expected RepoMap.RequestGeneration to be called")
	}
	if !retrievalCalled {
		t.Error("expected Retrieval.RequestIndex to be called")
	}
	if !graphCalled {
		t.Error("expected Graph.RequestBuild to be called")
	}
}
```

Note: Adapt this test to match the existing test mock patterns in `handlers_test.go` or `handlers_*_test.go`. The mock services need to implement the required interface methods.

**Step 2: Extract an autoIndexProject helper method**

In `internal/adapter/http/handlers.go`, add a method that replaces the inline goroutine:

```go
// autoIndexProject triggers background indexing for all context sources.
// Called after clone, adopt, or setup to ensure agents get full context.
// Each index build is independent — failures are logged but don't block.
func (h *Handlers) autoIndexProject(ctx context.Context, projectID, workspacePath string) {
	bg := context.Background()

	if h.RepoMap != nil {
		go func() {
			if err := h.RepoMap.RequestGeneration(bg, projectID, nil); err != nil {
				slog.Error("auto repomap generation failed", "project_id", projectID, "error", err)
			}
		}()
	}

	if h.Retrieval != nil {
		go func() {
			if err := h.Retrieval.RequestIndex(bg, projectID, workspacePath, ""); err != nil {
				slog.Error("auto retrieval index failed", "project_id", projectID, "error", err)
			}
		}()
	}

	if h.Graph != nil {
		go func() {
			if err := h.Graph.RequestBuild(bg, projectID, workspacePath); err != nil {
				slog.Error("auto graph build failed", "project_id", projectID, "error", err)
			}
		}()
	}
}
```

**Step 3: Update CloneProject to use autoIndexProject**

Replace lines 271-278 in `handlers.go`:

```go
// Auto-trigger all index builds after successful clone.
h.autoIndexProject(r.Context(), id, p.WorkspacePath)
```

Note: `p` is the cloned project returned by `h.Projects.Clone()`, so `p.WorkspacePath` is the correct path.

**Step 4: Run existing handler tests**

Run: `cd /workspaces/CodeForge && go test ./internal/adapter/http/ -v -count=1 2>&1 | tail -20`
Expected: PASS

**Step 5: Commit**

```
feat: auto-trigger all indexes (RepoMap, Retrieval, GraphRAG) after clone
```

---

## Task 5: Auto-Index After Adopt and Setup

**Files:**
- Modify: `internal/adapter/http/handlers.go:283-314` (AdoptProject)
- Modify: `internal/adapter/http/handlers.go:330-348` (SetupProject)

**Step 1: Add autoIndexProject call to AdoptProject**

After the successful `h.Projects.Adopt()` call (line 307-311), add:

```go
p, err := h.Projects.Adopt(r.Context(), id, cleanPath)
if err != nil {
    writeDomainError(w, err, "adopt failed")
    return
}

// Auto-trigger all index builds after successful adopt.
h.autoIndexProject(r.Context(), id, p.WorkspacePath)

writeJSON(w, http.StatusOK, p)
```

**Step 2: Add autoIndexProject call to SetupProject**

After the successful `h.Projects.SetupProject()` call, add:

```go
result, err := h.Projects.SetupProject(r.Context(), id, tenantID, body.Branch)
if err != nil {
    writeDomainError(w, err, "setup failed")
    return
}

// Auto-trigger all index builds after successful setup.
if result != nil && result.Project.WorkspacePath != "" {
    h.autoIndexProject(r.Context(), id, result.Project.WorkspacePath)
}

writeJSON(w, http.StatusOK, result)
```

Check the `SetupResult` type to confirm the field name:

Run: `grep -A5 "type SetupResult" internal/domain/project/`

Adapt the field access based on the actual struct shape.

**Step 3: Run handler tests**

Run: `cd /workspaces/CodeForge && go test ./internal/adapter/http/ -v -count=1 2>&1 | tail -20`
Expected: PASS

**Step 4: Commit**

```
feat: auto-trigger indexes after adopt and setup
```

---

## Task 6: Update Documentation

**Files:**
- Modify: `CLAUDE.md` (ContextEnabled default, adaptive budget)
- Modify: `docs/features/04-agent-orchestration.md` (context injection docs)
- Modify: `docs/todo.md` (mark task done)

**Step 1: Update CLAUDE.md**

In the agent config section (line ~105), change:
```
Agent.ContextEnabled` (false)
```
to:
```
Agent.ContextEnabled` (true)
```

**Step 2: Update feature doc**

In `docs/features/04-agent-orchestration.md`, update the context injection section (~lines 375-398) to document:
- `context_enabled: true` is now the default
- Budget adapts to conversation length (linear decay over 60 messages)
- All indexes auto-trigger after clone/adopt/setup
- The priority system handles source selection within the budget

**Step 3: Update todo.md**

Mark the adaptive context injection task as complete with the date.

**Step 4: Commit**

```
docs: update context injection defaults and adaptive budget docs
```

---

## Task 7: Full Verification

**Step 1: Run the complete Go test suite**

Run: `cd /workspaces/CodeForge && go test ./internal/... -count=1 2>&1 | tail -20`
Expected: ALL PASS

**Step 2: Run pre-commit hooks**

Run: `cd /workspaces/CodeForge && pre-commit run --all-files`
Expected: PASS

**Step 3: Verify the config change is respected by env var override**

Run: `CODEFORGE_AGENT_CONTEXT_ENABLED=false go test ./internal/config/ -run TestLoad -v -count=1`
Expected: Config loader still respects env var override to disable context

---

## Summary of Changes

| File | Change |
|---|---|
| `internal/service/context_budget.go` | NEW: `AdaptiveContextBudget()` function |
| `internal/service/context_budget_test.go` | NEW: Tests for adaptive budget |
| `internal/service/conversation_agent.go` | MODIFY: Use adaptive budget, accept history param |
| `internal/service/conversation_test.go` | MODIFY: Add adaptive budget test |
| `internal/config/config.go:550` | MODIFY: `ContextEnabled: true` |
| `internal/adapter/http/handlers.go` | MODIFY: Extract `autoIndexProject()`, call from Clone/Adopt/Setup |
| `CLAUDE.md` | MODIFY: Update ContextEnabled default |
| `docs/features/04-agent-orchestration.md` | MODIFY: Document adaptive budget |
| `docs/todo.md` | MODIFY: Mark task done |

**No Python changes needed** — the budget arrives via the existing NATS payload, and the existing `ConversationHistoryManager.build_messages()` + `_build_system_content()` already handles whatever entries Go sends.
