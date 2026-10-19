package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/agent"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
)

// A run that cannot be started ends as failed, whatever the request's
// context, and it counts as no agent work (review 2, findings 7 and 10).

// statsStore records the agent statistics a run's end increments.
type statsStore struct {
	*runtimeMockStore
	statsCalls int
}

func (s *statsStore) IncrementAgentStats(context.Context, string, float64, bool) error {
	s.statsCalls++
	return nil
}

// ctxStore refuses writes on a cancelled context, like the database driver.
type ctxStore struct {
	*runtimeMockStore
}

func (s *ctxStore) CompleteRun(ctx context.Context, req *run.CompletionRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.runtimeMockStore.CompleteRun(ctx, req)
}

func (s *ctxStore) UpdateTaskResult(ctx context.Context, id string, status task.Status, result task.Result, cost float64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.runtimeMockStore.UpdateTaskResult(ctx, id, status, result, cost)
}

// TestStartRun_FailedStartIsNoAgentFailure: a run that could not be started
// did no agent work; it does not count in the agent's statistics.
func TestStartRun_FailedStartIsNoAgentFailure(t *testing.T) {
	_, base, _, bc := newRuntimeTestEnv()
	store := &statsStore{runtimeMockStore: base}
	svc := service.NewRuntimeService(store, &runStartFailingQueue{}, bc, &runtimeMockEventStore{},
		service.NewPolicyService("headless-safe-sandbox", nil), &config.Runtime{StallThreshold: 5})

	if _, err := svc.StartRun(context.Background(), &run.StartRequest{TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1"}); err == nil {
		t.Fatal("StartRun: want the dispatch error")
	}
	runs, _ := base.ListRunsByTask(context.Background(), "task-1")
	if len(runs) != 1 || runs[0].Status != run.StatusFailed {
		t.Fatalf("runs = %+v, want one failed run", runs)
	}
	if store.statsCalls != 0 {
		t.Errorf("agent stats incremented %d times, want 0 for a run that never started", store.statsCalls)
	}
}

// TestStartRun_CancelledRequestStillEndsTheRun: the start failed because the
// request's context was cancelled (client gone); the run is still ended as
// failed, on a context of its own, instead of staying running.
func TestStartRun_CancelledRequestStillEndsTheRun(t *testing.T) {
	_, base, _, bc := newRuntimeTestEnv()
	svc := service.NewRuntimeService(&ctxStore{runtimeMockStore: base}, &ctxQueue{}, bc, &runtimeMockEventStore{},
		service.NewPolicyService("headless-safe-sandbox", nil), &config.Runtime{StallThreshold: 5})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := svc.StartRun(ctx, &run.StartRequest{TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1"}); err == nil {
		t.Fatal("StartRun: want the publish error of the cancelled request")
	}
	runs, _ := base.ListRunsByTask(context.Background(), "task-1")
	if len(runs) != 1 || runs[0].Status != run.StatusFailed {
		t.Fatalf("runs = %+v, want one failed run", runs)
	}
	if tsk, _ := base.GetTask(context.Background(), "task-1"); tsk.Status != task.StatusFailed {
		t.Errorf("task status = %s, want failed", tsk.Status)
	}
}

// runningWriteFailingStore cannot mark a created run running.
type runningWriteFailingStore struct {
	*runtimeMockStore
}

func (s *runningWriteFailingStore) UpdateRunStatus(context.Context, string, run.Status, int, float64, int64, int64) error {
	return errors.New("update run status: connection reset")
}

// TestStartRun_RunThatCannotBeMarkedRunningEnds: the run was created but
// could not be marked running; it ends as failed instead of staying pending,
// and the agent and task it never touched stay as they are.
func TestStartRun_RunThatCannotBeMarkedRunningEnds(t *testing.T) {
	_, base, queue, bc := newRuntimeTestEnv()
	svc := service.NewRuntimeService(&runningWriteFailingStore{runtimeMockStore: base}, queue, bc, &runtimeMockEventStore{},
		service.NewPolicyService("headless-safe-sandbox", nil), &config.Runtime{StallThreshold: 5})
	ctx := context.Background()

	if _, err := svc.StartRun(ctx, &run.StartRequest{TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1"}); err == nil {
		t.Fatal("StartRun: want the status write error")
	}
	runs, _ := base.ListRunsByTask(ctx, "task-1")
	if len(runs) != 1 || runs[0].Status != run.StatusFailed || runs[0].Error == "" {
		t.Fatalf("runs = %+v, want one failed run with the error", runs)
	}
	if tsk, _ := base.GetTask(ctx, "task-1"); tsk.Status != task.StatusPending {
		t.Errorf("task status = %s, want pending (never started)", tsk.Status)
	}
	if ag, _ := base.GetAgent(ctx, "agent-1"); ag.Status != agent.StatusIdle {
		t.Errorf("agent status = %s, want idle", ag.Status)
	}
	if _, ok := queue.lastMessage(messagequeue.SubjectRunStart); ok {
		t.Error("runs.start published for a run that could not be marked running")
	}
}
