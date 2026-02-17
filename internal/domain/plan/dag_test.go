package plan_test

import (
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/plan"
)

func TestReadySteps_AllPendingNoDeps(t *testing.T) {
	steps := []plan.Step{
		{ID: "s1", Status: plan.StepStatusPending},
		{ID: "s2", Status: plan.StepStatusPending},
	}
	ready := plan.ReadySteps(steps)
	if len(ready) != 2 {
		t.Fatalf("expected 2 ready, got %d", len(ready))
	}
}

func TestReadySteps_WithDeps(t *testing.T) {
	steps := []plan.Step{
		{ID: "s1", Status: plan.StepStatusCompleted},
		{ID: "s2", Status: plan.StepStatusPending, DependsOn: []string{"s1"}},
		{ID: "s3", Status: plan.StepStatusPending, DependsOn: []string{"s2"}},
	}
	ready := plan.ReadySteps(steps)
	if len(ready) != 1 || ready[0] != "s2" {
		t.Fatalf("expected [s2], got %v", ready)
	}
}

func TestReadySteps_NoneReady(t *testing.T) {
	steps := []plan.Step{
		{ID: "s1", Status: plan.StepStatusRunning},
		{ID: "s2", Status: plan.StepStatusPending, DependsOn: []string{"s1"}},
	}
	ready := plan.ReadySteps(steps)
	if len(ready) != 0 {
		t.Fatalf("expected 0 ready, got %d", len(ready))
	}
}

func TestRunningCount(t *testing.T) {
	steps := []plan.Step{
		{ID: "s1", Status: plan.StepStatusRunning},
		{ID: "s2", Status: plan.StepStatusRunning},
		{ID: "s3", Status: plan.StepStatusCompleted},
		{ID: "s4", Status: plan.StepStatusPending},
	}
	if count := plan.RunningCount(steps); count != 2 {
		t.Fatalf("expected 2 running, got %d", count)
	}
}

func TestAllTerminal(t *testing.T) {
	steps := []plan.Step{
		{ID: "s1", Status: plan.StepStatusCompleted},
		{ID: "s2", Status: plan.StepStatusFailed},
		{ID: "s3", Status: plan.StepStatusSkipped},
	}
	if !plan.AllTerminal(steps) {
		t.Fatal("expected all terminal")
	}
}

func TestAllTerminal_NotYet(t *testing.T) {
	steps := []plan.Step{
		{ID: "s1", Status: plan.StepStatusCompleted},
		{ID: "s2", Status: plan.StepStatusRunning},
	}
	if plan.AllTerminal(steps) {
		t.Fatal("expected not all terminal")
	}
}

func TestAnyFailed(t *testing.T) {
	steps := []plan.Step{
		{ID: "s1", Status: plan.StepStatusCompleted},
		{ID: "s2", Status: plan.StepStatusFailed},
	}
	if !plan.AnyFailed(steps) {
		t.Fatal("expected any failed")
	}
}

func TestAnyFailed_AllGood(t *testing.T) {
	steps := []plan.Step{
		{ID: "s1", Status: plan.StepStatusCompleted},
		{ID: "s2", Status: plan.StepStatusCompleted},
	}
	if plan.AnyFailed(steps) {
		t.Fatal("expected no failures")
	}
}
