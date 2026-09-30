package plan

import "testing"

func TestStepStatusWaitingApprovalIsNotTerminal(t *testing.T) {
	if StepStatusWaitingApproval.IsTerminal() {
		t.Error("waiting_approval should not be a terminal status")
	}
}

func TestStepStatusWaitingApprovalValue(t *testing.T) {
	if StepStatusWaitingApproval != "waiting_approval" {
		t.Errorf("expected 'waiting_approval', got %q", StepStatusWaitingApproval)
	}
}

func TestStatusIsTerminal(t *testing.T) {
	tests := []struct {
		status Status
		want   bool
	}{
		{StatusPending, false},
		{StatusRunning, false},
		{StatusCompleted, true},
		{StatusFailed, true},
		{StatusCancelled, true},
		{"", false},
	}
	for _, tc := range tests {
		if got := tc.status.IsTerminal(); got != tc.want {
			t.Errorf("%q.IsTerminal() = %v, want %v", tc.status, got, tc.want)
		}
	}
}
