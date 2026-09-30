package service

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/agent"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// Every path that ends a run - the worker's completion, the termination
// limits, the post-execution budget, stall detection, the user's cancel and
// the context-level timeout - must go through the same completion path:
// plan-step progress via onRunComplete, task and agent reset, run-state
// cleanup and the run's final events (KI-30).

type runCompleteCall struct {
	runID  string
	status run.Status
}

// newStopPathService returns a RuntimeService on the lifecycle test store with
// a recording onRunComplete callback.
func newStopPathService(store *lifecycleTestStoreEx) (*RuntimeService, *internalMockQueue, *internalMockBroadcaster, *[]runCompleteCall) {
	q := &internalMockQueue{}
	bc := &internalMockBroadcaster{}
	var calls []runCompleteCall
	svc := &RuntimeService{
		store:      store,
		queue:      q,
		hub:        bc,
		events:     &mockEventStore{},
		policy:     NewPolicyService("headless-safe-sandbox", nil),
		runtimeCfg: &config.Runtime{},
		state:      NewRunStateManager(),
		onRunComplete: func(_ context.Context, runID string, status run.Status) {
			calls = append(calls, runCompleteCall{runID: runID, status: status})
		},
	}
	return svc, q, bc, &calls
}

func TestRunStopPaths_UseTheCompletionPath(t *testing.T) {
	toolResult := func(svc *RuntimeService, runID, callID string, cost float64) error {
		return svc.HandleToolCallResult(context.Background(), &messagequeue.ToolCallResultPayload{
			RunID: runID, CallID: callID, Tool: "Read", Success: true, Output: "same output", CostUSD: cost,
		})
	}

	tests := []struct {
		name       string
		stepCount  int
		costUSD    float64
		stallAfter int // > 0: stall tracker threshold for the run
		stop       func(svc *RuntimeService, runID string) error
		wantStatus run.Status
		wantTask   task.Status
	}{
		{
			name:      "termination limit (max steps)",
			stepCount: 50, // headless-safe-sandbox: MaxSteps 50
			stop: func(svc *RuntimeService, runID string) error {
				return svc.HandleToolCallRequest(context.Background(), &messagequeue.ToolCallRequestPayload{
					RunID: runID, CallID: "call-over-limit", Tool: "Read", Path: "main.go",
				})
			},
			wantStatus: run.StatusTimeout,
			wantTask:   task.StatusFailed,
		},
		{
			name:    "post-execution budget",
			costUSD: 4.9, // headless-safe-sandbox: MaxCost 5.0
			stop: func(svc *RuntimeService, runID string) error {
				return toolResult(svc, runID, "call-over-budget", 0.2)
			},
			wantStatus: run.StatusTimeout,
			wantTask:   task.StatusFailed,
		},
		{
			name:       "stall detection",
			stallAfter: 2,
			stop: func(svc *RuntimeService, runID string) error {
				for i := range 2 {
					if err := toolResult(svc, runID, fmt.Sprintf("call-stall-%d", i), 0); err != nil {
						return err
					}
				}
				return nil
			},
			wantStatus: run.StatusFailed,
			wantTask:   task.StatusFailed,
		},
		{
			name: "user cancel",
			stop: func(svc *RuntimeService, runID string) error {
				return svc.CancelRun(context.Background(), runID)
			},
			wantStatus: run.StatusCancelled,
			wantTask:   task.StatusCancelled,
		},
		{
			name: "context-level timeout",
			stop: func(svc *RuntimeService, runID string) error {
				return svc.cancelRunWithReason(context.Background(), runID, "context-level timeout")
			},
			wantStatus: run.StatusTimeout,
			wantTask:   task.StatusFailed,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := newLifecycleTestStoreEx()
			r := &run.Run{
				ID: "run-stop", TaskID: "task-stop", AgentID: "agent-stop", ProjectID: "proj-stop",
				PolicyProfile: "headless-safe-sandbox", Status: run.StatusRunning,
				StepCount: tc.stepCount, CostUSD: tc.costUSD, StartedAt: time.Now(),
			}
			store.runs[r.ID] = r
			svc, q, bc, calls := newStopPathService(store)

			// Ephemeral state the completion path must release.
			svc.state.SetHeartbeat(r.ID, time.Now())
			threshold := 100
			if tc.stallAfter > 0 {
				threshold = tc.stallAfter
			}
			svc.state.SetStallTracker(r.ID, run.NewStallTracker(threshold, 0))
			svc.state.StoreBudgetAlert(r.ID + ":80")
			pending := make(chan string, 1)
			svc.state.SetPendingApproval(r.ID+":call-waiting", pending)

			if err := tc.stop(svc, r.ID); err != nil {
				t.Fatalf("stop: %v", err)
			}

			if got := store.completedRuns[r.ID]; got != tc.wantStatus {
				t.Errorf("run status = %q, want %q", got, tc.wantStatus)
			}
			if len(*calls) != 1 || (*calls)[0] != (runCompleteCall{runID: r.ID, status: tc.wantStatus}) {
				t.Errorf("onRunComplete calls = %+v, want exactly one for %s/%s", *calls, r.ID, tc.wantStatus)
			}
			if got := store.taskStatuses[r.TaskID]; got != tc.wantTask {
				t.Errorf("task status = %q, want %q", got, tc.wantTask)
			}
			if got := store.agentStatuses[r.AgentID]; got != agent.StatusIdle {
				t.Errorf("agent status = %q, want idle", got)
			}
			if stats, ok := store.agentStats[r.AgentID]; !ok || stats.success {
				t.Errorf("agent stats = %+v (recorded %v), want one unsuccessful run", stats, ok)
			}

			if _, ok := svc.state.GetHeartbeat(r.ID); ok {
				t.Error("heartbeat not released")
			}
			if _, ok := svc.state.GetStallTracker(r.ID); ok {
				t.Error("stall tracker not released")
			}
			if svc.state.StoreBudgetAlert(r.ID + ":80") {
				t.Error("budget alert not released")
			}
			select {
			case decision := <-pending:
				if decision != "deny" {
					t.Errorf("pending approval resolved with %q, want deny", decision)
				}
			default:
				t.Error("pending approval not resolved")
			}

			if !publishedRunCancel(t, q, r.ID) {
				t.Error("worker not told to stop the run (runs.cancel)")
			}
			if !broadcastRunFinished(bc, r.ID) {
				t.Error("no agui.run_finished broadcast")
			}
		})
	}
}

// publishedRunCancel reports whether a runs.cancel message for runID was published.
func publishedRunCancel(t *testing.T, q *internalMockQueue, runID string) bool {
	t.Helper()
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, msg := range q.messages {
		if msg.subject != messagequeue.SubjectRunCancel {
			continue
		}
		var payload struct {
			RunID string `json:"run_id"`
		}
		if err := json.Unmarshal(msg.data, &payload); err != nil {
			t.Fatalf("unmarshal runs.cancel: %v", err)
		}
		if payload.RunID == runID {
			return true
		}
	}
	return false
}

// broadcastRunFinished reports whether an AG-UI run_finished event for runID was broadcast.
func broadcastRunFinished(bc *internalMockBroadcaster, runID string) bool {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	for _, ev := range bc.events {
		if finished, ok := ev.data.(event.AGUIRunFinishedEvent); ok && finished.RunID == runID {
			return true
		}
	}
	return false
}
