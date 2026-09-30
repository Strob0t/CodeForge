package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// runStatusEvents returns the run.status payloads broadcast so far.
func runStatusEvents(t *testing.T, bc *runtimeMockBroadcaster) []event.RunStatusEvent {
	t.Helper()
	var out []event.RunStatusEvent
	for _, ev := range bc.snapshot() {
		if ev.EventType != event.EventRunStatus {
			continue
		}
		payload, ok := ev.Data.(event.RunStatusEvent)
		if !ok {
			t.Fatalf("run.status payload type %T", ev.Data)
		}
		out = append(out, payload)
	}
	return out
}

// KI-39: the agent lanes attribute runs (and through them tool calls and task
// output) to their agent by the run.status agent_id, which was never sent.
func TestRunStatusEvent_NamesTheAgent(t *testing.T) {
	t.Run("run start", func(t *testing.T) {
		svc, _, _, bc := newRuntimeTestEnv()
		r, err := svc.StartRun(context.Background(), &run.StartRequest{TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1"})
		if err != nil {
			t.Fatalf("StartRun: %v", err)
		}
		events := runStatusEvents(t, bc)
		if len(events) == 0 {
			t.Fatal("no run.status event")
		}
		for _, ev := range events {
			if ev.RunID != r.ID || ev.TaskID != "task-1" || ev.ProjectID != "proj-1" || ev.AgentID != "agent-1" {
				t.Fatalf("run.status = %+v, want run %s of task-1/proj-1 by agent-1", ev, r.ID)
			}
		}
	})

	t.Run("run completion", func(t *testing.T) {
		svc, store, _, bc := newRuntimeTestEnv()
		store.mu.Lock()
		store.runs = append(store.runs, run.Run{
			ID: "run-a1", TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1",
			PolicyProfile: "plan-readonly", Status: run.StatusRunning, StartedAt: time.Now(),
		})
		store.mu.Unlock()

		err := svc.HandleRunComplete(context.Background(), &messagequeue.RunCompletePayload{
			RunID: "run-a1", TaskID: "task-1", ProjectID: "proj-1", Status: "completed", StepCount: 3, CostUSD: 0.01,
		})
		if err != nil {
			t.Fatalf("HandleRunComplete: %v", err)
		}
		events := runStatusEvents(t, bc)
		if len(events) != 1 {
			t.Fatalf("got %d run.status events, want 1", len(events))
		}
		if ev := events[0]; ev.AgentID != "agent-1" || ev.Status != string(run.StatusCompleted) || ev.StepCount != 3 {
			t.Fatalf("run.status = %+v, want completed by agent-1 after 3 steps", ev)
		}
	})
}
