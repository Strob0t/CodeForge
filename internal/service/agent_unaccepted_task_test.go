package service

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/agent"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// Backend tasks that no worker ever accepted (S2-F review, F5). The
// watchdog that reset tasks stuck in queued was replaced by the heartbeat
// check, which only sees tasks with a heartbeat, and nobody read
// tasks.agent.*.dlq: a dispatch dead-lettered by the worker, or one that
// waited in NATS forever, left its task queued and its agent running.

// unacceptedTaskStore lists never accepted tasks on top of lostTaskStore.
type unacceptedTaskStore struct {
	lostTaskStore
	unaccepted  []task.Task
	acceptAfter time.Duration
}

func (s *unacceptedTaskStore) ListTasksNeverAccepted(_ context.Context, olderThan time.Duration, _ int) ([]task.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.acceptAfter = olderThan
	return s.unaccepted, nil
}

func newUnacceptedEnv(t *testing.T) (*unacceptedTaskStore, *AgentService, *tenantRecorder, *executionProbeBackend) {
	t.Helper()
	probe := registerExecutionProbe(t)
	store := &unacceptedTaskStore{}
	store.agents = []agent.Agent{{ID: "agent-1", ProjectID: "p-a", Name: "a", Backend: "execution-probe", Status: agent.StatusRunning}}
	hub := &tenantRecorder{}
	return store, NewAgentService(store, &mockQueue{}, hub), hub, probe
}

func TestFailTasksNeverAccepted(t *testing.T) {
	store, svc, hub, probe := newUnacceptedEnv(t)
	store.tasks = []task.Task{
		{ID: "t-a", ProjectID: "p-a", TenantID: scopeTenantA, AgentID: "agent-1", Status: task.StatusQueued, DispatchID: "d-1"},
		{ID: "t-ended", ProjectID: "p-a", TenantID: scopeTenantA, AgentID: "agent-1", Status: task.StatusQueued, DispatchID: "d-2"},
	}
	store.unaccepted = store.tasks
	store.ended = map[string]bool{"t-ended": true} // its result arrived meanwhile

	failed, err := svc.FailTasksNeverAccepted(context.Background(), time.Hour)
	if err != nil || failed != 1 {
		t.Fatalf("FailTasksNeverAccepted = %d, %v; want 1, nil", failed, err)
	}
	if store.acceptAfter != time.Hour {
		t.Errorf("listed tasks waiting for %s, want 1h", store.acceptAfter)
	}
	got, ok := store.results["t-a"]
	if !ok || got.status != task.StatusFailed || !strings.Contains(got.result.Error, "no worker accepted") || got.tenant != scopeTenantA {
		t.Fatalf("t-a result = %+v (stored %v), want failed in tenant A because no worker accepted it", got, ok)
	}
	if _, ok := store.results["t-ended"]; ok {
		t.Error("the task whose result arrived meanwhile was failed")
	}
	// The dispatch may still wait in NATS: its worker is told it is
	// cancelled, so a late pickup skips it.
	if stops := probe.stops(); !slices.Equal(stops, []string{"t-a"}) {
		t.Errorf("backend stops = %v, want t-a", stops)
	}
	if store.agents[0].Status != agent.StatusIdle {
		t.Errorf("agent status = %s, want idle", store.agents[0].Status)
	}
	var released bool
	for _, ev := range hub.snapshot() {
		released = released || (ev.eventType == event.EventActiveWorkReleased && ev.tenant == scopeTenantA)
	}
	if !released {
		t.Error("no active work release announced in tenant A")
	}
}

func TestFailTasksNeverAccepted_Disabled(t *testing.T) {
	store, svc, _, _ := newUnacceptedEnv(t)
	store.unaccepted = []task.Task{{ID: "t-a", ProjectID: "p-a", TenantID: scopeTenantA, Status: task.StatusQueued}}
	if failed, err := svc.FailTasksNeverAccepted(context.Background(), 0); err != nil || failed != 0 {
		t.Fatalf("FailTasksNeverAccepted(0) = %d, %v; want 0, nil", failed, err)
	}
	if len(store.results) != 0 {
		t.Fatalf("results stored: %v", store.results)
	}
}

func TestHandleDeadLetteredTaskDispatch(t *testing.T) {
	dispatch := func(taskID, dispatchID string) []byte {
		data, err := json.Marshal(messagequeue.TaskAgentPayload{
			TaskID: taskID, ProjectID: "p-a", TenantID: scopeTenantA, AgentID: "agent-1", DispatchID: dispatchID, Backend: "aider",
		})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		return data
	}

	t.Run("the dead-lettered dispatch fails its task", func(t *testing.T) {
		store, svc, _, _ := newUnacceptedEnv(t)
		store.tasks = []task.Task{{ID: "t-a", ProjectID: "p-a", TenantID: scopeTenantA, AgentID: "agent-1", Status: task.StatusQueued, DispatchID: "d-1"}}
		if err := svc.HandleDeadLetteredTaskDispatch(context.Background(), dispatch("t-a", "d-1")); err != nil {
			t.Fatalf("HandleDeadLetteredTaskDispatch: %v", err)
		}
		got, ok := store.results["t-a"]
		if !ok || got.status != task.StatusFailed || !strings.Contains(got.result.Error, "dead-lettered") || got.tenant != scopeTenantA {
			t.Fatalf("t-a result = %+v (stored %v), want failed as dead-lettered in tenant A", got, ok)
		}
		if store.agents[0].Status != agent.StatusIdle {
			t.Errorf("agent status = %s, want idle", store.agents[0].Status)
		}
	})

	t.Run("a dispatch that is not the task's current one is ignored", func(t *testing.T) {
		store, svc, _, _ := newUnacceptedEnv(t)
		store.ended = map[string]bool{"t-a": true}
		if err := svc.HandleDeadLetteredTaskDispatch(context.Background(), dispatch("t-a", "d-old")); err != nil {
			t.Fatalf("HandleDeadLetteredTaskDispatch: %v", err)
		}
		if len(store.results) != 0 {
			t.Fatalf("results stored: %v", store.results)
		}
	})

	t.Run("an unreadable dispatch is ignored", func(t *testing.T) {
		store, svc, _, _ := newUnacceptedEnv(t)
		for _, data := range [][]byte{[]byte("not json"), []byte(`{"task_id": ""}`)} {
			if err := svc.HandleDeadLetteredTaskDispatch(context.Background(), data); err != nil {
				t.Fatalf("HandleDeadLetteredTaskDispatch(%s): %v", data, err)
			}
		}
		if len(store.results) != 0 {
			t.Fatalf("results stored: %v", store.results)
		}
	})
}

// TestTaskDeadLetterSubscriber: the Go Core reads every backend's
// dead-lettered dispatches.
func TestTaskDeadLetterSubscriber(t *testing.T) {
	queue := &mockQueue{}
	svc := NewAgentService(&unacceptedTaskStore{}, queue, &tenantRecorder{})
	cancel, err := svc.StartDeadLetterSubscriber(context.Background())
	if err != nil {
		t.Fatalf("StartDeadLetterSubscriber: %v", err)
	}
	defer cancel()
	if !slices.Contains(queue.subscribed(), "tasks.agent.*.dlq") {
		t.Fatalf("subscriptions = %v, want tasks.agent.*.dlq", queue.subscribed())
	}
}
