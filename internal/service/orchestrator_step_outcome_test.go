package service_test

import (
	"context"
	"sync"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/service"
)

// A step that ends without completing (failed or cancelled) makes the steps
// that depend on it skipped, and a plan with such a step ends failed; it
// neither completes nor stays running forever (review finding 3).

func createPlan(t *testing.T, orchSvc *service.OrchestratorService, protocol plan.Protocol, maxParallel int, steps []plan.CreateStepRequest) *plan.ExecutionPlan {
	t.Helper()
	ctx := context.Background()
	p, err := orchSvc.CreatePlan(ctx, &plan.CreatePlanRequest{
		Name: "outcome", ProjectID: "proj-1", Protocol: protocol, MaxParallel: maxParallel, Steps: steps,
	})
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	if _, err := orchSvc.StartPlan(ctx, p.ID); err != nil {
		t.Fatalf("StartPlan: %v", err)
	}
	return p
}

func planState(t *testing.T, store *orchMockStore, planID string) *plan.ExecutionPlan {
	t.Helper()
	p, err := store.GetPlan(context.Background(), planID)
	if err != nil {
		t.Fatalf("GetPlan: %v", err)
	}
	return p
}

// stepEnd ends the run of a plan step (by index) with a status.
type stepEnd struct {
	step   int
	status run.Status
}

// endStepRun ends the run of step i the way the runtime reports it.
func endStepRun(t *testing.T, store *orchMockStore, rt *service.RuntimeService, orchSvc *service.OrchestratorService, planID string, i int, status run.Status) {
	t.Helper()
	ctx := context.Background()
	step := planState(t, store, planID).Steps[i]
	if step.RunID == "" || step.Status != plan.StepStatusRunning {
		t.Fatalf("step %d is %s without a run", i, step.Status)
	}
	if status == run.StatusCancelled {
		if err := rt.CancelRun(ctx, step.RunID); err != nil {
			t.Fatalf("CancelRun: %v", err)
		}
		return
	}
	if err := store.CompleteRun(ctx, &run.CompletionRequest{ID: step.RunID, Status: status}); err != nil {
		t.Fatalf("CompleteRun: %v", err)
	}
	orchSvc.HandleRunCompleted(ctx, step.RunID, status)
}

func TestPlan_UnsuccessfulStepEndsThePlan(t *testing.T) {
	tests := []struct {
		name      string
		protocol  plan.Protocol
		steps     []plan.CreateStepRequest
		ends      []stepEnd // in order
		wantSteps []plan.StepStatus
		wantPlan  plan.Status
	}{
		{
			name:      "sequential: a cancelled step fails the plan",
			protocol:  plan.ProtocolSequential,
			steps:     []plan.CreateStepRequest{{TaskID: "t1", AgentID: "a1"}, {TaskID: "t2", AgentID: "a2"}},
			ends:      []stepEnd{{0, run.StatusCancelled}},
			wantSteps: []plan.StepStatus{plan.StepStatusCancelled, plan.StepStatusSkipped},
			wantPlan:  plan.StatusFailed,
		},
		{
			name:      "sequential: the last step cancelled does not complete the plan",
			protocol:  plan.ProtocolSequential,
			steps:     []plan.CreateStepRequest{{TaskID: "t1", AgentID: "a1"}},
			ends:      []stepEnd{{0, run.StatusCancelled}},
			wantSteps: []plan.StepStatus{plan.StepStatusCancelled},
			wantPlan:  plan.StatusFailed,
		},
		{
			name:     "parallel: dependents of a failed step are skipped",
			protocol: plan.ProtocolParallel,
			steps: []plan.CreateStepRequest{
				{TaskID: "t1", AgentID: "a1"},
				{TaskID: "t2", AgentID: "a2", DependsOn: []string{"0"}},
				{TaskID: "t3", AgentID: "a3"},
			},
			ends:      []stepEnd{{0, run.StatusFailed}, {2, run.StatusCompleted}},
			wantSteps: []plan.StepStatus{plan.StepStatusFailed, plan.StepStatusSkipped, plan.StepStatusCompleted},
			wantPlan:  plan.StatusFailed,
		},
		{
			name:     "parallel: dependents of a cancelled step are skipped",
			protocol: plan.ProtocolParallel,
			steps: []plan.CreateStepRequest{
				{TaskID: "t1", AgentID: "a1"},
				{TaskID: "t2", AgentID: "a2", DependsOn: []string{"0"}},
				{TaskID: "t3", AgentID: "a3"},
			},
			ends:      []stepEnd{{2, run.StatusCompleted}, {0, run.StatusCancelled}},
			wantSteps: []plan.StepStatus{plan.StepStatusCancelled, plan.StepStatusSkipped, plan.StepStatusCompleted},
			wantPlan:  plan.StatusFailed,
		},
		{
			name:     "parallel: completed dependencies still run their dependents",
			protocol: plan.ProtocolParallel,
			steps: []plan.CreateStepRequest{
				{TaskID: "t1", AgentID: "a1"},
				{TaskID: "t2", AgentID: "a2", DependsOn: []string{"0"}},
			},
			ends:      []stepEnd{{0, run.StatusCompleted}, {1, run.StatusCompleted}},
			wantSteps: []plan.StepStatus{plan.StepStatusCompleted, plan.StepStatusCompleted},
			wantPlan:  plan.StatusCompleted,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store, orchSvc, rt := newOrchRuntimeSetup()
			p := createPlan(t, orchSvc, tc.protocol, 0, tc.steps)
			for _, end := range tc.ends {
				endStepRun(t, store, rt, orchSvc, p.ID, end.step, end.status)
			}

			got := planState(t, store, p.ID)
			for i, want := range tc.wantSteps {
				if got.Steps[i].Status != want {
					t.Errorf("step %d = %s, want %s", i, got.Steps[i].Status, want)
				}
				if want == plan.StepStatusSkipped && got.Steps[i].RunID != "" {
					t.Errorf("skipped step %d has run %s", i, got.Steps[i].RunID)
				}
			}
			if got.Status != tc.wantPlan {
				t.Errorf("plan = %s, want %s", got.Status, tc.wantPlan)
			}
		})
	}
}

// cancelOnReadStore cancels a plan right after a read returned it as running:
// the plan's status changes between the read and the decision made from it.
type cancelOnReadStore struct {
	*orchMockStore
	mu     sync.Mutex
	armed  string // plan ID to cancel after its next read
	onRead func()
}

func (s *cancelOnReadStore) GetPlan(ctx context.Context, id string) (*plan.ExecutionPlan, error) {
	p, err := s.orchMockStore.GetPlan(ctx, id)
	s.mu.Lock()
	fire := s.armed == id
	if fire {
		s.armed = ""
	}
	s.mu.Unlock()
	if fire {
		s.onRead()
	}
	return p, err
}

// TestAdvancePlan_NoStepStartsAfterCancel: a plan cancelled after the run
// completion read it as running does not start its next step (finding 10).
func TestAdvancePlan_NoStepStartsAfterCancel(t *testing.T) {
	base := newOrchStore()
	store := &cancelOnReadStore{orchMockStore: base}
	orchSvc, _ := newOrchRuntimeSetupWithStore(store)
	ctx := context.Background()
	p := createPlan(t, orchSvc, plan.ProtocolSequential, 0, []plan.CreateStepRequest{
		{TaskID: "t1", AgentID: "a1"}, {TaskID: "t2", AgentID: "a2"},
	})
	first := planState(t, base, p.ID).Steps[0]
	if err := base.CompleteRun(ctx, &run.CompletionRequest{ID: first.RunID, Status: run.StatusCompleted}); err != nil {
		t.Fatalf("CompleteRun: %v", err)
	}

	// CancelPlan's status write lands between the completion's read and advance.
	store.onRead = func() {
		if err := base.UpdatePlanStatus(ctx, p.ID, plan.StatusCancelled); err != nil {
			t.Errorf("cancel plan: %v", err)
		}
	}
	store.mu.Lock()
	store.armed = p.ID
	store.mu.Unlock()
	orchSvc.HandleRunCompleted(ctx, first.RunID, run.StatusCompleted)

	second := planState(t, base, p.ID).Steps[1]
	if second.Status == plan.StepStatusRunning || second.RunID != "" {
		t.Errorf("second step = %s with run %q, want no start in a cancelled plan", second.Status, second.RunID)
	}
}

// TestCancelPlan_ConcurrentWithCompletions: whatever the interleaving of a
// cancel and a step completion, no step of the cancelled plan keeps running.
func TestCancelPlan_ConcurrentWithCompletions(t *testing.T) {
	for range 30 {
		store, orchSvc, _ := newOrchRuntimeSetup()
		ctx := context.Background()
		p := createPlan(t, orchSvc, plan.ProtocolSequential, 0, []plan.CreateStepRequest{
			{TaskID: "t1", AgentID: "a1"}, {TaskID: "t2", AgentID: "a2"},
		})
		first := planState(t, store, p.ID).Steps[0]
		if err := store.CompleteRun(ctx, &run.CompletionRequest{ID: first.RunID, Status: run.StatusCompleted}); err != nil {
			t.Fatalf("CompleteRun: %v", err)
		}

		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); orchSvc.HandleRunCompleted(ctx, first.RunID, run.StatusCompleted) }()
		go func() {
			defer wg.Done()
			if err := orchSvc.CancelPlan(ctx, p.ID); err != nil {
				t.Errorf("CancelPlan: %v", err)
			}
		}()
		wg.Wait()

		got := planState(t, store, p.ID)
		if got.Status != plan.StatusCancelled {
			t.Fatalf("plan = %s, want cancelled", got.Status)
		}
		for i, st := range got.Steps {
			if st.Status == plan.StepStatusRunning || st.Status == plan.StepStatusPending {
				t.Fatalf("step %d is %s in a cancelled plan", i, st.Status)
			}
			if st.RunID == "" {
				continue
			}
			if r, _ := store.GetRun(ctx, st.RunID); !r.Status.IsTerminal() {
				t.Fatalf("run of step %d is %s in a cancelled plan", i, r.Status)
			}
		}
	}
}
