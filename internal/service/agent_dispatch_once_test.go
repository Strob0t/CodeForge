package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/agent"
	"github.com/Strob0t/CodeForge/internal/domain/task"
)

// A task is dispatched once at a time (S2 follow-up): the worker keys tasks
// by dispatch since KI-65, so a second dispatch of a task that is queued or
// running would run it twice on the same workspace. It is refused as a
// conflict (HTTP 409); a task that ended can be dispatched again.

// queueTaskStore applies the store's status guard to QueueTask.
type queueTaskStore struct {
	*mockStore
}

func (s queueTaskStore) QueueTask(_ context.Context, id, agentID string) error {
	for i := range s.tasks {
		if s.tasks[i].ID != id {
			continue
		}
		if s.tasks[i].Status == task.StatusQueued || s.tasks[i].Status == task.StatusRunning {
			return domain.ErrConflict
		}
		s.tasks[i].Status = task.StatusQueued
		s.tasks[i].AgentID = agentID
		return nil
	}
	return domain.ErrNotFound
}

// staleTaskStore reads every task as pending: another dispatch queued it
// after this one read it.
type staleTaskStore struct {
	queueTaskStore
}

func (s staleTaskStore) GetTask(ctx context.Context, id string) (*task.Task, error) {
	t, err := s.queueTaskStore.GetTask(ctx, id)
	if err != nil {
		return nil, err
	}
	stale := *t
	stale.Status = task.StatusPending
	return &stale, nil
}

func TestAgentDispatch_RefusesATaskThatIsQueuedOrRunning(t *testing.T) {
	for _, status := range []task.Status{task.StatusQueued, task.StatusRunning} {
		t.Run(string(status), func(t *testing.T) {
			probe := registerExecutionProbe(t)
			store := dispatchStore("/data/workspaces/proj-1")
			store.tasks[0].Status = status
			svc := NewAgentService(queueTaskStore{store}, &mockQueue{}, &mockBroadcaster{})

			err := svc.Dispatch(context.Background(), "agent-1", "task-1")

			if !errors.Is(err, domain.ErrConflict) {
				t.Fatalf("Dispatch error = %v, want ErrConflict", err)
			}
			if got := probe.reset(); len(got) != 0 {
				t.Fatalf("backend was asked to execute %d task(s)", len(got))
			}
			if store.agents[0].Status != agent.StatusIdle {
				t.Errorf("agent status = %q, want idle", store.agents[0].Status)
			}
			if store.tasks[0].Status != status {
				t.Errorf("task status = %q, want %q", store.tasks[0].Status, status)
			}
		})
	}
}

// TestAgentDispatch_QueuedByTheStoreGuard: two dispatches that both read
// the task as pending are decided by the store's status guard, not by the
// status read before.
func TestAgentDispatch_QueuedByTheStoreGuard(t *testing.T) {
	probe := registerExecutionProbe(t)
	store := dispatchStore("/data/workspaces/proj-1")
	store.tasks[0].Status = task.StatusQueued // the other dispatch won
	svc := NewAgentService(staleTaskStore{queueTaskStore{store}}, &mockQueue{}, &mockBroadcaster{})

	if err := svc.Dispatch(context.Background(), "agent-1", "task-1"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("Dispatch error = %v, want ErrConflict", err)
	}
	if got := probe.reset(); len(got) != 0 {
		t.Fatalf("backend was asked to execute %d task(s)", len(got))
	}
	if store.agents[0].Status != agent.StatusIdle {
		t.Errorf("agent status = %q, want idle", store.agents[0].Status)
	}
}

func TestAgentDispatch_ATaskThatEndedCanBeDispatchedAgain(t *testing.T) {
	for _, status := range []task.Status{task.StatusPending, task.StatusFailed, task.StatusCompleted, task.StatusCancelled} {
		t.Run(string(status), func(t *testing.T) {
			probe := registerExecutionProbe(t)
			store := dispatchStore("/data/workspaces/proj-1")
			store.tasks[0].Status = status
			svc := NewAgentService(queueTaskStore{store}, &mockQueue{}, &mockBroadcaster{})

			if err := svc.Dispatch(context.Background(), "agent-1", "task-1"); err != nil {
				t.Fatalf("Dispatch: %v", err)
			}
			if got := probe.reset(); len(got) != 1 {
				t.Fatalf("backend executions = %d, want 1", len(got))
			}
			if store.tasks[0].Status != task.StatusQueued {
				t.Errorf("task status = %q, want queued", store.tasks[0].Status)
			}
		})
	}
}
