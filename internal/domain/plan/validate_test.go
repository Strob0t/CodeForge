package plan_test

import (
	"errors"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/plan"
)

func validSequentialRequest() plan.CreatePlanRequest {
	return plan.CreatePlanRequest{
		Name:     "deploy pipeline",
		Protocol: plan.ProtocolSequential,
		Steps: []plan.CreateStepRequest{
			{TaskID: "t1", AgentID: "a1"},
			{TaskID: "t2", AgentID: "a2", DependsOn: []string{"0"}},
		},
	}
}

func TestValidate_ValidSequential(t *testing.T) {
	req := validSequentialRequest()
	if err := req.Validate(); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
}

func TestValidate_ValidParallel(t *testing.T) {
	req := plan.CreatePlanRequest{
		Name:     "parallel work",
		Protocol: plan.ProtocolParallel,
		Steps: []plan.CreateStepRequest{
			{TaskID: "t1", AgentID: "a1"},
			{TaskID: "t2", AgentID: "a2"},
			{TaskID: "t3", AgentID: "a3"},
		},
	}
	if err := req.Validate(); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
}

func TestValidate_ValidPingPong(t *testing.T) {
	req := plan.CreatePlanRequest{
		Name:     "review loop",
		Protocol: plan.ProtocolPingPong,
		Steps: []plan.CreateStepRequest{
			{TaskID: "t1", AgentID: "a1"},
			{TaskID: "t1", AgentID: "a2"},
		},
	}
	if err := req.Validate(); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
}

func TestValidate_ValidConsensus(t *testing.T) {
	req := plan.CreatePlanRequest{
		Name:     "consensus vote",
		Protocol: plan.ProtocolConsensus,
		Steps: []plan.CreateStepRequest{
			{TaskID: "t1", AgentID: "a1"},
			{TaskID: "t1", AgentID: "a2"},
			{TaskID: "t1", AgentID: "a3"},
		},
	}
	if err := req.Validate(); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
}

func TestValidate_MissingName(t *testing.T) {
	req := validSequentialRequest()
	req.Name = ""
	if err := req.Validate(); !errors.Is(err, plan.ErrNameRequired) {
		t.Fatalf("expected ErrNameRequired, got %v", err)
	}
}

func TestValidate_InvalidProtocol(t *testing.T) {
	req := validSequentialRequest()
	req.Protocol = "roundrobin"
	if err := req.Validate(); !errors.Is(err, plan.ErrInvalidProtocol) {
		t.Fatalf("expected ErrInvalidProtocol, got %v", err)
	}
}

func TestValidate_NoSteps(t *testing.T) {
	req := plan.CreatePlanRequest{
		Name:     "empty",
		Protocol: plan.ProtocolSequential,
		Steps:    nil,
	}
	if err := req.Validate(); !errors.Is(err, plan.ErrNoSteps) {
		t.Fatalf("expected ErrNoSteps, got %v", err)
	}
}

func TestValidate_PingPongWrongStepCount(t *testing.T) {
	req := plan.CreatePlanRequest{
		Name:     "bad ping pong",
		Protocol: plan.ProtocolPingPong,
		Steps: []plan.CreateStepRequest{
			{TaskID: "t1", AgentID: "a1"},
			{TaskID: "t1", AgentID: "a2"},
			{TaskID: "t1", AgentID: "a3"},
		},
	}
	if err := req.Validate(); !errors.Is(err, plan.ErrPingPongStepCount) {
		t.Fatalf("expected ErrPingPongStepCount, got %v", err)
	}
}

func TestValidate_ConsensusOneStep(t *testing.T) {
	req := plan.CreatePlanRequest{
		Name:     "one step consensus",
		Protocol: plan.ProtocolConsensus,
		Steps: []plan.CreateStepRequest{
			{TaskID: "t1", AgentID: "a1"},
		},
	}
	if err := req.Validate(); !errors.Is(err, plan.ErrConsensusStepCount) {
		t.Fatalf("expected ErrConsensusStepCount, got %v", err)
	}
}

func TestValidate_ConsensusDifferentTasks(t *testing.T) {
	req := plan.CreatePlanRequest{
		Name:     "mixed tasks consensus",
		Protocol: plan.ProtocolConsensus,
		Steps: []plan.CreateStepRequest{
			{TaskID: "t1", AgentID: "a1"},
			{TaskID: "t2", AgentID: "a2"},
		},
	}
	if err := req.Validate(); !errors.Is(err, plan.ErrConsensusSameTask) {
		t.Fatalf("expected ErrConsensusSameTask, got %v", err)
	}
}

func TestValidate_StepMissingTaskID(t *testing.T) {
	req := plan.CreatePlanRequest{
		Name:     "missing task",
		Protocol: plan.ProtocolSequential,
		Steps: []plan.CreateStepRequest{
			{TaskID: "", AgentID: "a1"},
		},
	}
	if err := req.Validate(); !errors.Is(err, plan.ErrStepMissingTask) {
		t.Fatalf("expected ErrStepMissingTask, got %v", err)
	}
}

func TestValidate_StepMissingAgentID(t *testing.T) {
	req := plan.CreatePlanRequest{
		Name:     "missing agent",
		Protocol: plan.ProtocolSequential,
		Steps: []plan.CreateStepRequest{
			{TaskID: "t1", AgentID: ""},
		},
	}
	if err := req.Validate(); !errors.Is(err, plan.ErrStepMissingAgent) {
		t.Fatalf("expected ErrStepMissingAgent, got %v", err)
	}
}

func TestValidate_DAGNoCycle(t *testing.T) {
	req := plan.CreatePlanRequest{
		Name:     "linear chain",
		Protocol: plan.ProtocolSequential,
		Steps: []plan.CreateStepRequest{
			{TaskID: "t1", AgentID: "a1"},
			{TaskID: "t2", AgentID: "a2", DependsOn: []string{"0"}},
			{TaskID: "t3", AgentID: "a3", DependsOn: []string{"1"}},
		},
	}
	if err := req.Validate(); err != nil {
		t.Fatalf("expected valid DAG, got %v", err)
	}
}

func TestValidate_DAGWithCycle(t *testing.T) {
	req := plan.CreatePlanRequest{
		Name:     "circular",
		Protocol: plan.ProtocolSequential,
		Steps: []plan.CreateStepRequest{
			{TaskID: "t1", AgentID: "a1", DependsOn: []string{"2"}},
			{TaskID: "t2", AgentID: "a2", DependsOn: []string{"0"}},
			{TaskID: "t3", AgentID: "a3", DependsOn: []string{"1"}},
		},
	}
	if err := req.Validate(); !errors.Is(err, plan.ErrDAGCycle) {
		t.Fatalf("expected ErrDAGCycle, got %v", err)
	}
}

func TestValidate_DAGSelfReference(t *testing.T) {
	req := plan.CreatePlanRequest{
		Name:     "self ref",
		Protocol: plan.ProtocolSequential,
		Steps: []plan.CreateStepRequest{
			{TaskID: "t1", AgentID: "a1", DependsOn: []string{"0"}},
		},
	}
	if err := req.Validate(); !errors.Is(err, plan.ErrDAGCycle) {
		t.Fatalf("expected ErrDAGCycle, got %v", err)
	}
}

func TestValidate_DAGInvalidRef(t *testing.T) {
	req := plan.CreatePlanRequest{
		Name:     "bad ref",
		Protocol: plan.ProtocolSequential,
		Steps: []plan.CreateStepRequest{
			{TaskID: "t1", AgentID: "a1", DependsOn: []string{"5"}},
		},
	}
	if err := req.Validate(); !errors.Is(err, plan.ErrDAGInvalidRef) {
		t.Fatalf("expected ErrDAGInvalidRef, got %v", err)
	}
}

func TestValidate_MaxParallelNegative(t *testing.T) {
	req := validSequentialRequest()
	req.MaxParallel = -1
	if err := req.Validate(); !errors.Is(err, plan.ErrMaxParallelNegative) {
		t.Fatalf("expected ErrMaxParallelNegative, got %v", err)
	}
}
