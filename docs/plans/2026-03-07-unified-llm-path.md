# Unified LLM Path — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Route ALL chat (simple + agentic) through Python Worker via NATS, eliminating the direct Go→LiteLLM call path. This ensures HybridRouter, Blocklist, Fallback, Cost Tracking, and Memory apply to every conversation — not just agentic ones.

**Architecture:** `SendMessage()` currently calls `s.llm.ChatCompletionStream()` directly. After this change, it dispatches via NATS like `SendMessageAgentic()` does, with `Agentic: false`. The Python worker's existing `_run_simple_chat()` handles it. The `llm` field is then removed from `ConversationService`.

**Tech Stack:** Go (service layer, NATS dispatch), existing Python handler, existing frontend (already done).

**Pre-implemented (verified in codebase):**
- Python `agentic: bool = True` field (`workers/codeforge/models.py:438`)
- Python `_run_simple_chat()` with per-chunk streaming (`_conversation.py:222-262`)
- Python `_execute_conversation_run()` branches on `run_msg.agentic` (`_conversation.py:191-220`)
- HTTP handler always returns 202 (`handlers_conversation.go:98-102`)
- `ConversationRunProvider.tsx` exists and wired into `App.tsx:304-306`
- Sidebar pulsing run indicator (`App.tsx:216-225`)
- `ChatPanel.tsx` uses `useConversationRuns()` and only handles 202

---

### Task 1: Add `Agentic bool` to Go NATS Schema

**Files:**
- Modify: `internal/port/messagequeue/schemas.go:429-447`

**Step 1: Add field to ConversationRunStartPayload**

In `schemas.go`, add `Agentic bool` to `ConversationRunStartPayload` after the `RoutingEnabled` field (line 446):

```go
RoutingEnabled    bool                         `json:"routing_enabled,omitempty"`    // Intelligent routing enabled (Phase 29)
Agentic           bool                         `json:"agentic"`                     // true = multi-turn tool loop, false = single-turn chat
```

**Step 2: Verify Go compiles**

Run: `go build ./...`
Expected: Success (new field is additive, zero-value `false` is backward compatible)

---

### Task 2: Write failing test for SendMessage NATS dispatch (RED)

**Files:**
- Modify: `internal/service/conversation_test.go`

**Step 1: Write test that SendMessage dispatches via NATS with Agentic=false**

Add after existing `TestConversation_SendMessage_EmptyContent` (around line 130):

```go
func TestSendMessage_DispatchesViaNATS(t *testing.T) {
	store := &convMockStore{}
	store.projects = []project.Project{
		{ID: "proj-1", Name: "Test", WorkspacePath: "/tmp/test"},
	}
	bc := &mockBroadcaster{}
	modes := service.NewModeService()
	svc := service.NewConversationService(store, bc, "gpt-4o", modes)
	q := &mockQueue{}
	svc.SetQueue(q)
	svc.SetAgentConfig(&config.Agent{DefaultModel: "gpt-4o"})
	ctx := context.Background()

	// Create conversation
	conv, err := svc.Create(ctx, conversation.CreateRequest{
		ProjectID: "proj-1",
		Title:     "NATS Dispatch Test",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// SendMessage should now dispatch via NATS, not call LLM directly
	_, err = svc.SendMessage(ctx, conv.ID, conversation.SendMessageRequest{Content: "Hello"})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	// Verify NATS message was published
	subject, data := q.snapshot()
	if subject != messagequeue.SubjectConversationRunStart {
		t.Fatalf("expected subject %s, got %s", messagequeue.SubjectConversationRunStart, subject)
	}
	if len(data) == 0 {
		t.Fatal("expected NATS payload")
	}

	// Verify Agentic=false in payload
	var payload messagequeue.ConversationRunStartPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.Agentic {
		t.Error("expected Agentic=false for simple chat dispatch")
	}
	if payload.ConversationID != conv.ID {
		t.Errorf("expected conversation_id %s, got %s", conv.ID, payload.ConversationID)
	}
	if payload.Model == "" {
		t.Error("expected non-empty model")
	}
	if payload.Termination.MaxSteps != 1 {
		t.Errorf("expected MaxSteps=1 for simple chat, got %d", payload.Termination.MaxSteps)
	}
}

func TestSendMessage_RequiresQueue(t *testing.T) {
	store := &convMockStore{}
	bc := &mockBroadcaster{}
	modes := service.NewModeService()
	svc := service.NewConversationService(store, bc, "gpt-4o", modes)
	// No queue set — should fail
	ctx := context.Background()

	conv, _ := svc.Create(ctx, conversation.CreateRequest{ProjectID: "proj-1", Title: "No Queue"})
	_, err := svc.SendMessage(ctx, conv.ID, conversation.SendMessageRequest{Content: "Hello"})
	if err == nil {
		t.Fatal("expected error when queue is nil")
	}
}
```

**Step 2: Run tests to verify RED**

Run: `go test ./internal/service/... -v -run "TestSendMessage_Dispatches|TestSendMessage_Requires" -count=1`
Expected: FAIL — `SendMessage` still calls LLM directly, constructor signature changed, etc.

---

### Task 3: Rewrite SendMessage to dispatch via NATS (GREEN)

**Files:**
- Modify: `internal/service/conversation.go:69-113` (ConversationService struct + constructor)
- Modify: `internal/service/conversation.go:197-332` (SendMessage method)

**Step 1: Remove `llm` from ConversationService struct and constructor**

In `conversation.go`, change the struct (line 69-94):

```go
// ConversationService manages conversations and LLM interactions.
type ConversationService struct {
	db            database.Store
	hub           broadcast.Broadcaster
	queue         messagequeue.Queue
	model         string // default model name for LiteLLM
	modelRegistry *ModelRegistry
	modeSvc       *ModeService
	mcpSvc        *MCPService
	policySvc     *PolicyService
	microagentSvc *MicroagentService
	goalSvc       *GoalDiscoveryService
	agentCfg      *config.Agent
	routingCfg    *config.Routing
	metrics       *cfotel.Metrics
	contextOpt    *ContextOptimizerService

	// processedRuns guards HandleConversationRunComplete against duplicate delivery.
	processedRuns   map[string]struct{}
	processedRunsMu sync.Mutex

	// completionWaiters allows in-process consumers (e.g. autoagent) to wait for
	// a conversation run to finish without creating a second NATS subscription.
	completionWaiters   map[string]chan CompletionResult
	completionWaitersMu sync.Mutex
}
```

Change the constructor (line 96-113):

```go
// NewConversationService creates a new ConversationService.
func NewConversationService(
	db database.Store,
	hub broadcast.Broadcaster,
	defaultModel string,
	modeSvc *ModeService,
) *ConversationService {
	return &ConversationService{
		db:                db,
		hub:               hub,
		model:             defaultModel,
		modeSvc:           modeSvc,
		processedRuns:     make(map[string]struct{}),
		completionWaiters: make(map[string]chan CompletionResult),
	}
}
```

Remove these imports from `conversation.go` (they're only used by the direct LLM path):
- `"github.com/Strob0t/CodeForge/internal/adapter/litellm"`
- `"go.opentelemetry.io/otel/codes"`

**Step 2: Rewrite `SendMessage` to dispatch via NATS**

Replace the entire `SendMessage` method (lines 197-332) with:

```go
// SendMessage stores the user message and dispatches a simple (non-agentic) chat run
// to the Python worker via NATS. The result arrives asynchronously via WebSocket AG-UI
// events and is stored by HandleConversationRunComplete.
func (s *ConversationService) SendMessage(ctx context.Context, conversationID string, req conversation.SendMessageRequest) (*conversation.Message, error) {
	if req.Content == "" {
		return nil, errors.New("content is required")
	}
	if s.queue == nil {
		return nil, errors.New("chat requires NATS queue")
	}

	// Verify conversation exists.
	conv, err := s.db.GetConversation(ctx, conversationID)
	if err != nil {
		return nil, fmt.Errorf("get conversation: %w", err)
	}

	// Store user message.
	userMsg := &conversation.Message{
		ConversationID: conversationID,
		Role:           "user",
		Content:        req.Content,
	}
	if _, err = s.db.CreateMessage(ctx, userMsg); err != nil {
		return nil, fmt.Errorf("store user message: %w", err)
	}

	// Load full conversation history.
	history, err := s.db.ListMessages(ctx, conversationID)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}

	// Build system prompt and convert history.
	systemPrompt := s.buildSystemPrompt(ctx, conv.ProjectID)
	protoMessages := s.historyToPayload(history)

	// Resolve model.
	model := s.resolveModel()
	if model == "" {
		return nil, errors.New("no LLM model configured — set conversation_model in litellm config or default_model in agent config")
	}

	// Use conversation ID as run ID.
	runID := conversationID

	payload := messagequeue.ConversationRunStartPayload{
		RunID:          runID,
		ConversationID: conversationID,
		ProjectID:      conv.ProjectID,
		Messages:       protoMessages,
		SystemPrompt:   systemPrompt,
		Model:          model,
		Agentic:        false,
		Termination: messagequeue.TerminationPayload{
			MaxSteps:       1,
			TimeoutSeconds: 120,
		},
		RoutingEnabled: s.routingCfg != nil && s.routingCfg.Enabled,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal conversation run start: %w", err)
	}

	// Broadcast run started via WebSocket.
	s.hub.BroadcastEvent(ctx, ws.AGUIRunStarted, ws.AGUIRunStartedEvent{
		RunID:     runID,
		ThreadID:  conversationID,
		AgentName: "assistant",
	})

	// Publish to NATS for the Python worker.
	if err := s.queue.PublishWithDedup(ctx, messagequeue.SubjectConversationRunStart, data, "conv-start-"+runID); err != nil {
		s.hub.BroadcastEvent(ctx, ws.AGUIRunFinished, ws.AGUIRunFinishedEvent{
			RunID:  runID,
			Status: "failed",
			Error:  err.Error(),
		})
		return nil, fmt.Errorf("publish conversation run start: %w", err)
	}

	if s.metrics != nil {
		s.metrics.RunsStarted.Add(ctx, 1, metric.WithAttributes(
			attribute.String("type", "conversation"),
			attribute.String("project.id", conv.ProjectID),
		))
	}

	slog.Info("conversation simple run dispatched",
		"run_id", runID,
		"conversation_id", conversationID,
		"project_id", conv.ProjectID,
		"model", model,
	)

	return nil, nil
}
```

**Step 3: Remove now-unused imports**

Remove from `conversation.go` imports:
- `"github.com/Strob0t/CodeForge/internal/adapter/litellm"`
- `"go.opentelemetry.io/otel/codes"`

Keep: `"encoding/json"` (used by `historyToPayload` in conversation_agent.go — verify with grep)

Actually `json` and `encoding/json` may still be needed — verify after edit. The `_ "embed"` import stays (for `//go:embed`).

---

### Task 4: Update all callers of NewConversationService

**Files:**
- Modify: `cmd/codeforge/main.go:502`
- Modify: `internal/service/conversation_test.go` (lines 81, 122, 136, 181, 243)
- Modify: `internal/service/autoagent_test.go` (lines 213, 814)
- Modify: `internal/adapter/http/handlers_test.go:1535`

**Step 1: Update main.go**

Change line 502 from:
```go
conversationSvc := service.NewConversationService(store, llmClient, hub, conversationModel, modeSvc)
```
To:
```go
conversationSvc := service.NewConversationService(store, hub, conversationModel, modeSvc)
```

**Step 2: Update all test constructors**

In `conversation_test.go`, change all `service.NewConversationService(store, nil, bc, "gpt-4o", modes)` to:
```go
service.NewConversationService(store, bc, "gpt-4o", modes)
```

In `autoagent_test.go`, change `NewConversationService(store, nil, hub, ...)` to:
```go
NewConversationService(store, hub, ...)
```

In `handlers_test.go:1535`, change:
```go
conversationSvc := service.NewConversationService(store, litellm.NewClient("http://localhost:4000", "test-key"), bc, "", nil)
```
To:
```go
conversationSvc := service.NewConversationService(store, bc, "", nil)
```
Remove the `litellm` import if it's no longer used in handlers_test.go.

---

### Task 5: Run tests and verify GREEN

**Step 1: Verify Go compiles**

Run: `go build ./...`
Expected: Success

**Step 2: Run new tests**

Run: `go test ./internal/service/... -v -run "TestSendMessage_Dispatches|TestSendMessage_Requires" -count=1`
Expected: PASS

**Step 3: Run full service test suite**

Run: `go test ./internal/service/... -v -count=1`
Expected: All pass (no regressions)

**Step 4: Run full Go test suite**

Run: `go test ./... -count=1`
Expected: All pass

---

### Task 6: Run full verification suite

**Step 1: Pre-commit hooks**

Run: `pre-commit run --all-files`
Expected: All 15 hooks pass

**Step 2: Python tests (regression)**

Run: `cd workers && python -m pytest tests/ -x -q`
Expected: 897+ passed

**Step 3: Frontend build (regression)**

Run: `cd frontend && npm run build`
Expected: Success

---

### Task 7: Commit

```bash
git add internal/port/messagequeue/schemas.go \
       internal/service/conversation.go \
       internal/service/conversation_test.go \
       internal/service/autoagent_test.go \
       internal/adapter/http/handlers_test.go \
       cmd/codeforge/main.go
git commit -m "feat: unify LLM path — all chat dispatched via NATS to Python Worker

SendMessage() no longer calls LiteLLM directly. Both simple and agentic
chat now route through NATS → Python Worker, ensuring HybridRouter,
Blocklist, Fallback, Cost Tracking, and Memory apply to all conversations.

- Add Agentic bool to ConversationRunStartPayload schema
- Rewrite SendMessage() to dispatch via NATS with Agentic=false
- Remove llm *litellm.Client from ConversationService (ADR-006 compliance)
- Update all callers of NewConversationService
- Add tests for NATS dispatch and queue requirement

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

## Summary

| Task | What | Files | Effort |
|------|------|-------|--------|
| 1 | Add `Agentic bool` to Go schema | `schemas.go` | 1 line |
| 2 | Write failing test (RED) | `conversation_test.go` | ~60 lines |
| 3 | Rewrite `SendMessage` + remove `llm` (GREEN) | `conversation.go` | ~80 lines |
| 4 | Update all callers | `main.go`, 3 test files | ~10 changes |
| 5 | Verify GREEN | — | Run tests |
| 6 | Full verification | — | Pre-commit + Python + Frontend |
| 7 | Commit | — | 1 commit |
