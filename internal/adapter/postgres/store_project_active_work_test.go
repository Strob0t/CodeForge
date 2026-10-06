package postgres_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/domain/task"
)

// A multi-rollout turn resets the workspace between rollouts (KI-195): it
// runs only while no run, task or other conversation turn of the project
// has not ended.
func TestStore_ProjectHasOtherActiveWork(t *testing.T) {
	a, b := newStatusFixture(t), newStatusFixture(t)
	conv, other := a.conversation(t), a.conversation(t)
	busy := func(name string, want bool) {
		t.Helper()
		got, err := a.store.ProjectHasOtherActiveWork(a.ctx, a.project.ID, conv.ID)
		if err != nil {
			t.Fatalf("%s: ProjectHasOtherActiveWork: %v", name, err)
		}
		if got != want {
			t.Fatalf("%s: busy = %v, want %v", name, got, want)
		}
	}

	busy("pending task only", false)
	if err := a.store.BeginConversationTurn(a.ctx, conv.ID, uuid.New().String()); err != nil {
		t.Fatalf("BeginConversationTurn(own): %v", err)
	}
	busy("own turn", false)
	a.runIn(t, run.StatusCompleted)
	busy("ended run", false)
	b.runIn(t, run.StatusRunning)
	busy("run of another tenant's project", false)

	if err := a.store.BeginConversationTurn(a.ctx, other.ID, uuid.New().String()); err != nil {
		t.Fatalf("BeginConversationTurn(other): %v", err)
	}
	busy("turn of another conversation", true)
	if foreign, err := b.store.ProjectHasOtherActiveWork(b.ctx, a.project.ID, ""); err != nil || foreign {
		t.Fatalf("another tenant sees the project busy = %v (%v), want false", foreign, err)
	}
	if _, err := a.store.EndConversationTurn(a.ctx, other.ID, ""); err != nil {
		t.Fatalf("EndConversationTurn: %v", err)
	}
	busy("ended turn", false)

	r := a.runIn(t, run.StatusRunning)
	busy("running run", true)
	if err := a.store.CompleteRun(a.ctx, &run.CompletionRequest{ID: r.ID, Status: run.StatusFailed}); err != nil {
		t.Fatalf("CompleteRun: %v", err)
	}
	busy("failed run", false)

	if err := a.store.UpdateTaskStatus(a.ctx, a.task.ID, task.StatusQueued); err != nil {
		t.Fatalf("UpdateTaskStatus: %v", err)
	}
	busy("queued task", true)
}
