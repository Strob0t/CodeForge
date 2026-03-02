package service

import (
	"context"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/adapter/ws"
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/agent"
	"github.com/Strob0t/CodeForge/internal/domain/task"
)

// --- ActiveWorkService mock store ---

// activeWorkMockStore extends mockStore with active-work-specific behaviour.
type activeWorkMockStore struct {
	mockStore

	activeItems     []task.ActiveWorkItem
	listActiveErr   error
	claimResult     *task.ClaimResult
	claimErr        error
	releasedTasks   []task.Task
	releaseStaleErr error
}

func (m *activeWorkMockStore) ListActiveWork(_ context.Context, _ string) ([]task.ActiveWorkItem, error) {
	return m.activeItems, m.listActiveErr
}

func (m *activeWorkMockStore) ClaimTask(_ context.Context, taskID, agentID string, version int) (*task.ClaimResult, error) {
	if m.claimErr != nil {
		return nil, m.claimErr
	}
	if m.claimResult != nil {
		return m.claimResult, nil
	}
	// Default: find task, check status + version
	for i := range m.tasks {
		if m.tasks[i].ID != taskID {
			continue
		}
		if m.tasks[i].Status != task.StatusPending {
			return &task.ClaimResult{Claimed: false, Reason: "task not pending"}, nil
		}
		if m.tasks[i].Version != version {
			return &task.ClaimResult{Claimed: false, Reason: "version mismatch"}, nil
		}
		m.tasks[i].AgentID = agentID
		m.tasks[i].Status = task.StatusQueued
		m.tasks[i].Version++
		return &task.ClaimResult{Task: &m.tasks[i], Claimed: true}, nil
	}
	return nil, domain.ErrNotFound
}

func (m *activeWorkMockStore) ReleaseStaleWork(_ context.Context, _ time.Duration) ([]task.Task, error) {
	return m.releasedTasks, m.releaseStaleErr
}

// --- Tests ---

func TestActiveWorkServiceListDelegatesToStore(t *testing.T) {
	items := []task.ActiveWorkItem{
		{TaskID: "t1", TaskTitle: "Fix auth", TaskStatus: task.StatusRunning, ProjectID: "p1", AgentID: "a1", AgentName: "Coder"},
		{TaskID: "t2", TaskTitle: "Add tests", TaskStatus: task.StatusQueued, ProjectID: "p1", AgentID: "a2", AgentName: "Tester"},
	}
	store := &activeWorkMockStore{activeItems: items}
	bc := &mockBroadcaster{}
	svc := NewActiveWorkService(store, bc)

	got, err := svc.ListActiveWork(context.Background(), "p1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 items, got %d", len(got))
	}
	if got[0].TaskID != "t1" {
		t.Errorf("first item task_id = %q, want t1", got[0].TaskID)
	}
	if got[1].AgentName != "Tester" {
		t.Errorf("second item agent_name = %q, want Tester", got[1].AgentName)
	}
}

func TestActiveWorkServiceListEmpty(t *testing.T) {
	store := &activeWorkMockStore{}
	svc := NewActiveWorkService(store, &mockBroadcaster{})

	got, err := svc.ListActiveWork(context.Background(), "p1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		// nil is acceptable but we prefer empty slice for JSON
		got = []task.ActiveWorkItem{}
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 items, got %d", len(got))
	}
}

func TestActiveWorkServiceListStoreError(t *testing.T) {
	store := &activeWorkMockStore{listActiveErr: domain.ErrNotFound}
	svc := NewActiveWorkService(store, &mockBroadcaster{})

	_, err := svc.ListActiveWork(context.Background(), "p1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestActiveWorkServiceClaimSuccess(t *testing.T) {
	store := &activeWorkMockStore{
		mockStore: mockStore{
			tasks: []task.Task{
				{ID: "t1", ProjectID: "p1", Title: "Fix auth", Status: task.StatusPending, Version: 1},
			},
			agents: []agent.Agent{
				{ID: "a1", Name: "Coder", ProjectID: "p1"},
			},
		},
	}
	bc := &mockBroadcaster{}
	svc := NewActiveWorkService(store, bc)

	result, err := svc.ClaimTask(context.Background(), "t1", "a1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Claimed {
		t.Fatalf("expected claimed=true, got false: %s", result.Reason)
	}
	if result.Task == nil {
		t.Fatal("expected task in result, got nil")
	}

	// Verify WS broadcast was sent
	if len(bc.events) != 1 {
		t.Fatalf("expected 1 broadcast, got %d", len(bc.events))
	}
	if bc.events[0].eventType != ws.EventActiveWorkClaimed {
		t.Errorf("event type = %q, want %q", bc.events[0].eventType, ws.EventActiveWorkClaimed)
	}
	ev, ok := bc.events[0].payload.(ws.ActiveWorkClaimedEvent)
	if !ok {
		t.Fatalf("payload type = %T, want ActiveWorkClaimedEvent", bc.events[0].payload)
	}
	if ev.TaskID != "t1" {
		t.Errorf("event task_id = %q, want t1", ev.TaskID)
	}
	if ev.AgentName != "Coder" {
		t.Errorf("event agent_name = %q, want Coder", ev.AgentName)
	}
}

func TestActiveWorkServiceClaimAlreadyClaimedNoBroadcast(t *testing.T) {
	store := &activeWorkMockStore{
		mockStore: mockStore{
			tasks: []task.Task{
				{ID: "t1", ProjectID: "p1", Title: "Fix auth", Status: task.StatusRunning, Version: 2},
			},
		},
	}
	bc := &mockBroadcaster{}
	svc := NewActiveWorkService(store, bc)

	result, err := svc.ClaimTask(context.Background(), "t1", "a1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Claimed {
		t.Fatal("expected claimed=false for non-pending task")
	}
	if len(bc.events) != 0 {
		t.Errorf("expected 0 broadcasts on failed claim, got %d", len(bc.events))
	}
}

func TestActiveWorkServiceClaimTaskNotFound(t *testing.T) {
	store := &activeWorkMockStore{}
	svc := NewActiveWorkService(store, &mockBroadcaster{})

	_, err := svc.ClaimTask(context.Background(), "nonexistent", "a1")
	if err == nil {
		t.Fatal("expected error for nonexistent task")
	}
}

func TestActiveWorkServiceClaimVersionMismatch(t *testing.T) {
	store := &activeWorkMockStore{
		mockStore: mockStore{
			tasks: []task.Task{
				{ID: "t1", ProjectID: "p1", Title: "Fix auth", Status: task.StatusPending, Version: 5},
			},
			agents: []agent.Agent{
				{ID: "a1", Name: "Coder", ProjectID: "p1"},
			},
		},
	}
	bc := &mockBroadcaster{}
	svc := NewActiveWorkService(store, bc)

	// Force version mismatch at store level
	store.claimResult = &task.ClaimResult{Claimed: false, Reason: "version mismatch"}

	result, err := svc.ClaimTask(context.Background(), "t1", "a1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Claimed {
		t.Fatal("expected claimed=false for version mismatch")
	}
	if len(bc.events) != 0 {
		t.Errorf("expected 0 broadcasts on version mismatch, got %d", len(bc.events))
	}
}

func TestActiveWorkServiceReleaseStaleWorkBroadcastsPerTask(t *testing.T) {
	released := []task.Task{
		{ID: "t1", ProjectID: "p1", Title: "Stuck task 1"},
		{ID: "t2", ProjectID: "p2", Title: "Stuck task 2"},
	}
	store := &activeWorkMockStore{releasedTasks: released}
	bc := &mockBroadcaster{}
	svc := NewActiveWorkService(store, bc)

	got, err := svc.ReleaseStaleWork(context.Background(), 30*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 released tasks, got %d", len(got))
	}

	// Should broadcast one EventActiveWorkReleased per task
	if len(bc.events) != 2 {
		t.Fatalf("expected 2 broadcasts, got %d", len(bc.events))
	}
	for i, ev := range bc.events {
		if ev.eventType != ws.EventActiveWorkReleased {
			t.Errorf("event[%d] type = %q, want %q", i, ev.eventType, ws.EventActiveWorkReleased)
		}
		rel, ok := ev.payload.(ws.ActiveWorkReleasedEvent)
		if !ok {
			t.Fatalf("event[%d] payload type = %T, want ActiveWorkReleasedEvent", i, ev.payload)
		}
		if rel.TaskID != released[i].ID {
			t.Errorf("event[%d] task_id = %q, want %q", i, rel.TaskID, released[i].ID)
		}
	}
}

func TestActiveWorkServiceReleaseStaleWorkNone(t *testing.T) {
	store := &activeWorkMockStore{} // no released tasks
	bc := &mockBroadcaster{}
	svc := NewActiveWorkService(store, bc)

	got, err := svc.ReleaseStaleWork(context.Background(), 30*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 released tasks, got %d", len(got))
	}
	if len(bc.events) != 0 {
		t.Errorf("expected 0 broadcasts when nothing released, got %d", len(bc.events))
	}
}

func TestActiveWorkServiceReleaseStaleWorkError(t *testing.T) {
	store := &activeWorkMockStore{releaseStaleErr: domain.ErrNotFound}
	svc := NewActiveWorkService(store, &mockBroadcaster{})

	_, err := svc.ReleaseStaleWork(context.Background(), 30*time.Minute)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
