package postgres_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/task"
)

// TestStore_QueueTask: a dispatch queues a task that is neither queued nor
// running; a queued or running task is refused as a conflict, so of two
// concurrent dispatches only one runs it.
func TestStore_QueueTask(t *testing.T) {
	f := newStatusFixture(t)
	other := newStatusFixture(t)

	for _, status := range []task.Status{task.StatusPending, task.StatusFailed, task.StatusCompleted, task.StatusCancelled} {
		tk, err := f.store.CreateTask(f.ctx, task.CreateRequest{ProjectID: f.project.ID, Title: "queue", Prompt: "p"})
		if err != nil {
			t.Fatalf("CreateTask: %v", err)
		}
		if err := f.store.UpdateTaskStatus(f.ctx, tk.ID, status); err != nil {
			t.Fatalf("UpdateTaskStatus: %v", err)
		}
		if err := f.store.QueueTask(f.ctx, tk.ID, f.agent.ID); err != nil {
			t.Fatalf("QueueTask(%s task): %v", status, err)
		}
		got, err := f.store.GetTask(f.ctx, tk.ID)
		if err != nil {
			t.Fatalf("GetTask: %v", err)
		}
		if got.Status != task.StatusQueued || got.AgentID != f.agent.ID {
			t.Fatalf("%s task queued: status = %s, agent = %q; want queued for %s", status, got.Status, got.AgentID, f.agent.ID)
		}
		if err := f.store.QueueTask(f.ctx, tk.ID, f.agent.ID); !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("second QueueTask = %v, want ErrConflict", err)
		}
		if err := other.store.QueueTask(other.ctx, tk.ID, other.agent.ID); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("QueueTask from another tenant = %v, want ErrNotFound", err)
		}
	}

	running, err := f.store.CreateTask(f.ctx, task.CreateRequest{ProjectID: f.project.ID, Title: "running", Prompt: "p"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := f.store.UpdateTaskStatus(f.ctx, running.ID, task.StatusRunning); err != nil {
		t.Fatalf("UpdateTaskStatus: %v", err)
	}
	if err := f.store.QueueTask(f.ctx, running.ID, f.agent.ID); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("QueueTask(running) = %v, want ErrConflict", err)
	}
	if err := f.store.QueueTask(f.ctx, uuid.New().String(), f.agent.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("QueueTask(unknown) = %v, want ErrNotFound", err)
	}
}
