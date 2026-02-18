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
