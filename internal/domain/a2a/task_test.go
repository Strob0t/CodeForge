package a2a

import "testing"

func TestA2ATask_Valid(t *testing.T) {
	task := NewA2ATask("task-1")
	if err := task.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestA2ATask_MissingID(t *testing.T) {
	task := NewA2ATask("")
	if err := task.Validate(); err == nil {
		t.Fatal("expected error for empty ID")
	}
}

func TestA2ATask_InvalidState(t *testing.T) {
	task := NewA2ATask("task-1")
	task.State = "bogus"
	if err := task.Validate(); err == nil {
		t.Fatal("expected error for invalid state")
	}
}

func TestA2ATask_InvalidDirection(t *testing.T) {
	task := NewA2ATask("task-1")
	task.Direction = "sideways"
	if err := task.Validate(); err == nil {
		t.Fatal("expected error for invalid direction")
	}
}

func TestA2ATask_AllStatesValid(t *testing.T) {
	states := []TaskState{
		TaskStateSubmitted, TaskStateWorking, TaskStateCompleted,
		TaskStateFailed, TaskStateCanceled, TaskStateRejected,
		TaskStateInputRequired, TaskStateAuthRequired,
	}
	for _, s := range states {
		if !IsValidState(s) {
			t.Errorf("state %q should be valid", s)
		}
	}
}

func TestA2ATask_Defaults(t *testing.T) {
	task := NewA2ATask("t1")
	if task.State != TaskStateSubmitted {
		t.Errorf("expected submitted, got %s", task.State)
	}
	if task.Direction != DirectionInbound {
		t.Errorf("expected inbound, got %s", task.Direction)
	}
	if task.TrustOrigin != "a2a" {
		t.Errorf("expected a2a, got %s", task.TrustOrigin)
	}
	if task.Version != 1 {
		t.Errorf("expected version 1, got %d", task.Version)
	}
}

func TestA2ATask_MetadataNilSafety(t *testing.T) {
	task := &A2ATask{ID: "t1", State: TaskStateSubmitted, Direction: DirectionInbound}
	if err := task.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestA2ATask_OutboundDirection(t *testing.T) {
	task := NewA2ATask("t1")
	task.Direction = DirectionOutbound
	if err := task.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
