package plan_test

import (
	"slices"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/plan"
)

func TestBlockedSteps(t *testing.T) {
	tests := []struct {
		name  string
		steps []plan.Step
		want  []string
	}{
		{
			name: "dependency failed",
			steps: []plan.Step{
				{ID: "a", Status: plan.StepStatusFailed},
				{ID: "b", Status: plan.StepStatusPending, DependsOn: []string{"a"}},
			},
			want: []string{"b"},
		},
		{
			name: "dependency cancelled",
			steps: []plan.Step{
				{ID: "a", Status: plan.StepStatusCancelled},
				{ID: "b", Status: plan.StepStatusPending, DependsOn: []string{"a"}},
			},
			want: []string{"b"},
		},
		{
			name: "dependency skipped, transitively",
			steps: []plan.Step{
				{ID: "a", Status: plan.StepStatusFailed},
				{ID: "b", Status: plan.StepStatusPending, DependsOn: []string{"a"}},
				{ID: "c", Status: plan.StepStatusPending, DependsOn: []string{"b"}},
				{ID: "d", Status: plan.StepStatusPending, DependsOn: []string{"x", "c"}},
			},
			want: []string{"b", "c", "d"},
		},
		{
			name: "dependencies still open or completed",
			steps: []plan.Step{
				{ID: "a", Status: plan.StepStatusCompleted},
				{ID: "r", Status: plan.StepStatusRunning},
				{ID: "w", Status: plan.StepStatusWaitingApproval},
				{ID: "b", Status: plan.StepStatusPending, DependsOn: []string{"a"}},
				{ID: "c", Status: plan.StepStatusPending, DependsOn: []string{"r", "w"}},
			},
		},
		{
			name: "only pending steps are blocked",
			steps: []plan.Step{
				{ID: "a", Status: plan.StepStatusFailed},
				{ID: "b", Status: plan.StepStatusRunning, DependsOn: []string{"a"}},
				{ID: "c", Status: plan.StepStatusCompleted, DependsOn: []string{"a"}},
			},
		},
		{name: "no steps"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := plan.BlockedSteps(tc.steps)
			slices.Sort(got)
			if !slices.Equal(got, tc.want) {
				t.Errorf("BlockedSteps = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAnyUnsuccessful(t *testing.T) {
	tests := []struct {
		status plan.StepStatus
		want   bool
	}{
		{plan.StepStatusFailed, true},
		{plan.StepStatusCancelled, true},
		{plan.StepStatusSkipped, false},
		{plan.StepStatusCompleted, false},
		{plan.StepStatusRunning, false},
		{plan.StepStatusPending, false},
		{plan.StepStatusWaitingApproval, false},
	}
	for _, tc := range tests {
		steps := []plan.Step{{ID: "ok", Status: plan.StepStatusCompleted}, {ID: "x", Status: tc.status}}
		if got := plan.AnyUnsuccessful(steps); got != tc.want {
			t.Errorf("AnyUnsuccessful with a %s step = %v, want %v", tc.status, got, tc.want)
		}
	}
}
