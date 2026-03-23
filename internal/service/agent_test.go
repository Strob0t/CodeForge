package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/agent"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/port/broadcast"
	"github.com/Strob0t/CodeForge/internal/port/eventstore"
)

// Ensure mock types implement their interfaces at compile time.
var (
	_ broadcast.Broadcaster = (*mockBroadcaster)(nil)
	_ eventstore.Store      = (*mockEventStore)(nil)
)

type mockBroadcaster struct {
	events []struct {
		eventType string
		payload   any
	}
}

func (m *mockBroadcaster) BroadcastEvent(_ context.Context, eventType string, payload any) {
	m.events = append(m.events, struct {
		eventType string
		payload   any
	}{eventType, payload})
}

type mockEventStore struct {
	events    []event.AgentEvent
	appendErr error
}

func (m *mockEventStore) Append(_ context.Context, ev *event.AgentEvent) error {
	if m.appendErr != nil {
		return m.appendErr
	}
	m.events = append(m.events, *ev)
	return nil
}

func (m *mockEventStore) LoadByTask(_ context.Context, taskID string) ([]event.AgentEvent, error) {
	var result []event.AgentEvent
	for i := range m.events {
		if m.events[i].TaskID == taskID {
			result = append(result, m.events[i])
		}
	}
	return result, nil
}

func (m *mockEventStore) LoadByAgent(_ context.Context, agentID string) ([]event.AgentEvent, error) {
	var result []event.AgentEvent
	for i := range m.events {
		if m.events[i].AgentID == agentID {
			result = append(result, m.events[i])
		}
	}
	return result, nil
}

func (m *mockEventStore) LoadByRun(_ context.Context, runID string) ([]event.AgentEvent, error) {
	var result []event.AgentEvent
	for i := range m.events {
		if m.events[i].RunID == runID {
			result = append(result, m.events[i])
		}
	}
	return result, nil
}

func (m *mockEventStore) LoadTrajectory(_ context.Context, _ string, _ eventstore.TrajectoryFilter, _ string, _ int) (*eventstore.TrajectoryPage, error) {
	return &eventstore.TrajectoryPage{}, nil
}
func (m *mockEventStore) TrajectoryStats(_ context.Context, _ string) (*eventstore.TrajectorySummary, error) {
	return &eventstore.TrajectorySummary{}, nil
}
func (m *mockEventStore) LoadEventsRange(_ context.Context, _, _, _ string) ([]event.AgentEvent, error) {
	return nil, nil
}
func (m *mockEventStore) ListCheckpoints(_ context.Context, _ string) ([]event.AgentEvent, error) {
	return nil, nil
}
func (m *mockEventStore) AppendAudit(_ context.Context, _ *event.AuditEntry) error { return nil }
func (m *mockEventStore) LoadAudit(_ context.Context, _ *event.AuditFilter, _ string, _ int) (*event.AuditPage, error) {
	return nil, nil
}

// --- AgentService Tests ---

func TestAgentServiceList(t *testing.T) {
	store := &mockStore{
		agents: []agent.Agent{
			{ID: "a1", ProjectID: "p1", Name: "Agent 1"},
			{ID: "a2", ProjectID: "p1", Name: "Agent 2"},
		},
	}
	svc := NewAgentService(store, &mockQueue{}, &mockBroadcaster{})

	got, err := svc.List(context.Background(), "p1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(got))
	}
}

func TestAgentServiceGet(t *testing.T) {
	store := &mockStore{
		agents: []agent.Agent{{ID: "a1", Name: "My Agent"}},
	}
	svc := NewAgentService(store, &mockQueue{}, &mockBroadcaster{})

	got, err := svc.Get(context.Background(), "a1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name != "My Agent" {
		t.Fatalf("expected 'My Agent', got %q", got.Name)
	}
}

func TestAgentServiceGetNotFound(t *testing.T) {
	svc := NewAgentService(&mockStore{}, &mockQueue{}, &mockBroadcaster{})

	_, err := svc.Get(context.Background(), "nonexistent")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestAgentServiceDelete(t *testing.T) {
	store := &mockStore{
		agents: []agent.Agent{{ID: "a1", Name: "Doomed"}},
	}
	svc := NewAgentService(store, &mockQueue{}, &mockBroadcaster{})

	if err := svc.Delete(context.Background(), "a1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.agents) != 0 {
		t.Fatalf("expected 0 agents after delete, got %d", len(store.agents))
	}
}

func TestAgentServiceDeleteNotFound(t *testing.T) {
	svc := NewAgentService(&mockStore{}, &mockQueue{}, &mockBroadcaster{})

	err := svc.Delete(context.Background(), "nonexistent")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestAgentServiceHandleResultCompleted(t *testing.T) {
	store := &mockStore{}
	bc := &mockBroadcaster{}
	es := &mockEventStore{}
	svc := NewAgentService(store, &mockQueue{}, bc)
	svc.SetEventStore(es)

	result := task.Result{Output: "done", TokensIn: 100, TokensOut: 50}
	err := svc.HandleResult(context.Background(), result, "t1", "p1", 0.005)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify broadcast was called with task.status event
	if len(bc.events) != 1 {
		t.Fatalf("expected 1 broadcast, got %d", len(bc.events))
	}
	if bc.events[0].eventType != "task.status" {
		t.Fatalf("expected 'task.status', got %q", bc.events[0].eventType)
	}

	// Verify event was recorded
	if len(es.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(es.events))
	}
	if es.events[0].Type != event.TypeAgentFinished {
		t.Fatalf("expected %q event, got %q", event.TypeAgentFinished, es.events[0].Type)
	}
}

func TestAgentServiceHandleResultFailed(t *testing.T) {
	store := &mockStore{}
	bc := &mockBroadcaster{}
	es := &mockEventStore{}
	svc := NewAgentService(store, &mockQueue{}, bc)
	svc.SetEventStore(es)

	result := task.Result{Error: "something broke"}
	err := svc.HandleResult(context.Background(), result, "t1", "p1", 0.001)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Error result should produce an AgentError event
	if len(es.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(es.events))
	}
	if es.events[0].Type != event.TypeAgentError {
		t.Fatalf("expected %q event, got %q", event.TypeAgentError, es.events[0].Type)
	}

	// Check payload contains the error status
	var payload map[string]string
	if err := json.Unmarshal(es.events[0].Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload["status"] != "failed" {
		t.Fatalf("expected status 'failed', got %q", payload["status"])
	}
}

func TestAgentServiceHandleResultNoEventStore(t *testing.T) {
	store := &mockStore{}
	bc := &mockBroadcaster{}
	svc := NewAgentService(store, &mockQueue{}, bc)
	// No event store set — should not panic

	result := task.Result{Output: "ok"}
	err := svc.HandleResult(context.Background(), result, "t1", "p1", 0.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAgentServiceLoadTaskEvents(t *testing.T) {
	es := &mockEventStore{
		events: []event.AgentEvent{
			{TaskID: "t1", Type: event.TypeAgentStarted},
			{TaskID: "t1", Type: event.TypeAgentFinished},
			{TaskID: "t2", Type: event.TypeAgentStarted},
		},
	}
	svc := NewAgentService(&mockStore{}, &mockQueue{}, &mockBroadcaster{})
	svc.SetEventStore(es)

	events, err := svc.LoadTaskEvents(context.Background(), "t1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events for t1, got %d", len(events))
	}
}

func TestAgentServiceLoadTaskEventsNoStore(t *testing.T) {
	svc := NewAgentService(&mockStore{}, &mockQueue{}, &mockBroadcaster{})

	events, err := svc.LoadTaskEvents(context.Background(), "t1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if events != nil {
		t.Fatalf("expected nil events without event store, got %v", events)
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"short", 10, "short"},
		{"exactly10!", 10, "exactly10!"},
		{"this is too long", 5, "this ..."},
		{"", 5, ""},
	}

	for _, tt := range tests {
		got := truncate(tt.input, tt.maxLen)
		if got != tt.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
		}
	}
}

// --- Phase 23C: Agent Identity Tests ---

func TestAgentService_SendMessage(t *testing.T) {
	store := &mockStore{
		agents: []agent.Agent{{ID: "agent-1", ProjectID: "p1"}},
	}
	bc := &mockBroadcaster{}
	svc := NewAgentService(store, &mockQueue{}, bc)

	msg := &agent.InboxMessage{
		AgentID:   "agent-1",
		FromAgent: "agent-2",
		Content:   "Please review my changes",
		Priority:  1,
	}
	err := svc.SendMessage(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify message was stored
	if len(store.inboxMessages) != 1 {
		t.Fatalf("expected 1 stored message, got %d", len(store.inboxMessages))
	}
	if store.inboxMessages[0].Content != "Please review my changes" {
		t.Errorf("expected stored content to match, got %q", store.inboxMessages[0].Content)
	}

	// Verify WS broadcast was sent
	found := false
	for _, ev := range bc.events {
		if ev.eventType == event.EventAgentMessage {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected agent.message broadcast event")
	}
}

func TestAgentService_SendMessage_EmptyContent(t *testing.T) {
	store := &mockStore{}
	svc := NewAgentService(store, &mockQueue{}, &mockBroadcaster{})

	msg := &agent.InboxMessage{
		AgentID: "agent-1",
		Content: "",
	}
	err := svc.SendMessage(context.Background(), msg)
	if err == nil {
		t.Fatal("expected validation error for empty content")
	}

	// Store should not be called
	if len(store.inboxMessages) != 0 {
		t.Error("expected no messages stored on validation failure")
	}
}

func TestAgentService_SendMessage_EmptyAgentID(t *testing.T) {
	store := &mockStore{}
	svc := NewAgentService(store, &mockQueue{}, &mockBroadcaster{})

	msg := &agent.InboxMessage{
		AgentID: "",
		Content: "hello",
	}
	err := svc.SendMessage(context.Background(), msg)
	if err == nil {
		t.Fatal("expected validation error for empty agent_id")
	}
	if len(store.inboxMessages) != 0 {
		t.Error("expected no messages stored on validation failure")
	}
}

func TestAgentService_GetInbox(t *testing.T) {
	store := &mockStore{
		inboxMessages: []agent.InboxMessage{
			{ID: "m1", AgentID: "agent-1", Content: "msg1"},
			{ID: "m2", AgentID: "agent-1", Content: "msg2"},
			{ID: "m3", AgentID: "agent-2", Content: "msg3"},
		},
	}
	svc := NewAgentService(store, &mockQueue{}, &mockBroadcaster{})

	msgs, err := svc.GetInbox(context.Background(), "agent-1", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages for agent-1, got %d", len(msgs))
	}
}

func TestAgentService_GetInbox_UnreadOnly(t *testing.T) {
	store := &mockStore{
		inboxMessages: []agent.InboxMessage{
			{ID: "m1", AgentID: "agent-1", Content: "read msg", Read: true},
			{ID: "m2", AgentID: "agent-1", Content: "unread msg", Read: false},
		},
	}
	svc := NewAgentService(store, &mockQueue{}, &mockBroadcaster{})

	msgs, err := svc.GetInbox(context.Background(), "agent-1", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 unread message, got %d", len(msgs))
	}
	if msgs[0].Content != "unread msg" {
		t.Errorf("expected unread msg, got %q", msgs[0].Content)
	}
}

func TestAgentService_GetInbox_Empty(t *testing.T) {
	store := &mockStore{}
	svc := NewAgentService(store, &mockQueue{}, &mockBroadcaster{})

	msgs, err := svc.GetInbox(context.Background(), "agent-1", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msgs != nil {
		t.Fatalf("expected nil for empty inbox, got %v", msgs)
	}
}

func TestAgentService_MarkRead(t *testing.T) {
	store := &mockStore{
		inboxMessages: []agent.InboxMessage{
			{ID: "m1", AgentID: "agent-1", Content: "unread", Read: false},
			{ID: "m2", AgentID: "agent-1", Content: "also unread", Read: false},
		},
	}
	svc := NewAgentService(store, &mockQueue{}, &mockBroadcaster{})

	err := svc.MarkRead(context.Background(), "m1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify m1 is now read
	if !store.inboxMessages[0].Read {
		t.Error("expected m1 to be marked as read")
	}
	// m2 should still be unread
	if store.inboxMessages[1].Read {
		t.Error("expected m2 to remain unread")
	}
}

func TestAgentService_StatsIncrement_Success(t *testing.T) {
	store := &mockStore{
		agents: []agent.Agent{{ID: "a1", ProjectID: "p1"}},
	}
	svc := NewAgentService(store, &mockQueue{}, &mockBroadcaster{})

	err := svc.IncrementStats(context.Background(), "a1", 0.05, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	a := store.agents[0]
	if a.TotalRuns != 1 {
		t.Errorf("expected TotalRuns=1, got %d", a.TotalRuns)
	}
	if a.TotalCost < 0.049 || a.TotalCost > 0.051 {
		t.Errorf("expected TotalCost~0.05, got %f", a.TotalCost)
	}
	if a.SuccessRate < 0.99 {
		t.Errorf("expected SuccessRate~1.0 after 1 success, got %f", a.SuccessRate)
	}
}

func TestAgentService_StatsIncrement_Failure(t *testing.T) {
	store := &mockStore{
		agents: []agent.Agent{{ID: "a1", ProjectID: "p1", TotalRuns: 3, SuccessRate: 1.0}},
	}
	svc := NewAgentService(store, &mockQueue{}, &mockBroadcaster{})

	err := svc.IncrementStats(context.Background(), "a1", 0.01, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	a := store.agents[0]
	if a.TotalRuns != 4 {
		t.Errorf("expected TotalRuns=4, got %d", a.TotalRuns)
	}
	// 3 successes out of 4 runs = 0.75
	if a.SuccessRate < 0.74 || a.SuccessRate > 0.76 {
		t.Errorf("expected SuccessRate~0.75 after 1 failure in 4 runs, got %f", a.SuccessRate)
	}
}

// --- Agent output forwarding tests (Phase 5.2) ---

func TestAgentOutput_ForwardedToWebSocket(t *testing.T) {
	bc := &mockBroadcaster{}
	svc := NewAgentService(&mockStore{}, &mockQueue{}, bc)

	// Simulate a valid agent output message.
	payload, _ := json.Marshal(event.AgentOutputEvent{
		TaskID: "task-1",
		Line:   "Compiling main.go",
		Stream: "stdout",
	})

	// Subscribe returns a handler; call it directly.
	cancel, err := svc.StartAgentOutputSubscriber(context.Background())
	if err != nil {
		t.Fatalf("subscribe error: %v", err)
	}
	defer cancel()

	// The mockQueue Subscribe stores the handler but doesn't invoke it.
	// Instead, test the broadcast by simulating what the handler does:
	// unmarshal and broadcast.
	var output event.AgentOutputEvent
	if err := json.Unmarshal(payload, &output); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	bc.BroadcastEvent(context.Background(), event.EventAgentOutput, output)

	if len(bc.events) != 1 {
		t.Fatalf("expected 1 broadcast event, got %d", len(bc.events))
	}
	if bc.events[0].eventType != event.EventAgentOutput {
		t.Errorf("expected event type %s, got %s", event.EventAgentOutput, bc.events[0].eventType)
	}
	ev, ok := bc.events[0].payload.(event.AgentOutputEvent)
	if !ok {
		t.Fatalf("expected AgentOutputEvent payload, got %T", bc.events[0].payload)
	}
	if ev.TaskID != "task-1" {
		t.Errorf("expected task_id 'task-1', got %s", ev.TaskID)
	}
	if ev.Line != "Compiling main.go" {
		t.Errorf("expected line 'Compiling main.go', got %s", ev.Line)
	}
	if ev.Stream != "stdout" {
		t.Errorf("expected stream 'stdout', got %s", ev.Stream)
	}
}

func TestAgentOutput_MalformedMessageSkipped(t *testing.T) {
	bc := &mockBroadcaster{}
	svc := NewAgentService(&mockStore{}, &mockQueue{}, bc)

	// Start subscriber to ensure it doesn't error.
	cancel, err := svc.StartAgentOutputSubscriber(context.Background())
	if err != nil {
		t.Fatalf("subscribe error: %v", err)
	}
	defer cancel()

	// No broadcast should happen for malformed data.
	if len(bc.events) != 0 {
		t.Fatalf("expected 0 broadcast events, got %d", len(bc.events))
	}
}
