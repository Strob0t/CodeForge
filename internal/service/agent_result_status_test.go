package service

import (
	"context"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/task"
)

// taskStatusStore records the task status written with the result and any
// status write after it.
type taskStatusStore struct {
	mockStore
	statuses map[string]task.Status
	patches  int
}

func (s *taskStatusStore) UpdateTaskResult(_ context.Context, id string, status task.Status, _ task.Result, _ float64) error {
	s.statuses[id] = status
	return nil
}

func (s *taskStatusStore) UpdateTaskStatus(_ context.Context, id string, status task.Status) error {
	s.statuses[id] = status
	s.patches++
	return nil
}

// TestAgentServiceHandleResult_TaskStatusFollowsTheResult: an error result
// leaves its task failed, not completed (review finding 8), written with
// the result in one statement.
func TestAgentServiceHandleResult_TaskStatusFollowsTheResult(t *testing.T) {
	tests := []struct {
		name   string
		result task.Result
		want   task.Status
	}{
		{name: "success", result: task.Result{Output: "done"}, want: task.StatusCompleted},
		{name: "error", result: task.Result{Error: "tests failed"}, want: task.StatusFailed},
		{name: "error with output", result: task.Result{Output: "partial", Error: "crashed"}, want: task.StatusFailed},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := &taskStatusStore{statuses: map[string]task.Status{}}
			bc := &mockBroadcaster{}
			svc := NewAgentService(store, &mockQueue{}, bc)

			if err := svc.HandleResult(context.Background(), tc.result, "t1", "p1", 0.01); err != nil {
				t.Fatalf("HandleResult: %v", err)
			}
			if got := store.statuses["t1"]; got != tc.want {
				t.Errorf("task status = %q, want %q", got, tc.want)
			}
			// One write: the status comes with the result.
			if store.patches != 0 {
				t.Errorf("task status patched %d times after the result, want 0", store.patches)
			}
		})
	}
}
