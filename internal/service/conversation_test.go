package service_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/conversation"
	"github.com/Strob0t/CodeForge/internal/domain/mcp"
	"github.com/Strob0t/CodeForge/internal/domain/microagent"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
)

// convMockStore provides in-memory conversation and message storage.
type convMockStore struct {
	runtimeMockStore
	conversations []conversation.Conversation
	messages      []conversation.Message
	microagents   []microagent.Microagent
}

func (m *convMockStore) ListMicroagents(_ context.Context, _ string) ([]microagent.Microagent, error) {
	return m.microagents, nil
}

func (m *convMockStore) CreateConversation(_ context.Context, c *conversation.Conversation) (*conversation.Conversation, error) {
	c.ID = fmt.Sprintf("conv-%d", len(m.conversations)+1)
	m.conversations = append(m.conversations, *c)
	return c, nil
}

func (m *convMockStore) GetConversation(_ context.Context, id string) (*conversation.Conversation, error) {
	for i := range m.conversations {
		if m.conversations[i].ID == id {
			return &m.conversations[i], nil
		}
	}
	return nil, errMockNotFound
}

func (m *convMockStore) UpdateConversationMode(_ context.Context, id, mode string) error {
	for i := range m.conversations {
		if m.conversations[i].ID == id {
			m.conversations[i].Mode = mode
			return nil
		}
	}
	return errMockNotFound
}

func (m *convMockStore) UpdateConversationModel(_ context.Context, id, model string) error {
	for i := range m.conversations {
		if m.conversations[i].ID == id {
			m.conversations[i].Model = model
			return nil
		}
	}
	return errMockNotFound
}

func (m *convMockStore) ListConversationsByProject(_ context.Context, projectID string) ([]conversation.Conversation, error) {
	var result []conversation.Conversation
	for i := range m.conversations {
		if m.conversations[i].ProjectID == projectID {
			result = append(result, m.conversations[i])
		}
	}
	return result, nil
}

func (m *convMockStore) DeleteConversation(_ context.Context, id string) error {
	for i := range m.conversations {
		if m.conversations[i].ID == id {
			m.conversations = append(m.conversations[:i], m.conversations[i+1:]...)
			return nil
		}
	}
	return errMockNotFound
}

func (m *convMockStore) CreateMessage(_ context.Context, msg *conversation.Message) (*conversation.Message, error) {
	msg.ID = fmt.Sprintf("msg-%d", len(m.messages)+1)
	m.messages = append(m.messages, *msg)
	return msg, nil
}

func (m *convMockStore) ListMessages(_ context.Context, conversationID string) ([]conversation.Message, error) {
	var result []conversation.Message
	for i := range m.messages {
		if m.messages[i].ConversationID == conversationID {
			result = append(result, m.messages[i])
		}
	}
	return result, nil
}

func TestConversation_Create(t *testing.T) {
	store := &convMockStore{}
	bc := &mockBroadcaster{}
	modes := service.NewModeService()
	svc := service.NewConversationService(store, bc, "gpt-4o", modes)
	ctx := context.Background()

	// Create with title
	conv, err := svc.Create(ctx, conversation.CreateRequest{
		ProjectID: "proj-1",
		Title:     "Test Chat",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if conv.ProjectID != "proj-1" {
		t.Errorf("expected ProjectID proj-1, got %s", conv.ProjectID)
	}
	if conv.Title != "Test Chat" {
		t.Errorf("expected title 'Test Chat', got %q", conv.Title)
	}

	// Empty title defaults to "New Conversation"
	conv2, err := svc.Create(ctx, conversation.CreateRequest{
		ProjectID: "proj-1",
		Title:     "",
	})
	if err != nil {
		t.Fatalf("Create default title: %v", err)
	}
	if conv2.Title != "New Conversation" {
		t.Errorf("expected default title 'New Conversation', got %q", conv2.Title)
	}

	// Missing project_id fails
	_, err = svc.Create(ctx, conversation.CreateRequest{})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

func TestConversation_SetMode_Persisted(t *testing.T) {
	store := &convMockStore{}
	bc := &mockBroadcaster{}
	modes := service.NewModeService()
	svc := service.NewConversationService(store, bc, "gpt-4o", modes)
	ctx := context.Background()

	conv, err := svc.Create(ctx, conversation.CreateRequest{ProjectID: "proj-1", Title: "Mode Test"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if conv.Mode != "" {
		t.Fatalf("expected empty mode on new conversation, got %q", conv.Mode)
	}

	// Set mode
	if err := svc.SetMode(ctx, conv.ID, "architect"); err != nil {
		t.Fatalf("SetMode: %v", err)
	}

	// Verify mode persisted
	got, err := svc.Get(ctx, conv.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Mode != "architect" {
		t.Errorf("expected mode 'architect', got %q", got.Mode)
	}
}

func TestConversation_SetModel_Persisted(t *testing.T) {
	store := &convMockStore{}
	bc := &mockBroadcaster{}
	modes := service.NewModeService()
	svc := service.NewConversationService(store, bc, "gpt-4o", modes)
	ctx := context.Background()

	conv, err := svc.Create(ctx, conversation.CreateRequest{ProjectID: "proj-1", Title: "Model Test"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Set model
	if err := svc.SetModel(ctx, conv.ID, "claude-sonnet-4-20250514"); err != nil {
		t.Fatalf("SetModel: %v", err)
	}

	// Verify model persisted
	got, err := svc.Get(ctx, conv.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Model != "claude-sonnet-4-20250514" {
		t.Errorf("expected model 'claude-sonnet-4-20250514', got %q", got.Model)
	}
}

func TestConversation_SendMessage_EmptyContent(t *testing.T) {
	store := &convMockStore{}
	bc := &mockBroadcaster{}
	modes := service.NewModeService()
	svc := service.NewConversationService(store, bc, "gpt-4o", modes)
	ctx := context.Background()

	// SendMessage with empty content should fail validation before touching LLM
	_, err := svc.SendMessage(ctx, "conv-1", &conversation.SendMessageRequest{Content: ""})
	if err == nil {
		t.Fatal("expected error for empty content")
	}
}

func TestConversation_ListMessages(t *testing.T) {
	store := &convMockStore{}
	bc := &mockBroadcaster{}
	modes := service.NewModeService()
	svc := service.NewConversationService(store, bc, "gpt-4o", modes)
	ctx := context.Background()

	// Create a conversation
	conv, err := svc.Create(ctx, conversation.CreateRequest{
		ProjectID: "proj-1",
		Title:     "Messages Test",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Manually seed messages into mock store
	store.messages = append(store.messages,
		conversation.Message{ID: "m1", ConversationID: conv.ID, Role: "user", Content: "hello"},
		conversation.Message{ID: "m2", ConversationID: conv.ID, Role: "assistant", Content: "hi there"},
		conversation.Message{ID: "m3", ConversationID: "other-conv", Role: "user", Content: "unrelated"},
	)

	msgs, err := svc.ListMessages(ctx, conv.ID)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages for conv %s, got %d", conv.ID, len(msgs))
	}
	if msgs[0].Content != "hello" {
		t.Errorf("expected first message 'hello', got %q", msgs[0].Content)
	}
	if msgs[1].Role != "assistant" {
		t.Errorf("expected second message role 'assistant', got %q", msgs[1].Role)
	}
}

func TestSendMessageAgentic_ContextPopulatedWhenEnabled(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "handler.go"), []byte("package main\n\nfunc handleAuth() {}"), 0o644)

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
	ctxOpt := service.NewContextOptimizerService(store, orchCfg, &config.Limits{MaxFiles: 50, MaxFileSize: 32768, SearchTimeout: 5 * time.Second})
	svc.SetContextOptimizer(ctxOpt)

	ctx := context.Background()
	conv, err := svc.Create(ctx, conversation.CreateRequest{ProjectID: "proj-1", Title: "Agent Test"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	err = svc.SendMessageAgentic(ctx, conv.ID, &conversation.SendMessageRequest{Content: "Implement authentication handler"})
	if err != nil {
		t.Fatalf("SendMessageAgentic: %v", err)
	}

	_, data := q.snapshot()
	if len(data) == 0 {
		t.Fatal("expected NATS payload")
	}

	var payload messagequeue.ConversationRunStartPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if len(payload.Context) == 0 {
		t.Error("expected non-empty Context in payload when ContextEnabled=true")
	}

	foundHandler := false
	for _, ce := range payload.Context {
		if ce.Path == "handler.go" {
			foundHandler = true
		}
	}
	if !foundHandler {
		t.Error("expected handler.go in context entries")
	}
}

func TestSendMessage_DispatchesViaNATS(t *testing.T) {
	store := &convMockStore{}
	store.projects = []project.Project{
		{ID: "proj-1", Name: "Test", WorkspacePath: "/tmp/test"},
	}
	bc := &mockBroadcaster{}
	modes := service.NewModeService()
	svc := service.NewConversationService(store, bc, "gpt-4o", modes)
	q := &captureQueue{}
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
	_, err = svc.SendMessage(ctx, conv.ID, &conversation.SendMessageRequest{Content: "Hello"})
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
	_, err := svc.SendMessage(ctx, conv.ID, &conversation.SendMessageRequest{Content: "Hello"})
	if err == nil {
		t.Fatal("expected error when queue is nil")
	}
}

func TestSendMessageAgentic_ContextEmptyWhenDisabled(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "handler.go"), []byte("package main\n\nfunc handleAuth() {}"), 0o644)

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
		MaxLoopIterations: 10,
		MaxContextTokens:  120000,
		ContextEnabled:    false,
	}
	svc.SetAgentConfig(agentCfg)

	orchCfg := &config.Orchestrator{DefaultContextBudget: 8192, PromptReserve: 1024}
	ctxOpt := service.NewContextOptimizerService(store, orchCfg, &config.Limits{MaxFiles: 50, MaxFileSize: 32768, SearchTimeout: 5 * time.Second})
	svc.SetContextOptimizer(ctxOpt)

	ctx := context.Background()
	conv, err := svc.Create(ctx, conversation.CreateRequest{ProjectID: "proj-1", Title: "Agent Test Disabled"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	err = svc.SendMessageAgentic(ctx, conv.ID, &conversation.SendMessageRequest{Content: "Implement authentication handler"})
	if err != nil {
		t.Fatalf("SendMessageAgentic: %v", err)
	}

	_, data := q.snapshot()
	if len(data) == 0 {
		t.Fatal("expected NATS payload")
	}

	var payload messagequeue.ConversationRunStartPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if len(payload.Context) != 0 {
		t.Errorf("expected empty Context when ContextEnabled=false, got %d entries", len(payload.Context))
	}
}

func TestSendMessageAgenticWithMode_IncludesMicroagentsAndMCP(t *testing.T) {
	store := &convMockStore{}
	store.projects = []project.Project{
		{ID: "proj-1", Name: "test", WorkspacePath: "/tmp/test"},
	}
	// Seed a microagent with a trigger pattern that matches the test content.
	store.microagents = []microagent.Microagent{
		{
			ID:             "ma-1",
			ProjectID:      "proj-1",
			Name:           "auth-helper",
			TriggerPattern: "auth",
			Prompt:         "You are an authentication specialist.",
			Enabled:        true,
		},
	}

	q := &captureQueue{}
	bc := &mockBroadcaster{}
	modes := service.NewModeService()
	svc := service.NewConversationService(store, bc, "gpt-4o", modes)
	svc.SetQueue(q)
	svc.SetAgentConfig(&config.Agent{MaxLoopIterations: 10})

	// Configure MCP service with a registered server.
	mcpSvc := service.NewMCPService(&config.MCP{}, nil)
	_ = mcpSvc.Register(mcp.ServerDef{
		Name:      "test-mcp",
		Transport: mcp.TransportSSE,
		URL:       "http://localhost:3001",
		Enabled:   true,
	})
	svc.SetMCPService(mcpSvc)

	// Configure microagent service backed by our store.
	maSvc := service.NewMicroagentService(store)
	svc.SetMicroagentService(maSvc)

	ctx := context.Background()
	conv, err := svc.Create(ctx, conversation.CreateRequest{ProjectID: "proj-1", Title: "Mode Test"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Use mode override — this is the code path under test.
	err = svc.SendMessageAgenticWithMode(ctx, conv.ID, "Implement auth handler", "architect")
	if err != nil {
		t.Fatalf("SendMessageAgenticWithMode: %v", err)
	}

	_, data := q.snapshot()
	if len(data) == 0 {
		t.Fatal("expected NATS payload")
	}

	var payload messagequeue.ConversationRunStartPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	// Verify MCP servers are present in the payload.
	if len(payload.MCPServers) == 0 {
		t.Error("expected MCPServers in payload, got none")
	} else if payload.MCPServers[0].Name != "test-mcp" {
		t.Errorf("expected MCP server name 'test-mcp', got %q", payload.MCPServers[0].Name)
	}

	// Verify microagent prompts are present in the payload.
	if len(payload.MicroagentPrompts) == 0 {
		t.Error("expected MicroagentPrompts in payload, got none")
	} else if payload.MicroagentPrompts[0] != "You are an authentication specialist." {
		t.Errorf("expected microagent prompt, got %q", payload.MicroagentPrompts[0])
	}
}

func TestSendMessageAgentic_AdaptiveBudgetReducesContext(t *testing.T) {
	dir := t.TempDir()
	// Create a file that BM25 retrieval will match against the user message.
	_ = os.WriteFile(filepath.Join(dir, "handler.go"),
		[]byte("package main\n\nfunc handleAuth() {}\n"), 0o644)

	store := &convMockStore{}
	store.projects = []project.Project{
		{ID: "proj-1", Name: "test", WorkspacePath: dir},
	}
	bc := &mockBroadcaster{}
	modes := service.NewModeService()

	agentCfg := &config.Agent{
		MaxLoopIterations:    10,
		MaxContextTokens:     120000,
		ContextEnabled:       true,
		ContextBudget:        2048,
		ContextPromptReserve: 512,
	}
	orchCfg := &config.Orchestrator{DefaultContextBudget: 8192, PromptReserve: 1024}
	limCfg := &config.Limits{MaxFiles: 50, MaxFileSize: 32768, SearchTimeout: 5 * time.Second}
	ctxOpt := service.NewContextOptimizerService(store, orchCfg, limCfg)
	ctx := context.Background()

	// ---- Sub-test 1: fresh conversation (0 history) => context entries present ----
	t.Run("fresh conversation gets context", func(t *testing.T) {
		q := &captureQueue{}
		svc := service.NewConversationService(store, bc, "gpt-4o", modes)
		svc.SetQueue(q)
		svc.SetAgentConfig(agentCfg)
		svc.SetContextOptimizer(ctxOpt)

		conv, err := svc.Create(ctx, conversation.CreateRequest{ProjectID: "proj-1", Title: "Fresh"})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		err = svc.SendMessageAgentic(ctx, conv.ID, &conversation.SendMessageRequest{
			Content: "Implement authentication handler",
		})
		if err != nil {
			t.Fatalf("SendMessageAgentic: %v", err)
		}

		_, data := q.snapshot()
		var payload messagequeue.ConversationRunStartPayload
		if err := json.Unmarshal(data, &payload); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		if len(payload.Context) == 0 {
			t.Fatal("expected non-empty context entries for fresh conversation")
		}
	})

	// ---- Sub-test 2: 70+ messages => adaptive budget is 0 => no context entries ----
	t.Run("long history skips context", func(t *testing.T) {
		q := &captureQueue{}
		svc := service.NewConversationService(store, bc, "gpt-4o", modes)
		svc.SetQueue(q)
		svc.SetAgentConfig(agentCfg)
		svc.SetContextOptimizer(ctxOpt)

		conv, err := svc.Create(ctx, conversation.CreateRequest{ProjectID: "proj-1", Title: "Long"})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		// Seed 70+ messages so adaptive budget decays to 0 (threshold is 60).
		for i := 0; i < 70; i++ {
			role := "user"
			if i%2 == 1 {
				role = "assistant"
			}
			store.messages = append(store.messages, conversation.Message{
				ConversationID: conv.ID,
				Role:           role,
				Content:        fmt.Sprintf("Message %d about handlers and authentication", i),
			})
		}

		err = svc.SendMessageAgentic(ctx, conv.ID, &conversation.SendMessageRequest{
			Content: "Implement authentication handler",
		})
		if err != nil {
			t.Fatalf("SendMessageAgentic: %v", err)
		}

		_, data := q.snapshot()
		var payload messagequeue.ConversationRunStartPayload
		if err := json.Unmarshal(data, &payload); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		// With 70+ messages, adaptive budget should be 0, so no context entries.
		if len(payload.Context) != 0 {
			t.Errorf("expected zero context entries with 70+ message history, got %d", len(payload.Context))
		}
	})
}

func TestSendMessageAgenticWithMode_WithContextEntries(t *testing.T) {
	store := &convMockStore{}
	store.projects = []project.Project{
		{ID: "proj-1", Name: "test", WorkspacePath: "/tmp/test"},
	}

	q := &captureQueue{}
	bc := &mockBroadcaster{}
	modes := service.NewModeService()
	svc := service.NewConversationService(store, bc, "gpt-4o", modes)
	svc.SetQueue(q)
	svc.SetAgentConfig(&config.Agent{MaxLoopIterations: 10})

	ctx := context.Background()
	conv, err := svc.Create(ctx, conversation.CreateRequest{ProjectID: "proj-1", Title: "Context Test"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Provide extra context entries via the option.
	extraEntries := []messagequeue.ContextEntryPayload{
		{Kind: "file", Path: "docs/PROJECT.md", Content: "# My Project\nGoal: build something."},
		{Kind: "file", Path: "docs/STATE.md", Content: "## State\nIn progress."},
	}
	err = svc.SendMessageAgenticWithMode(ctx, conv.ID, "discover goals", "goal_researcher",
		service.WithContextEntries(extraEntries),
	)
	if err != nil {
		t.Fatalf("SendMessageAgenticWithMode: %v", err)
	}

	_, data := q.snapshot()
	if len(data) == 0 {
		t.Fatal("expected NATS payload")
	}

	var payload messagequeue.ConversationRunStartPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	// Verify the extra context entries appear in the payload.
	if len(payload.Context) < 2 {
		t.Fatalf("expected at least 2 context entries, got %d", len(payload.Context))
	}

	// Find our injected entries by path.
	foundProject := false
	foundState := false
	for _, ce := range payload.Context {
		switch ce.Path {
		case "docs/PROJECT.md":
			foundProject = true
			if ce.Kind != "file" {
				t.Errorf("expected Kind=file for PROJECT.md, got %q", ce.Kind)
			}
			if ce.Content != "# My Project\nGoal: build something." {
				t.Errorf("unexpected PROJECT.md content: %q", ce.Content)
			}
		case "docs/STATE.md":
			foundState = true
			if ce.Content != "## State\nIn progress." {
				t.Errorf("unexpected STATE.md content: %q", ce.Content)
			}
		}
	}
	if !foundProject {
		t.Error("docs/PROJECT.md context entry not found in payload")
	}
	if !foundState {
		t.Error("docs/STATE.md context entry not found in payload")
	}
}

func TestSendMessageAgenticWithMode_WithoutOptions_NoExtraContext(t *testing.T) {
	store := &convMockStore{}
	store.projects = []project.Project{
		{ID: "proj-1", Name: "test", WorkspacePath: "/tmp/test"},
	}

	q := &captureQueue{}
	bc := &mockBroadcaster{}
	modes := service.NewModeService()
	svc := service.NewConversationService(store, bc, "gpt-4o", modes)
	svc.SetQueue(q)
	svc.SetAgentConfig(&config.Agent{MaxLoopIterations: 10})

	ctx := context.Background()
	conv, err := svc.Create(ctx, conversation.CreateRequest{ProjectID: "proj-1", Title: "No Context Test"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Call without options -- backward compatibility.
	err = svc.SendMessageAgenticWithMode(ctx, conv.ID, "do something", "coder")
	if err != nil {
		t.Fatalf("SendMessageAgenticWithMode: %v", err)
	}

	_, data := q.snapshot()
	if len(data) == 0 {
		t.Fatal("expected NATS payload")
	}

	var payload messagequeue.ConversationRunStartPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	// Without context optimizer, Context should be nil/empty.
	if len(payload.Context) != 0 {
		t.Fatalf("expected 0 context entries without options, got %d", len(payload.Context))
	}
}

// --- Plan/Act Mode Toggle Tests (A3) ---

func TestPlanActEnabled_PayloadSerialization(t *testing.T) {
	// Verify plan_act_enabled field survives JSON round-trip.
	payload := messagequeue.ConversationRunStartPayload{
		RunID:          "run-1",
		ConversationID: "conv-1",
		ProjectID:      "proj-1",
		SystemPrompt:   "test",
		Model:          "gpt-4o",
		Agentic:        true,
		PlanActEnabled: true,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Verify JSON contains the field.
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal to map: %v", err)
	}
	val, ok := raw["plan_act_enabled"]
	if !ok {
		t.Fatal("expected plan_act_enabled in JSON output")
	}
	if val != true {
		t.Errorf("expected plan_act_enabled=true, got %v", val)
	}

	// Verify deserialization.
	var decoded messagequeue.ConversationRunStartPayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !decoded.PlanActEnabled {
		t.Error("expected PlanActEnabled=true after round-trip")
	}
}

func TestPlanActEnabled_DefaultFalse(t *testing.T) {
	// Verify default is false when field is omitted.
	jsonData := `{"run_id":"r1","conversation_id":"c1","project_id":"p1","messages":[],"system_prompt":"test","model":"gpt-4o","agentic":true}`

	var payload messagequeue.ConversationRunStartPayload
	if err := json.Unmarshal([]byte(jsonData), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload.PlanActEnabled {
		t.Error("expected PlanActEnabled=false when omitted from JSON")
	}
}

func TestSendMessageAgentic_PlanActEnabledForHighAutonomy(t *testing.T) {
	// Modes with autonomy >= 4 should set PlanActEnabled=true in the NATS payload.
	store := &convMockStore{}
	store.projects = []project.Project{
		{ID: "proj-1", Name: "test", WorkspacePath: "/tmp/test"},
	}

	q := &captureQueue{}
	bc := &mockBroadcaster{}
	modes := service.NewModeService()
	svc := service.NewConversationService(store, bc, "gpt-4o", modes)
	svc.SetQueue(q)
	svc.SetAgentConfig(&config.Agent{MaxLoopIterations: 10})

	ctx := context.Background()
	conv, err := svc.Create(ctx, conversation.CreateRequest{ProjectID: "proj-1", Title: "Plan/Act Test"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// "prototyper" mode has Autonomy=4 (>= 4 threshold).
	err = svc.SendMessageAgenticWithMode(ctx, conv.ID, "Build a prototype", "prototyper")
	if err != nil {
		t.Fatalf("SendMessageAgenticWithMode: %v", err)
	}

	_, data := q.snapshot()
	if len(data) == 0 {
		t.Fatal("expected NATS payload")
	}

	var payload messagequeue.ConversationRunStartPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !payload.PlanActEnabled {
		t.Error("expected PlanActEnabled=true for prototyper mode (autonomy=4)")
	}
}

func TestSendMessageAgentic_PlanActDisabledForLowAutonomy(t *testing.T) {
	// Modes with autonomy < 4 should set PlanActEnabled=false in the NATS payload.
	store := &convMockStore{}
	store.projects = []project.Project{
		{ID: "proj-1", Name: "test", WorkspacePath: "/tmp/test"},
	}

	q := &captureQueue{}
	bc := &mockBroadcaster{}
	modes := service.NewModeService()
	svc := service.NewConversationService(store, bc, "gpt-4o", modes)
	svc.SetQueue(q)
	svc.SetAgentConfig(&config.Agent{MaxLoopIterations: 10})

	ctx := context.Background()
	conv, err := svc.Create(ctx, conversation.CreateRequest{ProjectID: "proj-1", Title: "No Plan/Act Test"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// "coder" mode has Autonomy=3 (< 4 threshold).
	err = svc.SendMessageAgenticWithMode(ctx, conv.ID, "Write some code", "coder")
	if err != nil {
		t.Fatalf("SendMessageAgenticWithMode: %v", err)
	}

	_, data := q.snapshot()
	if len(data) == 0 {
		t.Fatal("expected NATS payload")
	}

	var payload messagequeue.ConversationRunStartPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if payload.PlanActEnabled {
		t.Error("expected PlanActEnabled=false for coder mode (autonomy=3)")
	}
}

func TestSendMessageAgentic_DefaultMode_PlanActDisabled(t *testing.T) {
	// SendMessageAgentic defaults to "coder" mode (autonomy=3), so PlanActEnabled should be false.
	store := &convMockStore{}
	store.projects = []project.Project{
		{ID: "proj-1", Name: "test", WorkspacePath: "/tmp/test"},
	}

	q := &captureQueue{}
	bc := &mockBroadcaster{}
	modes := service.NewModeService()
	svc := service.NewConversationService(store, bc, "gpt-4o", modes)
	svc.SetQueue(q)
	svc.SetAgentConfig(&config.Agent{MaxLoopIterations: 10})

	ctx := context.Background()
	conv, err := svc.Create(ctx, conversation.CreateRequest{ProjectID: "proj-1", Title: "Default Mode Test"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	err = svc.SendMessageAgentic(ctx, conv.ID, &conversation.SendMessageRequest{Content: "Hello"})
	if err != nil {
		t.Fatalf("SendMessageAgentic: %v", err)
	}

	_, data := q.snapshot()
	if len(data) == 0 {
		t.Fatal("expected NATS payload")
	}

	var payload messagequeue.ConversationRunStartPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if payload.PlanActEnabled {
		t.Error("expected PlanActEnabled=false for default coder mode (autonomy=3)")
	}
}

func TestSendMessageAgenticWithMode_BoundaryAnalyzer_PlanActEnabled(t *testing.T) {
	// boundary_analyzer has Autonomy=4, should enable plan/act.
	store := &convMockStore{}
	store.projects = []project.Project{
		{ID: "proj-1", Name: "test", WorkspacePath: "/tmp/test"},
	}

	q := &captureQueue{}
	bc := &mockBroadcaster{}
	modes := service.NewModeService()
	svc := service.NewConversationService(store, bc, "gpt-4o", modes)
	svc.SetQueue(q)
	svc.SetAgentConfig(&config.Agent{MaxLoopIterations: 10})

	ctx := context.Background()
	conv, err := svc.Create(ctx, conversation.CreateRequest{ProjectID: "proj-1", Title: "Boundary Test"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	err = svc.SendMessageAgenticWithMode(ctx, conv.ID, "Analyze boundaries", "boundary_analyzer")
	if err != nil {
		t.Fatalf("SendMessageAgenticWithMode: %v", err)
	}

	_, data := q.snapshot()
	var payload messagequeue.ConversationRunStartPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !payload.PlanActEnabled {
		t.Error("expected PlanActEnabled=true for boundary_analyzer (autonomy=4)")
	}
}
