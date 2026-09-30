package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/agent"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
)

// runStartFailingQueue cannot publish run starts.
type runStartFailingQueue struct {
	runtimeMockQueue
}

func (q *runStartFailingQueue) Publish(ctx context.Context, subject string, data []byte) error {
	if subject == messagequeue.SubjectRunStart {
		return errors.New("nats unavailable")
	}
	return q.runtimeMockQueue.Publish(ctx, subject, data)
}

// TestStartRun_FailedDispatchEndsTheRun: a run whose start cannot be
// dispatched after it was created and marked running ends as failed through
// the completion path; its run, task and agent do not stay running (finding 7).
func TestStartRun_FailedDispatchEndsTheRun(t *testing.T) {
	_, store, _, bc := newRuntimeTestEnv()
	svc := service.NewRuntimeService(store, &runStartFailingQueue{}, bc, &runtimeMockEventStore{},
		service.NewPolicyService("headless-safe-sandbox", nil), &config.Runtime{StallThreshold: 5})
	var completed []run.Status
	svc.SetOnRunComplete(func(_ context.Context, _ string, status run.Status) { completed = append(completed, status) })
	ctx := context.Background()

	_, err := svc.StartRun(ctx, &run.StartRequest{TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1"})
	if err == nil {
		t.Fatal("StartRun: want the dispatch error")
	}

	runs, _ := store.ListRunsByTask(ctx, "task-1")
	if len(runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(runs))
	}
	r := runs[0]
	if r.Status != run.StatusFailed || r.Error == "" {
		t.Errorf("run = %s (%q), want failed with the dispatch error", r.Status, r.Error)
	}
	if tsk, _ := store.GetTask(ctx, "task-1"); tsk.Status != task.StatusFailed {
		t.Errorf("task status = %s, want failed", tsk.Status)
	}
	if ag, _ := store.GetAgent(ctx, "agent-1"); ag.Status != agent.StatusIdle {
		t.Errorf("agent status = %s, want idle", ag.Status)
	}
	if len(completed) != 1 || completed[0] != run.StatusFailed {
		t.Errorf("onRunComplete = %v, want one failed completion", completed)
	}
	finished := false
	for _, ev := range bc.snapshot() {
		if f, ok := ev.Data.(event.AGUIRunFinishedEvent); ok && f.RunID == r.ID && f.Status == "failed" {
			finished = true
		}
	}
	if !finished {
		t.Error("no agui.run_finished failed broadcast")
	}
}
