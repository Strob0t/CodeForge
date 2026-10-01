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

// TestStore_EndedDispatchDoesNotWatchARun (S2-G fix, 1): the lost-task
// watchdog matches a task's heartbeat by dispatch, and a task kept the
// dispatch ID and heartbeat row of its ended dispatch: when a run took the
// task later, the stale heartbeat counted for the run, and the watchdog
// failed the task and cancelled the healthy run (tasks.cancel). Every end
// of a dispatch, and a run taking the task, now clear the task's dispatch
// and delete its heartbeat row; a late heartbeat of the ended dispatch is
// not recorded.
func TestStore_EndedDispatchDoesNotWatchARun(t *testing.T) {
	f := newStatusFixture(t)
	pool := retentionPool(t)
	ends := map[string]func(tk *task.Task, dispatch string) error{
		"worker result": func(tk *task.Task, _ string) error {
			return f.store.UpdateTaskResult(f.ctx, tk.ID, task.StatusCompleted, task.Result{Output: "done"}, 0.1)
		},
		"control plane end": func(tk *task.Task, dispatch string) error {
			return f.store.EndTaskDispatch(f.ctx, tk.ID, dispatch, task.StatusFailed, task.Result{Error: "lost"})
		},
		"stopped": func(tk *task.Task, _ string) error {
			return f.store.UpdateTaskStatus(f.ctx, tk.ID, task.StatusCancelled)
		},
		"none, the run takes the dispatched task": func(*task.Task, string) error { return nil },
	}
	for name, end := range ends {
		t.Run(name, func(t *testing.T) {
			tk, err := f.store.CreateTask(f.ctx, task.CreateRequest{ProjectID: f.project.ID, Title: "run after dispatch", Prompt: "p"})
			if err != nil {
				t.Fatalf("CreateTask: %v", err)
			}
			dispatch := uuid.New().String()
			if err := f.store.QueueTask(f.ctx, tk.ID, f.agent.ID, dispatch); err != nil {
				t.Fatalf("QueueTask: %v", err)
			}
			if err := f.store.TouchTaskHeartbeat(f.ctx, tk.ID, dispatch); err != nil {
				t.Fatalf("TouchTaskHeartbeat: %v", err)
			}
			if err := end(tk, dispatch); err != nil {
				t.Fatalf("end the dispatch: %v", err)
			}
			// StartRun takes the task.
			if err := f.store.UpdateTaskStatus(f.ctx, tk.ID, task.StatusRunning); err != nil {
				t.Fatalf("UpdateTaskStatus(running): %v", err)
			}
			// A late heartbeat of the ended dispatch.
			if err := f.store.TouchTaskHeartbeat(f.ctx, tk.ID, dispatch); err != nil {
				t.Fatalf("TouchTaskHeartbeat(late): %v", err)
			}
			if _, err := pool.Exec(context.Background(),
				`UPDATE task_heartbeats SET beat_at = NOW() - interval '`+ancientBeat+`' WHERE task_id = $1`, tk.ID); err != nil {
				t.Fatalf("age heartbeat: %v", err)
			}

			stale, err := f.store.ListTasksWithStaleHeartbeat(context.Background(), staleForTest, 1000)
			if err != nil {
				t.Fatalf("ListTasksWithStaleHeartbeat: %v", err)
			}
			for i := range stale {
				if stale[i].ID == tk.ID {
					t.Fatalf("the run's task is listed as lost through dispatch %s", stale[i].DispatchID)
				}
			}
			var beats int
			var current *string
			if err := pool.QueryRow(context.Background(),
				`SELECT (SELECT count(*) FROM task_heartbeats WHERE task_id = $1), dispatch_id FROM tasks WHERE id = $1`,
				tk.ID).Scan(&beats, &current); err != nil {
				t.Fatalf("read task: %v", err)
			}
			if beats != 0 || current != nil {
				t.Fatalf("heartbeat rows = %d, dispatch = %v; want none and no dispatch", beats, current)
			}
		})
	}
}

// TestStore_RecordTaskResult (S2-G fix, 6): a worker's task result named no
// dispatch, so a late result of a dispatch the watchdog had failed
// overwrote the task's next dispatch. Only the result of the task's current
// dispatch ends it (status, result, cost); a result of another dispatch
// only adds its cost, once; a repeated result changes nothing. A result
// without dispatch ID (an older worker) is current only for a task never
// dispatched with an ID.
func TestStore_RecordTaskResult(t *testing.T) {
	f := newStatusFixture(t)
	other := newStatusFixture(t)
	pool := retentionPool(t)
	newTask := func() *task.Task {
		t.Helper()
		tk, err := f.store.CreateTask(f.ctx, task.CreateRequest{ProjectID: f.project.ID, Title: "result", Prompt: "p"})
		if err != nil {
			t.Fatalf("CreateTask: %v", err)
		}
		return tk
	}
	queue := func(tk *task.Task) string {
		t.Helper()
		dispatch := uuid.New().String()
		if err := f.store.QueueTask(f.ctx, tk.ID, f.agent.ID, dispatch); err != nil {
			t.Fatalf("QueueTask: %v", err)
		}
		return dispatch
	}
	record := func(tk *task.Task, dispatch string, status task.Status, output string, cost float64) bool {
		t.Helper()
		current, err := f.store.RecordTaskResult(f.ctx, tk.ID, dispatch, status, task.Result{Output: output}, cost)
		if err != nil {
			t.Fatalf("RecordTaskResult(%s): %v", dispatch, err)
		}
		return current
	}
	check := func(tk *task.Task, status task.Status, output string, cost float64) {
		t.Helper()
		got, err := f.store.GetTask(f.ctx, tk.ID)
		if err != nil {
			t.Fatalf("GetTask: %v", err)
		}
		gotOutput := ""
		if got.Result != nil {
			gotOutput = got.Result.Output
		}
		if got.Status != status || gotOutput != output || got.CostUSD != cost {
			t.Fatalf("task = %s %q cost %v, want %s %q cost %v", got.Status, gotOutput, got.CostUSD, status, output, cost)
		}
	}

	// The current dispatch's result ends it; repeated, it changes nothing.
	tk := newTask()
	d1 := queue(tk)
	if !record(tk, d1, task.StatusCompleted, "done", 0.1) {
		t.Fatal("the current dispatch's result was not current")
	}
	if record(tk, d1, task.StatusCompleted, "done", 0.1) {
		t.Fatal("a repeated result was current")
	}
	check(tk, task.StatusCompleted, "done", 0.1)

	// The watchdog failed d1 and the task was dispatched again: the late
	// result of d1 adds only its cost, once, and d2's result ends d2.
	late := newTask()
	old := queue(late)
	if err := f.store.EndTaskDispatch(f.ctx, late.ID, old, task.StatusFailed, task.Result{Error: "lost"}); err != nil {
		t.Fatalf("EndTaskDispatch: %v", err)
	}
	d2 := queue(late)
	for range 2 { // delivered twice
		if record(late, old, task.StatusCompleted, "late", 0.2) {
			t.Fatal("a result of an ended dispatch was current")
		}
	}
	check(late, task.StatusQueued, "", 0.2)
	if !record(late, d2, task.StatusFailed, "second", 0.3) {
		t.Fatal("the result of the current dispatch was not current")
	}
	check(late, task.StatusFailed, "second", 0.5)

	// No dispatch ID: not current for a task dispatched with an ID.
	withID := newTask()
	queue(withID)
	if record(withID, "", task.StatusCompleted, "anonymous", 0) {
		t.Fatal("a result without dispatch ID ended a dispatch with an ID")
	}
	check(withID, task.StatusQueued, "", 0)
	// It is for a task dispatched before dispatches had IDs.
	legacy := newTask()
	if _, err := pool.Exec(context.Background(), `UPDATE tasks SET status = 'queued' WHERE id = $1`, legacy.ID); err != nil {
		t.Fatalf("queue legacy task: %v", err)
	}
	if !record(legacy, "", task.StatusCompleted, "legacy", 0.4) {
		t.Fatal("a result without dispatch ID did not end a dispatch without ID")
	}
	check(legacy, task.StatusCompleted, "legacy", 0.4)

	if _, err := other.store.RecordTaskResult(other.ctx, tk.ID, d1, task.StatusFailed, task.Result{}, 0); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("RecordTaskResult(other tenant) = %v, want ErrNotFound", err)
	}
	if _, err := f.store.RecordTaskResult(f.ctx, uuid.New().String(), d1, task.StatusFailed, task.Result{}, 0); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("RecordTaskResult(unknown task) = %v, want ErrNotFound", err)
	}
}
