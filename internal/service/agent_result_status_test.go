package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
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

			if err := svc.HandleResult(context.Background(), "completed", tc.result, "t1", "p1", 0.01); err != nil {
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

// TestAgentResultSubscriber_TaskStatusFollowsTheReportedStatus: a task the
// worker stopped on tasks.cancel is reported "cancelled" and stays cancelled;
// before, its error made it failed (KI-22).
func TestAgentResultSubscriber_TaskStatusFollowsTheReportedStatus(t *testing.T) {
	tests := []struct {
		name    string
		payload messagequeue.TaskResultPayload
		want    task.Status
		wantWS  string
	}{
		{
			name:    "completed",
			payload: messagequeue.TaskResultPayload{TaskID: "t1", ProjectID: "p1", Status: "completed", Output: "done"},
			want:    task.StatusCompleted,
			wantWS:  "completed",
		},
		{
			name:    "failed",
			payload: messagequeue.TaskResultPayload{TaskID: "t1", ProjectID: "p1", Status: "failed", Error: "exit 1"},
			want:    task.StatusFailed,
			wantWS:  "failed",
		},
		{
			name:    "cancelled",
			payload: messagequeue.TaskResultPayload{TaskID: "t1", ProjectID: "p1", Status: "cancelled", Error: "cancelled by user"},
			want:    task.StatusCancelled,
			wantWS:  "cancelled",
		},
		// A failure without message (a bare TimeoutError, OpenHands
		// {"status":"failed","error":""}) must not be stored completed.
		{
			name:    "failed without error",
			payload: messagequeue.TaskResultPayload{TaskID: "t1", ProjectID: "p1", Status: "failed"},
			want:    task.StatusFailed,
			wantWS:  "failed",
		},
		{
			name:    "completed with error",
			payload: messagequeue.TaskResultPayload{TaskID: "t1", ProjectID: "p1", Status: "completed", Error: "partial"},
			want:    task.StatusFailed,
			wantWS:  "failed",
		},
		{
			name:    "unknown status",
			payload: messagequeue.TaskResultPayload{TaskID: "t1", ProjectID: "p1", Status: "weird", Output: "?"},
			want:    task.StatusFailed,
			wantWS:  "failed",
		},
		{
			name:    "no status",
			payload: messagequeue.TaskResultPayload{TaskID: "t1", ProjectID: "p1", Output: "?"},
			want:    task.StatusFailed,
			wantWS:  "failed",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := &taskStatusStore{statuses: map[string]task.Status{}}
			bc := &mockBroadcaster{}
			queue := newHandlerCapturingQueue()
			svc := NewAgentService(store, queue, bc)
			if _, err := svc.StartResultSubscriber(context.Background()); err != nil {
				t.Fatalf("StartResultSubscriber: %v", err)
			}
			handler, ok := queue.getHandler(messagequeue.SubjectTaskResult)
			if !ok {
				t.Fatal("no tasks.result handler")
			}
			data, err := json.Marshal(tc.payload)
			if err != nil {
				t.Fatal(err)
			}

			if err := handler(context.Background(), messagequeue.SubjectTaskResult, data); err != nil {
				t.Fatalf("handler: %v", err)
			}
			if got := store.statuses["t1"]; got != tc.want {
				t.Errorf("task status = %q, want %q", got, tc.want)
			}
			last := bc.events[len(bc.events)-1]
			if ev, ok := last.payload.(event.TaskStatusEvent); !ok || ev.Status != tc.wantWS {
				t.Errorf("broadcast = %+v, want task status %q", last.payload, tc.wantWS)
			}
		})
	}
}
