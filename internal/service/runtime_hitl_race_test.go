package service_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
)

// A tool call that waits for HITL approval must not move its run back to
// running when the run ended meanwhile (cancel, timeout, completion on
// another path), and must not be allowed to execute (KI-31).

// waitForPermissionRequest blocks until the runtime asked the user to approve
// the tool call.
func waitForPermissionRequest(t *testing.T, bc *runtimeMockBroadcaster, runID, callID string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, ev := range bc.snapshot() {
			if req, ok := ev.Data.(event.AGUIPermissionRequestEvent); ok && req.RunID == runID && req.CallID == callID {
				return
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("no permission request for %s/%s", runID, callID)
}

func TestHandleToolCallRequest_RunEndsWhileWaitingForApproval(t *testing.T) {
	const runID, callID = "run-hitl-race", "call-hitl-race"

	endElsewhere := func(status run.Status) func(ctx context.Context, _ *service.RuntimeService, store *runtimeMockStore) error {
		return func(ctx context.Context, _ *service.RuntimeService, store *runtimeMockStore) error {
			return store.CompleteRun(ctx, &run.CompletionRequest{ID: runID, Status: status, StepCount: 2})
		}
	}
	tests := []struct {
		name       string
		end        func(ctx context.Context, svc *service.RuntimeService, store *runtimeMockStore) error
		resolve    string // the user's decision after the run ended; "" = none
		wantStatus run.Status
	}{
		{
			name: "user cancels the run",
			end: func(ctx context.Context, svc *service.RuntimeService, _ *runtimeMockStore) error {
				return svc.CancelRun(ctx, runID)
			},
			wantStatus: run.StatusCancelled,
		},
		{
			name:       "run times out elsewhere, then the user approves",
			end:        endElsewhere(run.StatusTimeout),
			resolve:    "allow",
			wantStatus: run.StatusTimeout,
		},
		{
			name:       "run completes elsewhere, then the user approves",
			end:        endElsewhere(run.StatusCompleted),
			resolve:    "allow",
			wantStatus: run.StatusCompleted,
		},
		{
			name:       "run is cancelled elsewhere, then the user denies",
			end:        endElsewhere(run.StatusCancelled),
			resolve:    "deny",
			wantStatus: run.StatusCancelled,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, store, queue, bc := newRuntimeTestEnv()
			ctx := context.Background()
			store.mu.Lock()
			store.runs = append(store.runs, run.Run{
				ID: runID, TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1",
				PolicyProfile: "supervised-ask-all", // Edit needs approval
				Status:        run.StatusRunning, StepCount: 2, StartedAt: time.Now(),
			})
			store.mu.Unlock()

			done := make(chan error, 1)
			go func() {
				done <- svc.HandleToolCallRequest(ctx, &messagequeue.ToolCallRequestPayload{
					RunID: runID, CallID: callID, Tool: "Edit", Path: "main.go",
				})
			}()
			waitForPermissionRequest(t, bc, runID, callID)

			if err := tc.end(ctx, svc, store); err != nil {
				t.Fatalf("end run: %v", err)
			}
			if tc.resolve != "" && !svc.ResolveApproval(context.Background(), runID, callID, tc.resolve) {
				t.Fatal("approval was no longer pending")
			}
			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("HandleToolCallRequest: %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("HandleToolCallRequest did not return")
			}

			msg, ok := queue.lastMessage(messagequeue.SubjectRunToolCallResponse)
			if !ok {
				t.Fatal("no tool call response")
			}
			var resp messagequeue.ToolCallResponsePayload
			if err := json.Unmarshal(msg.Data, &resp); err != nil {
				t.Fatalf("unmarshal response: %v", err)
			}
			if resp.CallID != callID || resp.Decision != "deny" {
				t.Errorf("response = %s/%s (%s), want deny for %s", resp.CallID, resp.Decision, resp.Reason, callID)
			}

			r, err := store.GetRun(ctx, runID)
			if err != nil {
				t.Fatalf("GetRun: %v", err)
			}
			if r.Status != tc.wantStatus || r.StepCount != 2 {
				t.Errorf("run = %s with %d steps, want %s with 2 steps", r.Status, r.StepCount, tc.wantStatus)
			}
		})
	}
}
