package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/agent"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// dispatchResultStore plays the store's dispatch check: only a result of
// the task's current dispatch becomes its result; a result of another
// dispatch only adds its cost.
type dispatchResultStore struct {
	mockStore
	current map[string]string // task -> current dispatch
	results map[string]task.Status
	costs   map[string]float64
}

func (s *dispatchResultStore) RecordTaskResult(_ context.Context, id, dispatchID string, status task.Status, _ task.Result, costUSD float64) (bool, error) {
	s.costs[id] += costUSD
	if s.current[id] != dispatchID {
		return false, nil
	}
	s.results[id] = status
	delete(s.current, id)
	return true, nil
}

// TestTaskResult_OnlyTheCurrentDispatchEndsTheTask (S2-G fix, 6): task
// results named no dispatch, so a late result of a dispatch the watchdog had
// failed overwrote the task's next dispatch and set its agent idle while
// that dispatch ran. tasks.result carries the dispatch_id the worker echoes;
// a result of another dispatch only records its cost.
func TestTaskResult_OnlyTheCurrentDispatchEndsTheTask(t *testing.T) {
	store := &dispatchResultStore{
		current: map[string]string{"t1": "d-2"},
		results: map[string]task.Status{},
		costs:   map[string]float64{},
	}
	store.agents = []agent.Agent{{ID: "agent-1", ProjectID: "p1", Name: "a", Status: agent.StatusRunning}}
	store.tasks = []task.Task{{ID: "t1", ProjectID: "p1", AgentID: "agent-1", Status: task.StatusQueued}}
	bc := &mockBroadcaster{}
	queue := newHandlerCapturingQueue()
	svc := NewAgentService(store, queue, bc)
	if _, err := svc.StartResultSubscriber(context.Background()); err != nil {
		t.Fatalf("StartResultSubscriber: %v", err)
	}
	handler, ok := queue.getHandler(messagequeue.SubjectTaskResult)
	if !ok {
		t.Fatal("no tasks.result handler")
	}
	deliver := func(p messagequeue.TaskResultPayload) {
		t.Helper()
		data, err := json.Marshal(p)
		if err != nil {
			t.Fatal(err)
		}
		if err := handler(context.Background(), messagequeue.SubjectTaskResult, data); err != nil {
			t.Fatalf("handler: %v", err)
		}
	}
	taskStatuses := func() []string {
		var statuses []string
		for _, ev := range bc.events {
			if ts, ok := ev.payload.(event.TaskStatusEvent); ok {
				statuses = append(statuses, ts.Status)
			}
		}
		return statuses
	}

	// The late result of the failed dispatch d-1.
	deliver(messagequeue.TaskResultPayload{TaskID: "t1", ProjectID: "p1", DispatchID: "d-1", Status: "completed", Output: "late", CostUSD: 0.2})
	if _, ok := store.results["t1"]; ok {
		t.Fatal("a result of an earlier dispatch became the task's result")
	}
	if store.costs["t1"] != 0.2 {
		t.Errorf("cost = %v, want the late result's 0.2 recorded", store.costs["t1"])
	}
	if got := taskStatuses(); len(got) != 0 {
		t.Errorf("task status broadcasts = %v, want none for an earlier dispatch", got)
	}
	if store.agents[0].Status != agent.StatusRunning {
		t.Errorf("agent status = %s, want running: its current dispatch runs", store.agents[0].Status)
	}

	// The result of the current dispatch d-2 ends the task.
	deliver(messagequeue.TaskResultPayload{TaskID: "t1", ProjectID: "p1", DispatchID: "d-2", Status: "completed", Output: "done", CostUSD: 0.3})
	if store.results["t1"] != task.StatusCompleted {
		t.Fatalf("task result = %q, want completed", store.results["t1"])
	}
	if got := taskStatuses(); len(got) != 1 || got[0] != "completed" {
		t.Errorf("task status broadcasts = %v, want completed", got)
	}
	if store.agents[0].Status != agent.StatusIdle {
		t.Errorf("agent status = %s, want idle", store.agents[0].Status)
	}
}
