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
