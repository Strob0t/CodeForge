package postgres_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/task"
)

// TestStore_EndTaskDispatch: the control plane ends a task's dispatch (lost
// worker, dead-lettered dispatch, never accepted) only while that dispatch
// is the task's current one and the task is still queued or running (S2-F
// review, F9): a result that arrived meanwhile, or a later dispatch, is
// never overwritten.
func TestStore_EndTaskDispatch(t *testing.T) {
	f := newStatusFixture(t)
	other := newStatusFixture(t)
	dispatched := func() (*task.Task, string) {
		t.Helper()
		tk, err := f.store.CreateTask(f.ctx, task.CreateRequest{ProjectID: f.project.ID, Title: "end", Prompt: "p"})
		if err != nil {
			t.Fatalf("CreateTask: %v", err)
		}
		dispatch := uuid.New().String()
		if err := f.store.QueueTask(f.ctx, tk.ID, f.agent.ID, dispatch); err != nil {
			t.Fatalf("QueueTask: %v", err)
		}
		return tk, dispatch
	}
	failed := task.Result{Error: "heartbeat timeout"}

	tk, dispatch := dispatched()
	if err := f.store.EndTaskDispatch(f.ctx, tk.ID, uuid.New().String(), task.StatusFailed, failed); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("EndTaskDispatch(another dispatch) = %v, want ErrConflict", err)
	}
	if err := other.store.EndTaskDispatch(other.ctx, tk.ID, dispatch, task.StatusFailed, failed); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("EndTaskDispatch(other tenant) = %v, want ErrNotFound", err)
	}
	if err := f.store.EndTaskDispatch(f.ctx, tk.ID, dispatch, task.StatusFailed, failed); err != nil {
		t.Fatalf("EndTaskDispatch: %v", err)
	}
	got, err := f.store.GetTask(f.ctx, tk.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status != task.StatusFailed || got.Result == nil || got.Result.Error != "heartbeat timeout" {
		t.Fatalf("task = %s %+v, want failed with the reason", got.Status, got.Result)
	}
	// The dispatch ended once.
	if err := f.store.EndTaskDispatch(f.ctx, tk.ID, dispatch, task.StatusFailed, failed); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("EndTaskDispatch(again) = %v, want ErrConflict", err)
	}

	// A result that arrived first is kept.
	done, doneDispatch := dispatched()
	if err := f.store.UpdateTaskResult(f.ctx, done.ID, task.StatusCompleted, task.Result{Output: "done"}, 0.1); err != nil {
		t.Fatalf("UpdateTaskResult: %v", err)
	}
	if err := f.store.EndTaskDispatch(f.ctx, done.ID, doneDispatch, task.StatusFailed, failed); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("EndTaskDispatch(completed task) = %v, want ErrConflict", err)
	}
	if got, _ := f.store.GetTask(f.ctx, done.ID); got.Status != task.StatusCompleted || got.Result.Output != "done" {
		t.Fatalf("completed task = %s %+v, want its result kept", got.Status, got.Result)
	}

	if err := f.store.EndTaskDispatch(f.ctx, uuid.New().String(), dispatch, task.StatusFailed, failed); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("EndTaskDispatch(unknown task) = %v, want ErrNotFound", err)
	}
}
