package service_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
)

// slowCompleteStore takes a while to commit a run's completion, which opens
// the window between waking the run's HITL waiters and the terminal record.
type slowCompleteStore struct {
	*runtimeMockStore
}

func (s slowCompleteStore) CompleteRun(ctx context.Context, req *run.CompletionRequest) error {
	time.Sleep(100 * time.Millisecond)
	return s.runtimeMockStore.CompleteRun(ctx, req)
}

// TestStop_WakesApprovalWaitersAfterTheRunEnded: a stop commits the terminal
// status before it wakes the calls waiting for approval, so a woken call is
// denied because the run ended - no step is counted and no policy denial is
// recorded (review finding 12).
func TestStop_WakesApprovalWaitersAfterTheRunEnded(t *testing.T) {
	_, mock, queue, bc := newRuntimeTestEnv()
	es := &recordingEventStore{}
	svc := service.NewRuntimeService(slowCompleteStore{mock}, queue, bc, es, service.NewPolicyService("headless-safe-sandbox", nil), &config.Runtime{})
	ctx := context.Background()
	const runID, callID = "run-stop-waiter", "call-waiting"
	setStoredRun(mock, &run.Run{
		ID: runID, TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1",
		PolicyProfile: "supervised-ask-all", Status: run.StatusRunning, StepCount: 2,
	})

	done := make(chan error, 1)
	go func() {
		done <- svc.HandleToolCallRequest(ctx, &messagequeue.ToolCallRequestPayload{RunID: runID, CallID: callID, Tool: "Edit", Path: "main.go"})
	}()
	waitForPermissionRequest(t, bc, runID, callID)

	if err := svc.CancelRun(ctx, runID); err != nil {
		t.Fatalf("CancelRun: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("HandleToolCallRequest: %v", err)
	}

	resp := toolCallResponse(t, queue, callID)
	if resp.Decision != "deny" || resp.Reason != "run is no longer running" {
		t.Errorf("response = %s (%s), want deny because the run ended", resp.Decision, resp.Reason)
	}
	if r := storedRun(t, mock, runID); r.Status != run.StatusCancelled || r.StepCount != 2 {
		t.Errorf("run = %s with %d steps, want cancelled with 2 steps", r.Status, r.StepCount)
	}
	for _, a := range es.audits {
		if a.Action == "policy.denied" {
			t.Errorf("policy denial recorded for a call of an ended run: %s", a.Details)
		}
	}
	for _, ev := range es.events {
		if ev.Type == event.TypeToolCallDenied {
			t.Errorf("tool call denied event recorded for a call of an ended run: %s", ev.Payload)
		}
	}
}

func toolCallResponse(t *testing.T, queue *runtimeMockQueue, callID string) messagequeue.ToolCallResponsePayload {
	t.Helper()
	queue.mu.Lock()
	defer queue.mu.Unlock()
	for _, msg := range queue.messages {
		if msg.Subject != messagequeue.SubjectRunToolCallResponse {
			continue
		}
		var resp messagequeue.ToolCallResponsePayload
		if err := json.Unmarshal(msg.Data, &resp); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}
		if resp.CallID == callID {
			return resp
		}
	}
	t.Fatalf("no response for %s", callID)
	return messagequeue.ToolCallResponsePayload{}
}

// ctxQueue refuses to publish on a cancelled context, like the NATS client.
type ctxQueue struct {
	runtimeMockQueue
}

func (q *ctxQueue) Publish(ctx context.Context, subject string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("publish %s: %w", subject, err)
	}
	return q.runtimeMockQueue.Publish(ctx, subject, data)
}

func (q *ctxQueue) cancelPublished(runID string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, msg := range q.messages {
		var payload struct {
			RunID string `json:"run_id"`
		}
		if msg.Subject == messagequeue.SubjectRunCancel && json.Unmarshal(msg.Data, &payload) == nil && payload.RunID == runID {
			return true
		}
	}
	return false
}

// TestStopRun_TellsTheWorkerFirst: the worker is told to stop before the
// completion path runs (checkpoint cleanup, the next plan step), and even when
// the caller's context is gone or the run cannot be completed (finding 2).
func TestStopRun_TellsTheWorkerFirst(t *testing.T) {
	newEnv := func(t *testing.T) (*service.RuntimeService, *runtimeMockStore, *ctxQueue) {
		t.Helper()
		_, store, _, bc := newRuntimeTestEnv()
		queue := &ctxQueue{}
		svc := service.NewRuntimeService(store, queue, bc, &runtimeMockEventStore{}, service.NewPolicyService("headless-safe-sandbox", nil), &config.Runtime{})
		setStoredRun(store, &run.Run{
			ID: "run-stop", TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1",
			PolicyProfile: "headless-safe-sandbox", Status: run.StatusRunning,
		})
		return svc, store, queue
	}

	t.Run("before the completion path", func(t *testing.T) {
		svc, _, queue := newEnv(t)
		var sentBeforeCompletion bool
		var once sync.Once
		svc.SetOnRunComplete(func(context.Context, string, run.Status) {
			once.Do(func() { sentBeforeCompletion = queue.cancelPublished("run-stop") })
		})
		if err := svc.CancelRun(context.Background(), "run-stop"); err != nil {
			t.Fatalf("CancelRun: %v", err)
		}
		if !sentBeforeCompletion {
			t.Error("runs.cancel was not published before onRunComplete")
		}
	})

	t.Run("the caller's context is cancelled", func(t *testing.T) {
		svc, _, queue := newEnv(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // the HTTP client went away
		_ = svc.CancelRun(ctx, "run-stop")
		if !queue.cancelPublished("run-stop") {
			t.Error("runs.cancel not published with the caller's cancelled context")
		}
	})

	t.Run("the run cannot be completed", func(t *testing.T) {
		_, store, queue := newEnv(t)
		store.mu.Lock()
		store.runs[0].StepCount = 50 // headless-safe-sandbox: MaxSteps 50
		store.mu.Unlock()
		svc := service.NewRuntimeService(&failingCompleteStore{runtimeMockStore: store}, queue, &runtimeMockBroadcaster{},
			&runtimeMockEventStore{}, service.NewPolicyService("headless-safe-sandbox", nil), &config.Runtime{})

		if err := svc.HandleToolCallRequest(context.Background(), &messagequeue.ToolCallRequestPayload{RunID: "run-stop", CallID: "c1", Tool: "Read"}); err != nil {
			t.Fatalf("HandleToolCallRequest: %v", err)
		}
		if !queue.cancelPublished("run-stop") {
			t.Error("runs.cancel not published when completing the run failed")
		}
	})
}

// failingCompleteStore cannot write completions (database unavailable).
type failingCompleteStore struct {
	*runtimeMockStore
}

func (s *failingCompleteStore) CompleteRun(context.Context, *run.CompletionRequest) error {
	return fmt.Errorf("complete run: connection refused")
}
