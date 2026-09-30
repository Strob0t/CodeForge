package service_test

import (
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/domain/run"
)

// A step whose run cannot be started ends failed, and the plan is decided
// again right away: it does not stay running with nothing left to run
// (review 2, finding 4). "a-missing" is an agent the store does not know, so
// starting its run fails.

func TestPlan_StepThatCannotStartEndsThePlan(t *testing.T) {
	tests := []struct {
		name      string
		protocol  plan.Protocol
		steps     []plan.CreateStepRequest
		ends      []stepEnd
		wantSteps []plan.StepStatus
		wantPlan  plan.Status
	}{
		{
			name:      "sequential: the first step cannot start",
			protocol:  plan.ProtocolSequential,
			steps:     []plan.CreateStepRequest{{TaskID: "t1", AgentID: "a-missing"}, {TaskID: "t2", AgentID: "a2"}},
			wantSteps: []plan.StepStatus{plan.StepStatusFailed, plan.StepStatusSkipped},
			wantPlan:  plan.StatusFailed,
		},
		{
			name:      "sequential: the next step cannot start",
			protocol:  plan.ProtocolSequential,
			steps:     []plan.CreateStepRequest{{TaskID: "t1", AgentID: "a1"}, {TaskID: "t2", AgentID: "a-missing"}},
			ends:      []stepEnd{{0, run.StatusCompleted}},
			wantSteps: []plan.StepStatus{plan.StepStatusCompleted, plan.StepStatusFailed},
			wantPlan:  plan.StatusFailed,
		},
		{
			name:      "ping-pong: a round cannot start",
			protocol:  plan.ProtocolPingPong,
			steps:     []plan.CreateStepRequest{{TaskID: "t1", AgentID: "a1"}, {TaskID: "t2", AgentID: "a-missing"}},
			ends:      []stepEnd{{0, run.StatusCompleted}},
			wantSteps: []plan.StepStatus{plan.StepStatusCompleted, plan.StepStatusFailed},
			wantPlan:  plan.StatusFailed,
		},
		{
			name:     "parallel: the only other step cannot start",
			protocol: plan.ProtocolParallel,
			steps: []plan.CreateStepRequest{
				{TaskID: "t1", AgentID: "a1"},
				{TaskID: "t2", AgentID: "a-missing", DependsOn: []string{"0"}},
			},
			ends:      []stepEnd{{0, run.StatusCompleted}},
			wantSteps: []plan.StepStatus{plan.StepStatusCompleted, plan.StepStatusFailed},
			wantPlan:  plan.StatusFailed,
		},
		{
			name:      "consensus: no step can start",
			protocol:  plan.ProtocolConsensus,
			steps:     []plan.CreateStepRequest{{TaskID: "t1", AgentID: "a-missing"}, {TaskID: "t1", AgentID: "a-missing"}},
			wantSteps: []plan.StepStatus{plan.StepStatusFailed, plan.StepStatusFailed},
			wantPlan:  plan.StatusFailed,
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
					t.Errorf("step %d = %s (%s), want %s", i, got.Steps[i].Status, got.Steps[i].Error, want)
				}
			}
			if got.Status != tc.wantPlan {
				t.Errorf("plan = %s, want %s", got.Status, tc.wantPlan)
			}
		})
	}
}
