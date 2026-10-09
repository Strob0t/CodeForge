package service_test

import (
	"context"
	"testing"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/service"
)

// replicaRaceStore lets the other Go Core replica act on the step between
// this replica's read of it and its re-plan: both took the duplicated
// completion of the stalled run (S7-F review).
type replicaRaceStore struct {
	*orchMockStore
	otherReplica func()
}

func (s *replicaRaceStore) ReplanStalledStep(ctx context.Context, stepID, runID string, maxReplans int) (plan.ReplanOutcome, error) {
	if s.otherReplica != nil {
		s.otherReplica()
		s.otherReplica = nil
	}
	return s.orchMockStore.ReplanStalledStep(ctx, stepID, runID, maxReplans)
}

// TestStallReplan_OtherReplicaReplannedTheStep: the completion of a stalled
// run reaches two replicas. One re-plans the step (it is pending, then runs
// a new run); the other's re-plan finds the step no longer running the
// stalled run and leaves it alone instead of marking it failed.
func TestStallReplan_OtherReplicaReplannedTheStep(t *testing.T) {
	tests := []struct {
		name string
		// moved is what the other replica did to the step meanwhile.
		moved      func(st *plan.Step)
		wantStatus plan.StepStatus
		wantRun    string // "" keeps the stalled run
	}{
		{"pending again", func(st *plan.Step) { st.Status = plan.StepStatusPending }, plan.StepStatusPending, ""},
		{"running a new run", func(st *plan.Step) { st.Status, st.RunID = plan.StepStatusRunning, "run-of-the-other-replica" },
			plan.StepStatusRunning, "run-of-the-other-replica"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newOrchStore()
			racing := &replicaRaceStore{orchMockStore: store}
			bc := &runtimeMockBroadcaster{}
			es := &runtimeMockEventStore{}
			runtimeSvc := service.NewRuntimeService(store, &runtimeMockQueue{}, bc, es,
				service.NewPolicyService("headless-safe-sandbox", nil), &config.Runtime{StallThreshold: 5, StallMaxRetries: 2})
			orchSvc := service.NewOrchestratorService(racing, bc, es, runtimeSvc, &config.Orchestrator{MaxParallel: 4, PingPongMaxRounds: 3})
			runtimeSvc.SetOnRunComplete(orchSvc.HandleRunCompleted)
			p := createPlan(t, orchSvc, plan.ProtocolSequential, 0, []plan.CreateStepRequest{{TaskID: "t1", AgentID: "a1"}})

			stalledRun := planState(t, store, p.ID).Steps[0].RunID
			stepID := planState(t, store, p.ID).Steps[0].ID
			racing.otherReplica = func() {
				store.mu.Lock()
				defer store.mu.Unlock()
				for i := range store.steps {
					if store.steps[i].ID == stepID {
						tt.moved(&store.steps[i])
					}
				}
			}
			endFirstRun(t, store, orchSvc, p.ID, run.StatusFailed, run.StallDetectedError)

			got := planState(t, store, p.ID)
			wantRun := tt.wantRun
			if wantRun == "" {
				wantRun = stalledRun
			}
			if st := got.Steps[0]; st.Status != tt.wantStatus || st.RunID != wantRun {
				t.Fatalf("step = %s with run %s, want %s with run %s: the other replica's re-plan was undone", st.Status, st.RunID, tt.wantStatus, wantRun)
			}
			if got.Status != plan.StatusRunning {
				t.Fatalf("plan = %s, want running", got.Status)
			}
		})
	}
}
