package postgres_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/task"
)

// TestStore_UpdateTaskResult: the result, its cost and the task's status are
// written in one statement, with the status the caller gives (a failed or
// cancelled run leaves its task failed or cancelled, not completed).
func TestStore_UpdateTaskResult(t *testing.T) {
	store := setupStore(t)
	ctx := ctxWithTenant(t, createTestTenant(t, store))

	proj, err := store.CreateProject(ctx, &project.CreateRequest{Name: "task-result-project", Provider: "local"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteProject(ctx, proj.ID) })

	for _, status := range []task.Status{task.StatusCompleted, task.StatusFailed, task.StatusCancelled} {
		t.Run(string(status), func(t *testing.T) {
			tsk, err := store.CreateTask(ctx, task.CreateRequest{ProjectID: proj.ID, Title: "result", Prompt: "p"})
			if err != nil {
				t.Fatalf("CreateTask: %v", err)
			}
			result := task.Result{Output: "out", Error: "err"}
			if err := store.UpdateTaskResult(ctx, tsk.ID, status, result, 0.5); err != nil {
				t.Fatalf("UpdateTaskResult: %v", err)
			}
			got, err := store.GetTask(ctx, tsk.ID)
			if err != nil {
				t.Fatalf("GetTask: %v", err)
			}
			if got.Status != status || got.CostUSD != 0.5 || got.Result == nil || got.Result.Output != "out" || got.Result.Error != "err" {
				t.Errorf("task = %s cost %.2f result %+v, want %s cost 0.50 with the result", got.Status, got.CostUSD, got.Result, status)
			}
		})
	}

	t.Run("missing or foreign task", func(t *testing.T) {
		tsk, err := store.CreateTask(ctx, task.CreateRequest{ProjectID: proj.ID, Title: "foreign", Prompt: "p"})
		if err != nil {
			t.Fatalf("CreateTask: %v", err)
		}
		otherTenant := ctxWithTenant(t, createTestTenant(t, store))
		if err := store.UpdateTaskResult(otherTenant, tsk.ID, task.StatusFailed, task.Result{}, 1); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("other tenant's task: err = %v, want ErrNotFound", err)
		}
		if err := store.UpdateTaskResult(ctx, uuid.New().String(), task.StatusFailed, task.Result{}, 1); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("unknown task: err = %v, want ErrNotFound", err)
		}
		if got, _ := store.GetTask(ctx, tsk.ID); got.Status != task.StatusPending {
			t.Errorf("other tenant changed the task: %s", got.Status)
		}
	})
}
