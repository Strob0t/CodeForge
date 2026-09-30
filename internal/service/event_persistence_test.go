package service

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/domain/review"
	"github.com/Strob0t/CodeForge/internal/domain/task"
)

// Result and plan events are persisted with the IDs they really have, and an
// event that cannot be stored is logged, never discarded (KI-32).

// captureLogs routes the default logger into a buffer for the test.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(old) })
	return &buf
}

func assertLogged(t *testing.T, logs *bytes.Buffer, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(logs.String(), w) {
			t.Errorf("log does not contain %q:\n%s", w, logs.String())
		}
	}
}

func TestAgentServiceHandleResult_RecordsTheTaskAgent(t *testing.T) {
	tests := []struct {
		name      string
		tasks     []task.Task
		wantAgent string
	}{
		{name: "task assigned to an agent", tasks: []task.Task{{ID: "t1", ProjectID: "p1", AgentID: "a1"}}, wantAgent: "a1"},
		{name: "task without agent", tasks: []task.Task{{ID: "t1", ProjectID: "p1"}}},
		{name: "task not found", tasks: nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			es := &mockEventStore{}
			svc := NewAgentService(&mockStore{tasks: tc.tasks}, &mockQueue{}, &mockBroadcaster{})
			svc.SetEventStore(es)

			if err := svc.HandleResult(context.Background(), "completed", task.Result{Output: "done"}, "t1", "p1", 0.01); err != nil {
				t.Fatalf("HandleResult: %v", err)
			}
			if len(es.events) != 1 {
				t.Fatalf("events = %d, want 1", len(es.events))
			}
			ev := es.events[0]
			if ev.AgentID != tc.wantAgent || ev.TaskID != "t1" || ev.ProjectID != "p1" {
				t.Errorf("event agent/task/project = %q/%q/%q, want %q/t1/p1", ev.AgentID, ev.TaskID, ev.ProjectID, tc.wantAgent)
			}
		})
	}
}

func TestAgentServiceHandleResult_LogsAppendError(t *testing.T) {
	logs := captureLogs(t)
	svc := NewAgentService(&mockStore{}, &mockQueue{}, &mockBroadcaster{})
	svc.SetEventStore(&mockEventStore{appendErr: errors.New("db down")})

	if err := svc.HandleResult(context.Background(), "completed", task.Result{Output: "done"}, "t1", "p1", 0); err != nil {
		t.Fatalf("HandleResult: %v", err)
	}
	assertLogged(t, logs, "best-effort operation failed", "AppendEvent", "db down", "task_id=t1")
}

func TestAppendPlanEvent_WithoutAgentOrTask(t *testing.T) {
	es := &mockEventStore{}
	svc := NewOrchestratorService(newOrchTestStore(), &mockBroadcaster{}, es, nil, &config.Orchestrator{})

	svc.appendPlanEvent(context.Background(), event.TypePlanStarted, &plan.ExecutionPlan{ID: "plan-1", ProjectID: "p1", Name: "n", Status: plan.StatusRunning})

	if len(es.events) != 1 {
		t.Fatalf("events = %d, want 1", len(es.events))
	}
	ev := es.events[0]
	if ev.Type != event.TypePlanStarted || ev.ProjectID != "p1" || ev.AgentID != "" || ev.TaskID != "" {
		t.Errorf("event = %s project %q agent %q task %q, want plan.started for p1 without agent and task", ev.Type, ev.ProjectID, ev.AgentID, ev.TaskID)
	}
}

func TestAppendPlanEvent_LogsAppendError(t *testing.T) {
	logs := captureLogs(t)
	svc := NewOrchestratorService(newOrchTestStore(), &mockBroadcaster{}, &mockEventStore{appendErr: errors.New("db down")}, nil, &config.Orchestrator{})

	svc.appendPlanEvent(context.Background(), event.TypePlanFailed, &plan.ExecutionPlan{ID: "plan-1", ProjectID: "p1"})

	assertLogged(t, logs, "best-effort operation failed", "AppendEvent", "db down", "plan_id=plan-1")
}

func TestReviewAppendEvent_LogsAppendError(t *testing.T) {
	logs := captureLogs(t)
	svc := &ReviewService{events: &mockEventStore{appendErr: errors.New("db down")}}

	svc.appendEvent(context.Background(), event.TypeReviewTriggered, &review.Review{ID: "review-1", ProjectID: "p1"})

	assertLogged(t, logs, "best-effort operation failed", "AppendEvent", "db down", "review_id=review-1")
}
