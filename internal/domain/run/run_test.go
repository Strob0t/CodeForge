package run_test

import (
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/run"
)

func TestRunValidate_Valid(t *testing.T) {
	r := &run.Run{
		TaskID:    "task-1",
		AgentID:   "agent-1",
		ProjectID: "proj-1",
		Status:    run.StatusRunning,
		ExecMode:  run.ExecModeMount,
	}
	if err := r.Validate(); err != nil {
		t.Fatalf("expected valid, got: %v", err)
	}
}

func TestRunValidate_MissingTaskID(t *testing.T) {
	r := &run.Run{AgentID: "a", ProjectID: "p"}
	if err := r.Validate(); err == nil {
		t.Fatal("expected error for missing task_id")
	}
}

func TestRunValidate_MissingAgentID(t *testing.T) {
	r := &run.Run{TaskID: "t", ProjectID: "p"}
	if err := r.Validate(); err == nil {
		t.Fatal("expected error for missing agent_id")
	}
}

func TestRunValidate_MissingProjectID(t *testing.T) {
	r := &run.Run{TaskID: "t", AgentID: "a"}
	if err := r.Validate(); err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

func TestRunValidate_InvalidStatus(t *testing.T) {
	r := &run.Run{
		TaskID:    "t",
		AgentID:   "a",
		ProjectID: "p",
		Status:    "invalid",
	}
	if err := r.Validate(); err == nil {
		t.Fatal("expected error for invalid status")
	}
}

func TestRunValidate_InvalidExecMode(t *testing.T) {
	r := &run.Run{
		TaskID:    "t",
		AgentID:   "a",
		ProjectID: "p",
		ExecMode:  "hybrid",
	}
	if err := r.Validate(); err == nil {
		t.Fatal("expected error for invalid exec_mode")
	}
}

func TestRunValidate_NegativeStepCount(t *testing.T) {
	r := &run.Run{
		TaskID:    "t",
		AgentID:   "a",
		ProjectID: "p",
		StepCount: -1,
	}
	if err := r.Validate(); err == nil {
		t.Fatal("expected error for negative step_count")
	}
}

func TestRunValidate_NegativeCost(t *testing.T) {
	r := &run.Run{
		TaskID:    "t",
		AgentID:   "a",
		ProjectID: "p",
		CostUSD:   -0.5,
	}
	if err := r.Validate(); err == nil {
		t.Fatal("expected error for negative cost_usd")
	}
}

func TestRunValidate_EmptyStatusAndMode(t *testing.T) {
	r := &run.Run{
		TaskID:    "t",
		AgentID:   "a",
		ProjectID: "p",
	}
	if err := r.Validate(); err != nil {
		t.Fatalf("empty status/mode should be valid, got: %v", err)
	}
}

func TestStartRequestValidate_Valid(t *testing.T) {
	req := &run.StartRequest{
		TaskID:    "task-1",
		AgentID:   "agent-1",
		ProjectID: "proj-1",
		ExecMode:  run.ExecModeSandbox,
	}
	if err := req.Validate(); err != nil {
		t.Fatalf("expected valid, got: %v", err)
	}
}

func TestStartRequestValidate_MissingFields(t *testing.T) {
	tests := []struct {
		name string
		req  run.StartRequest
	}{
		{"missing task_id", run.StartRequest{AgentID: "a", ProjectID: "p"}},
		{"missing agent_id", run.StartRequest{TaskID: "t", ProjectID: "p"}},
		{"missing project_id", run.StartRequest{TaskID: "t", AgentID: "a"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.req.Validate(); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestStartRequestValidate_InvalidExecMode(t *testing.T) {
	req := &run.StartRequest{
		TaskID:    "t",
		AgentID:   "a",
		ProjectID: "p",
		ExecMode:  "invalid",
	}
	if err := req.Validate(); err == nil {
		t.Fatal("expected error for invalid exec_mode")
	}
}

func TestStartRequestValidate_EmptyExecMode(t *testing.T) {
	req := &run.StartRequest{
		TaskID:    "t",
		AgentID:   "a",
		ProjectID: "p",
	}
	if err := req.Validate(); err != nil {
		t.Fatalf("empty exec_mode should be valid, got: %v", err)
	}
}

func TestAllStatuses(t *testing.T) {
	statuses := []run.Status{
		run.StatusPending,
		run.StatusRunning,
		run.StatusCompleted,
		run.StatusFailed,
		run.StatusCancelled,
		run.StatusTimeout,
	}
	for _, s := range statuses {
		r := &run.Run{TaskID: "t", AgentID: "a", ProjectID: "p", Status: s}
		if err := r.Validate(); err != nil {
			t.Errorf("status %q should be valid: %v", s, err)
		}
	}
}

func TestAllExecModes(t *testing.T) {
	modes := []run.ExecMode{run.ExecModeMount, run.ExecModeSandbox}
	for _, m := range modes {
		r := &run.Run{TaskID: "t", AgentID: "a", ProjectID: "p", ExecMode: m}
		if err := r.Validate(); err != nil {
			t.Errorf("exec_mode %q should be valid: %v", m, err)
		}
	}
}
