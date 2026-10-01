package postgres_test

import (
	"context"
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

// TestStore_ListTasksNeverAccepted: a dispatch that no worker accepted (no
// heartbeat for it) within the accept timeout is listed, across tenants,
// with its tenant and dispatch (S2-F review, F5): nothing else ended a task
// whose message never reached a worker.
func TestStore_ListTasksNeverAccepted(t *testing.T) {
	a, b := newStatusFixture(t), newStatusFixture(t)
	pool := retentionPool(t)
	dispatched := func(f *statusFixture) (*task.Task, string) {
		t.Helper()
		tk, err := f.store.CreateTask(f.ctx, task.CreateRequest{ProjectID: f.project.ID, Title: "accept", Prompt: "p"})
		if err != nil {
			t.Fatalf("CreateTask: %v", err)
		}
		dispatch := uuid.New().String()
		if err := f.store.QueueTask(f.ctx, tk.ID, f.agent.ID, dispatch); err != nil {
			t.Fatalf("QueueTask: %v", err)
		}
		return tk, dispatch
	}
	age := func(id string) {
		t.Helper()
		if _, err := pool.Exec(context.Background(), `UPDATE tasks SET dispatched_at = NOW() - interval '`+ancientBeat+`' WHERE id = $1`, id); err != nil {
			t.Fatalf("age dispatch: %v", err)
		}
	}

	waiting, waitingDispatch := dispatched(a)
	foreign, foreignDispatch := dispatched(b)
	accepted, acceptedDispatch := dispatched(a)
	recent, _ := dispatched(a)
	ended, _ := dispatched(a)
	redispatched, oldDispatch := dispatched(a)

	if err := a.store.TouchTaskHeartbeat(a.ctx, accepted.ID, acceptedDispatch); err != nil {
		t.Fatalf("TouchTaskHeartbeat: %v", err)
	}
	if err := a.store.UpdateTaskStatus(a.ctx, ended.ID, task.StatusFailed); err != nil {
		t.Fatalf("UpdateTaskStatus: %v", err)
	}
	// Accepted once, then dispatched again: the new dispatch has no heartbeat.
	if err := a.store.TouchTaskHeartbeat(a.ctx, redispatched.ID, oldDispatch); err != nil {
		t.Fatalf("TouchTaskHeartbeat: %v", err)
	}
	if err := a.store.UpdateTaskStatus(a.ctx, redispatched.ID, task.StatusFailed); err != nil {
		t.Fatalf("UpdateTaskStatus: %v", err)
	}
	newDispatch := uuid.New().String()
	if err := a.store.QueueTask(a.ctx, redispatched.ID, a.agent.ID, newDispatch); err != nil {
		t.Fatalf("QueueTask(again): %v", err)
	}
	for _, id := range []string{waiting.ID, foreign.ID, accepted.ID, ended.ID, redispatched.ID} {
		age(id)
	}

	tasks, err := a.store.ListTasksNeverAccepted(context.Background(), staleForTest, 1000)
	if err != nil {
		t.Fatalf("ListTasksNeverAccepted: %v", err)
	}
	got := map[string]task.Task{}
	for i := range tasks {
		got[tasks[i].ID] = tasks[i]
	}
	for want, dispatch := range map[*task.Task]string{waiting: waitingDispatch, foreign: foreignDispatch, redispatched: newDispatch} {
		tk, ok := got[want.ID]
		if !ok {
			t.Fatalf("never accepted task %s not listed", want.ID)
		}
		if tk.TenantID == "" || tk.ProjectID == "" || tk.DispatchID != dispatch {
			t.Fatalf("listed task = %+v, want its tenant, project and dispatch %s", tk, dispatch)
		}
	}
	for name, tk := range map[string]*task.Task{"accepted": accepted, "recent": recent, "ended": ended} {
		if _, listed := got[tk.ID]; listed {
			t.Errorf("%s task %s listed", name, tk.ID)
		}
	}
}
