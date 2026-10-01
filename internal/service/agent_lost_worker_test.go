package service

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/agent"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// KI-65: backend tasks (tasks.agent.*) are acked on accept; a task whose
// worker died used to be reset to pending after 30 minutes without a row
// update (a healthy long task too, its agent left "running"). The worker now
// sends a heartbeat every 30 s while it executes a task, and the watchdog
// fails the tasks whose heartbeats stopped through the task result path and
// resets their agent.

// lostTaskStore records task results and heartbeats and lists lost tasks.
type lostTaskStore struct {
	mockStore
	mu      sync.Mutex
	lost    []task.Task
	idleFor time.Duration
	results map[string]lostTaskResult
	beats   []string // tenant/task of recorded heartbeats
}

type lostTaskResult struct {
	status task.Status
	result task.Result
	tenant string
}

func (s *lostTaskStore) ListTasksWithStaleHeartbeat(_ context.Context, idleFor time.Duration, _ int) ([]task.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.idleFor = idleFor
	return s.lost, nil
}

func (s *lostTaskStore) UpdateTaskResult(ctx context.Context, id string, status task.Status, result task.Result, _ float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.results == nil {
		s.results = map[string]lostTaskResult{}
	}
	s.results[id] = lostTaskResult{status: status, result: result, tenant: tenantctx.FromContext(ctx)}
	return nil
}

func (s *lostTaskStore) TouchTaskHeartbeat(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.beats = append(s.beats, tenantctx.FromContext(ctx)+"/"+id)
	return nil
}

func TestFailTasksWithLostWorker(t *testing.T) {
	probe := registerExecutionProbe(t)
	store := &lostTaskStore{}
	store.agents = []agent.Agent{{ID: "agent-1", ProjectID: "p-a", Name: "a", Backend: "execution-probe", Status: agent.StatusRunning}}
	store.tasks = []task.Task{
		{ID: "t-a", ProjectID: "p-a", TenantID: scopeTenantA, AgentID: "agent-1", Status: task.StatusRunning},
		{ID: "t-b", ProjectID: "p-b", TenantID: scopeTenantB, Status: task.StatusQueued},
	}
	store.lost = store.tasks
	queue := &mockQueue{}
	hub := &tenantRecorder{}
	svc := NewAgentService(store, queue, hub)

	failed, err := svc.FailTasksWithLostWorker(context.Background(), 3*time.Minute)
	if err != nil {
		t.Fatalf("FailTasksWithLostWorker: %v", err)
	}
	if failed != 2 {
		t.Fatalf("failed %d tasks, want 2", failed)
	}
	if store.idleFor != 3*time.Minute {
		t.Errorf("listed tasks idle for %s, want 3m", store.idleFor)
	}
	for id, tenant := range map[string]string{"t-a": scopeTenantA, "t-b": scopeTenantB} {
		got, ok := store.results[id]
		if !ok {
			t.Fatalf("task %s: no result stored", id)
		}
		if got.status != task.StatusFailed || !strings.Contains(got.result.Error, "heartbeat") {
			t.Errorf("task %s result = %s %q, want failed for a lost heartbeat", id, got.status, got.result.Error)
		}
		if got.tenant != tenant {
			t.Errorf("task %s failed in tenant %q, want %q", id, got.tenant, tenant)
		}
	}
	if store.agents[0].Status != agent.StatusIdle {
		t.Errorf("agent status = %s, want idle", store.agents[0].Status)
	}

	// The worker is told to stop through the agent's backend, as StopTask
	// does; a task without agent cannot be routed to one.
	if got := probe.stops(); !slices.Equal(got, []string{"t-a"}) {
		t.Errorf("backend stops = %v, want t-a", got)
	}

	byTenant := map[string][]string{}
	for _, ev := range hub.snapshot() {
		byTenant[ev.tenant] = append(byTenant[ev.tenant], ev.eventType)
	}
	if len(byTenant) != 2 {
		t.Fatalf("events by tenant = %v, want events for tenant A and B only", byTenant)
	}
	if !containsEvent(byTenant[scopeTenantA], event.EventAgentStatus) || !containsEvent(byTenant[scopeTenantA], event.EventTaskStatus) {
		t.Errorf("tenant A events = %v, want the task status and the agent's status", byTenant[scopeTenantA])
	}
	if !containsEvent(byTenant[scopeTenantB], event.EventActiveWorkReleased) {
		t.Errorf("tenant B events = %v, want the active work release", byTenant[scopeTenantB])
	}
}

func containsEvent(events []string, want string) bool {
	for _, ev := range events {
		if ev == want {
			return true
		}
	}
	return false
}

func TestFailTasksWithLostWorker_DisabledWithoutThreshold(t *testing.T) {
	store := &lostTaskStore{lost: []task.Task{{ID: "t-a", ProjectID: "p-a", Status: task.StatusRunning}}}
	svc := NewAgentService(store, &mockQueue{}, &tenantRecorder{})
	if failed, err := svc.FailTasksWithLostWorker(context.Background(), 0); err != nil || failed != 0 {
		t.Fatalf("FailTasksWithLostWorker = %d, %v; want 0, nil", failed, err)
	}
	if len(store.results) != 0 {
		t.Fatalf("results stored: %v", store.results)
	}
}

func TestHandleTaskHeartbeat_RecordsInThePayloadsTenant(t *testing.T) {
	store := &lostTaskStore{}
	svc := NewAgentService(store, &mockQueue{}, &tenantRecorder{})
	if err := svc.HandleTaskHeartbeat(context.Background(), &messagequeue.TaskHeartbeatPayload{TaskID: "t-a", TenantID: scopeTenantA}); err != nil {
		t.Fatalf("HandleTaskHeartbeat: %v", err)
	}
	if len(store.beats) != 1 || store.beats[0] != scopeTenantA+"/t-a" {
		t.Fatalf("heartbeats = %v, want t-a in tenant A", store.beats)
	}
}
