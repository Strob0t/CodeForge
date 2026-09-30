package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// endedRunStore reports every status update of a run as a conflict: the run
// ended on another path after it was loaded (KI-31).
type endedRunStore struct {
	lifecycleTestStoreEx
}

func (s *endedRunStore) CompleteRun(_ context.Context, req *run.CompletionRequest) error {
	return fmt.Errorf("complete run %s: %w", req.ID, domain.ErrConflict)
}

func (s *endedRunStore) UpdateRunStatus(_ context.Context, id string, _ run.Status, _ int, _ float64, _, _ int64) error {
	return fmt.Errorf("update run status %s: %w", id, domain.ErrConflict)
}

func (s *endedRunStore) EnterQualityGate(_ context.Context, req *run.CompletionRequest) error {
	return fmt.Errorf("enter quality gate %s: %w", req.ID, domain.ErrConflict)
}

func (s *endedRunStore) CountRunStep(_ context.Context, id string) error {
	return fmt.Errorf("count run step %s: %w", id, domain.ErrConflict)
}

// TestRunEndedElsewhere_CompletionPathsSkip: when the store refuses the
// terminal update because the run already ended, no path repeats the
// completion's side effects - the path that ended the run did them - and the
// NATS handlers do not fail (which would redeliver the message).
func TestRunEndedElsewhere_CompletionPathsSkip(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name      string
		status    run.Status // status the loaded (stale) run has
		profile   string
		stepCount int
		act       func(svc *RuntimeService, runID string) error
		wantErr   bool // a conflict the caller must see (HTTP 409)
		// Subjects the path may publish. A control-plane stop tells the worker
		// to stop before it completes the run, whatever the completion does.
		wantPublish []string
	}{
		{
			name: "worker completion", status: run.StatusRunning, profile: "plan-readonly",
			act: func(svc *RuntimeService, runID string) error {
				return svc.HandleRunComplete(ctx, &messagequeue.RunCompletePayload{RunID: runID, Status: "completed", Output: "late"})
			},
		},
		{
			name: "quality gate transition", status: run.StatusRunning, profile: "headless-safe-sandbox",
			act: func(svc *RuntimeService, runID string) error {
				return svc.HandleRunComplete(ctx, &messagequeue.RunCompletePayload{RunID: runID, Status: "completed"})
			},
		},
		{
			name: "quality gate result", status: run.StatusQualityGate, profile: "headless-safe-sandbox",
			act: func(svc *RuntimeService, runID string) error {
				return svc.HandleQualityGateResult(ctx, &messagequeue.QualityGateResultPayload{RunID: runID})
			},
		},
		{
			name: "termination limit", status: run.StatusRunning, profile: "headless-safe-sandbox", stepCount: 50,
			act: func(svc *RuntimeService, runID string) error {
				return svc.HandleToolCallRequest(ctx, &messagequeue.ToolCallRequestPayload{RunID: runID, CallID: "c1", Tool: "Read"})
			},
			wantPublish: []string{messagequeue.SubjectRunCancel, messagequeue.SubjectRunToolCallResponse},
		},
		{
			name: "post-execution budget", status: run.StatusRunning, profile: "headless-safe-sandbox",
			act: func(svc *RuntimeService, runID string) error {
				return svc.HandleToolCallResult(ctx, &messagequeue.ToolCallResultPayload{RunID: runID, CallID: "c1", Tool: "Read", Success: true, CostUSD: 9})
			},
			wantPublish: []string{messagequeue.SubjectRunCancel},
		},
		{
			name: "user cancel", status: run.StatusRunning, profile: "headless-safe-sandbox",
			act: func(svc *RuntimeService, runID string) error {
				return svc.CancelRun(ctx, runID)
			},
			wantErr:     true,
			wantPublish: []string{messagequeue.SubjectRunCancel},
		},
		{
			name: "context-level timeout", status: run.StatusRunning, profile: "headless-safe-sandbox",
			act: func(svc *RuntimeService, runID string) error {
				return svc.cancelRunWithReason(ctx, runID, "context-level timeout")
			},
			wantErr:     true,
			wantPublish: []string{messagequeue.SubjectRunCancel},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := &endedRunStore{lifecycleTestStoreEx: *newLifecycleTestStoreEx()}
			r := &run.Run{
				ID: "run-ended", TaskID: "task-ended", AgentID: "agent-ended", ProjectID: "proj-ended",
				PolicyProfile: tc.profile, Status: tc.status, StepCount: tc.stepCount, StartedAt: time.Now(),
			}
			store.runs[r.ID] = r
			q := &internalMockQueue{}
			bc := &internalMockBroadcaster{}
			es := &mockEventStore{}
			completions := 0
			svc := &RuntimeService{
				store: store, queue: q, hub: bc, events: es,
				policy:     NewPolicyService("headless-safe-sandbox", nil),
				runtimeCfg: &config.Runtime{},
				state:      NewRunStateManager(),
				onRunComplete: func(context.Context, string, run.Status) {
					completions++
				},
			}

			err := tc.act(svc, r.ID)
			if tc.wantErr {
				if !errors.Is(err, domain.ErrConflict) {
					t.Fatalf("err = %v, want ErrConflict", err)
				}
			} else if err != nil {
				t.Fatalf("err = %v, want nil (skip)", err)
			}

			if completions != 0 {
				t.Errorf("onRunComplete called %d times, want 0", completions)
			}
			if len(store.taskStatuses) != 0 || len(store.taskResults) != 0 || len(store.agentStatuses) != 0 || len(store.agentStats) != 0 {
				t.Errorf("task/agent reset repeated: tasks %v results %v agents %v stats %v",
					store.taskStatuses, store.taskResults, store.agentStatuses, store.agentStats)
			}
			if broadcastRunFinished(bc, r.ID) {
				t.Error("agui.run_finished repeated")
			}
			q.mu.Lock()
			defer q.mu.Unlock()
			for _, msg := range q.messages {
				if !slices.Contains(tc.wantPublish, msg.subject) {
					t.Errorf("unexpected publish on %s", msg.subject)
				}
			}
		})
	}
}
