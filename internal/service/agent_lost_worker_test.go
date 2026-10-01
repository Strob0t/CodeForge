package service

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain"
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
	beats   []string        // tenant/task@dispatch of recorded heartbeats
	ended   map[string]bool // tasks whose dispatch ended on another path

	getAgentCalls int
}

func (s *lostTaskStore) GetAgent(ctx context.Context, id string) (*agent.Agent, error) {
	s.mu.Lock()
	s.getAgentCalls++
	s.mu.Unlock()
	return s.mockStore.GetAgent(ctx, id)
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

// EndTaskDispatch refuses the tasks in ended, like the store's status and
// dispatch predicate, and records the others' result.
func (s *lostTaskStore) EndTaskDispatch(ctx context.Context, id, _ string, status task.Status, result task.Result) error {
	s.mu.Lock()
	refused := s.ended[id]
	s.mu.Unlock()
	if refused {
		return fmt.Errorf("task %s: %w", id, domain.ErrConflict)
	}
	return s.UpdateTaskResult(ctx, id, status, result, 0)
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

func (s *lostTaskStore) TouchTaskHeartbeat(ctx context.Context, id, dispatchID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.beats = append(s.beats, tenantctx.FromContext(ctx)+"/"+id+"@"+dispatchID)
	return nil
}

// taskCancels returns the tasks.cancel messages published, as tenant/task.
func taskCancels(t *testing.T, q *mockQueue) []string {
	t.Helper()
	var cancels []string
	for _, m := range q.published {
		if m.subject != messagequeue.SubjectTaskCancel {
			continue
		}
		var p messagequeue.TaskCancelPayload
		if err := json.Unmarshal(m.data, &p); err != nil {
			t.Fatalf("tasks.cancel payload %s: %v", m.data, err)
		}
		cancels = append(cancels, p.TenantID+"/"+p.TaskID)
	}
	return cancels
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

	// S2-G fix, 8: the worker is told to stop with a tasks.cancel for the
	// task in its tenant, which every worker listens to; the task's agent is
	// not looked up, so a task without agent (or whose agent is gone) is
	// stopped too, and the backend's own Stop is not involved.
	if got := taskCancels(t, queue); !slices.Equal(got, []string{scopeTenantA + "/t-a", scopeTenantB + "/t-b"}) {
		t.Errorf("tasks.cancel = %v, want t-a in tenant A and t-b in tenant B", got)
	}
	if got := probe.stops(); len(got) != 0 {
		t.Errorf("backend stops = %v, want none", got)
	}
	if store.getAgentCalls != 0 {
		t.Errorf("GetAgent called %d times to tell the worker to stop, want 0", store.getAgentCalls)
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
	if err := svc.HandleTaskHeartbeat(context.Background(), &messagequeue.TaskHeartbeatPayload{TaskID: "t-a", TenantID: scopeTenantA, DispatchID: "d-1"}); err != nil {
		t.Fatalf("HandleTaskHeartbeat: %v", err)
	}
	// The heartbeat counts for the dispatch it names (S2-F review, F7).
	if len(store.beats) != 1 || store.beats[0] != scopeTenantA+"/t-a@d-1" {
		t.Fatalf("heartbeats = %v, want t-a's dispatch d-1 in tenant A", store.beats)
	}
}

// TestFailTasksWithLostWorker_KeepsAResultThatArrivedMeanwhile (S2-F review,
// F9): the watchdog's failed result overwrote whatever the task had by then,
// also the worker's own result that arrived after the task was listed, or a
// later dispatch. Its write is refused then: the task is skipped, its worker
// is not told to stop (it may run the later dispatch) and nothing is
// announced.
func TestFailTasksWithLostWorker_KeepsAResultThatArrivedMeanwhile(t *testing.T) {
	probe := registerExecutionProbe(t)
	store := &lostTaskStore{ended: map[string]bool{"t-a": true}}
	store.agents = []agent.Agent{{ID: "agent-1", ProjectID: "p-a", Name: "a", Backend: "execution-probe", Status: agent.StatusIdle}}
	store.tasks = []task.Task{{ID: "t-a", ProjectID: "p-a", TenantID: scopeTenantA, AgentID: "agent-1", Status: task.StatusQueued, DispatchID: "d-1"}}
	store.lost = store.tasks
	hub := &tenantRecorder{}
	svc := NewAgentService(store, &mockQueue{}, hub)

	failed, err := svc.FailTasksWithLostWorker(context.Background(), 3*time.Minute)
	if err != nil || failed != 0 {
		t.Fatalf("FailTasksWithLostWorker = %d, %v; want 0, nil", failed, err)
	}
	if len(store.results) != 0 {
		t.Errorf("results stored: %v", store.results)
	}
	if got := probe.stops(); len(got) != 0 {
		t.Errorf("backend stops = %v, want none", got)
	}
	if events := hub.snapshot(); len(events) != 0 {
		t.Errorf("events = %v, want none", events)
	}
	if store.agents[0].Status != agent.StatusIdle {
		t.Errorf("agent status = %s, want unchanged idle", store.agents[0].Status)
	}
}
