package service_test

import (
	"context"
	"testing"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
)

// A tool call's usage is counted once: while the run is running it is added
// per result (and a redelivered result is dropped); once the worker reported
// its totals (quality gate, completion) the totals include the call, so a
// result that arrives later adds nothing (review 2, finding 1).

func toolResult(runID, callID string, cost float64) *messagequeue.ToolCallResultPayload {
	return &messagequeue.ToolCallResultPayload{RunID: runID, CallID: callID, Tool: "Read", Success: true, CostUSD: cost, TokensIn: 10, TokensOut: 5}
}

func countEvents(es *recordingEventStore, evType event.Type) int {
	n := 0
	for i := range es.events {
		if es.events[i].Type == evType {
			n++
		}
	}
	return n
}

func countBroadcasts(bc *runtimeMockBroadcaster, eventType string) int {
	n := 0
	for _, ev := range bc.snapshot() {
		if ev.EventType == eventType {
			n++
		}
	}
	return n
}

// TestHandleToolCallResult_AfterTheWorkersTotalsIsNotCountedTwice: the worker's
// run.complete is processed before the result of its last call (separate
// consumers); the call is in the totals and is not added again.
func TestHandleToolCallResult_AfterTheWorkersTotalsIsNotCountedTwice(t *testing.T) {
	svc, store, _, _, es := newRunStateEnv()
	ctx := context.Background()
	setStoredRun(store, &run.Run{
		ID: "run-late", TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1",
		PolicyProfile: "plan-readonly", Status: run.StatusRunning,
	})

	if err := svc.HandleToolCallResult(ctx, toolResult("run-late", "c1", 0.5)); err != nil {
		t.Fatalf("HandleToolCallResult c1: %v", err)
	}
	// The worker's totals: both calls.
	if err := svc.HandleRunComplete(ctx, &messagequeue.RunCompletePayload{
		RunID: "run-late", Status: "completed", CostUSD: 1.0, TokensIn: 20, TokensOut: 10, StepCount: 2,
	}); err != nil {
		t.Fatalf("HandleRunComplete: %v", err)
	}
	if err := svc.HandleToolCallResult(ctx, toolResult("run-late", "c2", 0.5)); err != nil {
		t.Fatalf("HandleToolCallResult c2: %v", err)
	}

	r := storedRun(t, store, "run-late")
	if r.Status != run.StatusCompleted || r.CostUSD != 1.0 || r.TokensIn != 20 || r.TokensOut != 10 {
		t.Errorf("run = %s cost %.2f tokens %d/%d, want completed 1.00 20/10 (the worker's totals)", r.Status, r.CostUSD, r.TokensIn, r.TokensOut)
	}
	// Both calls keep their per-tool usage record.
	if n := countEvents(es, event.TypeToolCallResultEv); n != 2 {
		t.Errorf("tool call result events = %d, want 2", n)
	}
}

// TestHandleToolCallResult_RedeliveredResultCountsOnce: a result delivered
// twice (at-least-once) adds its usage and records its event once.
func TestHandleToolCallResult_RedeliveredResultCountsOnce(t *testing.T) {
	svc, store, _, bc, es := newRunStateEnv()
	ctx := context.Background()
	setStoredRun(store, &run.Run{
		ID: "run-dup", TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1",
		PolicyProfile: "headless-safe-sandbox", Status: run.StatusRunning,
	})

	for range 2 {
		if err := svc.HandleToolCallResult(ctx, toolResult("run-dup", "c1", 0.5)); err != nil {
			t.Fatalf("HandleToolCallResult: %v", err)
		}
	}
	if err := svc.HandleToolCallResult(ctx, toolResult("run-dup", "c2", 0.25)); err != nil {
		t.Fatalf("HandleToolCallResult c2: %v", err)
	}

	if r := storedRun(t, store, "run-dup"); r.CostUSD != 0.75 || r.TokensIn != 20 || r.TokensOut != 10 {
		t.Errorf("run cost %.2f tokens %d/%d, want 0.75 20/10", r.CostUSD, r.TokensIn, r.TokensOut)
	}
	if n := countEvents(es, event.TypeToolCallResultEv); n != 2 {
		t.Errorf("tool call result events = %d, want 2", n)
	}
	if n := countBroadcasts(bc, event.AGUIToolResult); n != 2 {
		t.Errorf("tool result broadcasts = %d, want 2", n)
	}
}

// endsBeforeUsageStore ends the run right before a tool result's usage is
// added: the run was running when the result handler loaded it.
type endsBeforeUsageStore struct {
	*runtimeMockStore
}

func (s *endsBeforeUsageStore) AddRunUsage(ctx context.Context, id string, usage *run.Usage) (*run.Run, error) {
	if err := s.CompleteRun(ctx, &run.CompletionRequest{ID: id, Status: run.StatusCompleted, CostUSD: 4.9}); err != nil {
		return nil, err
	}
	return s.runtimeMockStore.AddRunUsage(ctx, id, usage)
}

// TestHandleToolCallResult_RunEndsWhileTheResultIsHandled: the store refuses
// the usage of a run that ended in between; nothing is added and the budget
// is not checked.
func TestHandleToolCallResult_RunEndsWhileTheResultIsHandled(t *testing.T) {
	_, base, queue, bc := newRuntimeTestEnv()
	svc := service.NewRuntimeService(&endsBeforeUsageStore{runtimeMockStore: base}, queue, bc, &runtimeMockEventStore{},
		service.NewPolicyService("headless-safe-sandbox", nil), &config.Runtime{StallThreshold: 5})
	ctx := context.Background()
	setStoredRun(base, &run.Run{
		ID: "run-race", TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1",
		PolicyProfile: "headless-safe-sandbox", Status: run.StatusRunning, CostUSD: 4.8,
	})

	// headless-safe-sandbox: MaxCost 5.0; adding the call would exceed it.
	if err := svc.HandleToolCallResult(ctx, toolResult("run-race", "c1", 0.5)); err != nil {
		t.Fatalf("HandleToolCallResult: %v", err)
	}
	if r := storedRun(t, base, "run-race"); r.Status != run.StatusCompleted || r.CostUSD != 4.9 {
		t.Errorf("run = %s cost %.2f, want completed with the worker's 4.90", r.Status, r.CostUSD)
	}
	if len(queue.messages) != 0 {
		t.Errorf("published %d messages, want no stop of the ended run", len(queue.messages))
	}
}
