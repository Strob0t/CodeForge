package service_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
)

// Run status writes follow the run's transitions: `running` is written only
// while the run is pending or running, usage counters never touch the status,
// and a run that ended still records the usage totals the worker reports.

// recordingEventStore records appended events and audit entries.
type recordingEventStore struct {
	runtimeMockEventStore
	events []event.AgentEvent
	audits []event.AuditEntry
}

func (s *recordingEventStore) Append(_ context.Context, ev *event.AgentEvent) error {
	s.events = append(s.events, *ev)
	return nil
}

func (s *recordingEventStore) AppendAudit(_ context.Context, entry *event.AuditEntry) error {
	s.audits = append(s.audits, *entry)
	return nil
}

// newRunStateEnv is newRuntimeTestEnv with a recording event store.
func newRunStateEnv() (*service.RuntimeService, *runtimeMockStore, *runtimeMockQueue, *runtimeMockBroadcaster, *recordingEventStore) {
	_, store, queue, bc := newRuntimeTestEnv()
	es := &recordingEventStore{}
	svc := service.NewRuntimeService(store, queue, bc, es, service.NewPolicyService("headless-safe-sandbox", nil), &config.Runtime{StallThreshold: 5})
	return svc, store, queue, bc, es
}

func setStoredRun(store *runtimeMockStore, r *run.Run) {
	store.mu.Lock()
	defer store.mu.Unlock()
	r.StartedAt = time.Now()
	store.runs = append(store.runs, *r)
}

func setStoredStatus(store *runtimeMockStore, id string, status run.Status) {
	store.mu.Lock()
	defer store.mu.Unlock()
	for i := range store.runs {
		if store.runs[i].ID == id {
			store.runs[i].Status = status
		}
	}
}

func storedRun(t *testing.T, store *runtimeMockStore, id string) *run.Run {
	t.Helper()
	r, err := store.GetRun(context.Background(), id)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	return r
}

// TestHandleToolCallRequest_RunEntersQualityGateWhileWaiting: the worker
// finished while a call waited for approval; the call is denied and the run
// is not moved from quality_gate back to running.
func TestHandleToolCallRequest_RunEntersQualityGateWhileWaiting(t *testing.T) {
	svc, store, queue, bc := newRuntimeTestEnv()
	ctx := context.Background()
	const runID, callID = "run-gate-wait", "call-gate-wait"
	setStoredRun(store, &run.Run{
		ID: runID, TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1",
		PolicyProfile: "supervised-ask-all", Status: run.StatusRunning, StepCount: 2,
	})

	done := make(chan error, 1)
	go func() {
		done <- svc.HandleToolCallRequest(ctx, &messagequeue.ToolCallRequestPayload{RunID: runID, CallID: callID, Tool: "Edit", Path: "main.go"})
	}()
	waitForPermissionRequest(t, bc, runID, callID)
	setStoredStatus(store, runID, run.StatusQualityGate)
	if !svc.ResolveApproval(context.Background(), runID, callID, "allow") {
		t.Fatal("approval was no longer pending")
	}
	if err := <-done; err != nil {
		t.Fatalf("HandleToolCallRequest: %v", err)
	}

	msg, _ := queue.lastMessage(messagequeue.SubjectRunToolCallResponse)
	var resp messagequeue.ToolCallResponsePayload
	_ = json.Unmarshal(msg.Data, &resp)
	if resp.Decision != "deny" {
		t.Errorf("decision = %q (%s), want deny", resp.Decision, resp.Reason)
	}
	if r := storedRun(t, store, runID); r.Status != run.StatusQualityGate || r.StepCount != 2 {
		t.Errorf("run = %s with %d steps, want quality_gate with 2 steps", r.Status, r.StepCount)
	}
}

// TestHandleToolCallResult_QualityGateRunKeepsItsStatus: a late tool result
// leaves the gate waiting and adds no usage (the worker's totals stored with
// the gate include the call); the gate result then ends the run.
func TestHandleToolCallResult_QualityGateRunKeepsItsStatus(t *testing.T) {
	svc, store, _, _, _ := newRunStateEnv()
	ctx := context.Background()
	setStoredRun(store, &run.Run{
		ID: "run-gate", TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1",
		PolicyProfile: "headless-safe-sandbox", Status: run.StatusQualityGate, CostUSD: 1.0,
	})

	if err := svc.HandleToolCallResult(ctx, &messagequeue.ToolCallResultPayload{RunID: "run-gate", CallID: "c1", Tool: "Read", Success: true, CostUSD: 0.5, TokensIn: 10}); err != nil {
		t.Fatalf("HandleToolCallResult: %v", err)
	}
	if r := storedRun(t, store, "run-gate"); r.Status != run.StatusQualityGate || r.CostUSD != 1.0 || r.TokensIn != 0 {
		t.Fatalf("run = %s cost %.2f tokens in %d, want quality_gate 1.00 0", r.Status, r.CostUSD, r.TokensIn)
	}

	passed := true
	if err := svc.HandleQualityGateResult(ctx, &messagequeue.QualityGateResultPayload{RunID: "run-gate", TestsPassed: &passed, LintPassed: &passed}); err != nil {
		t.Fatalf("HandleQualityGateResult: %v", err)
	}
	if r := storedRun(t, store, "run-gate"); r.Status != run.StatusCompleted {
		t.Errorf("run status = %s after the gate passed, want completed", r.Status)
	}
}

// TestHandleToolCallResult_EndedRunRecordsTheCallOnly: a tool call that
// finishes after its run ended records its per-tool usage event and nothing
// else: the run's counters stay (the worker's totals include the call), no
// budget decision, no stop, no broadcast.
func TestHandleToolCallResult_EndedRunRecordsTheCallOnly(t *testing.T) {
	for _, ended := range []run.Status{run.StatusCancelled, run.StatusTimeout, run.StatusCompleted} {
		t.Run(string(ended), func(t *testing.T) {
			svc, store, queue, bc, es := newRunStateEnv()
			ctx := context.Background()
			setStoredRun(store, &run.Run{
				ID: "run-ended", TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1",
				PolicyProfile: "headless-safe-sandbox", Status: ended, CostUSD: 4.8, TokensIn: 100, TokensOut: 50,
			})

			// headless-safe-sandbox: MaxCost 5.0; this call would exceed it.
			if err := svc.HandleToolCallResult(ctx, &messagequeue.ToolCallResultPayload{
				RunID: "run-ended", CallID: "c-late", Tool: "Read", Success: true, CostUSD: 0.5, TokensIn: 20, TokensOut: 10,
			}); err != nil {
				t.Fatalf("HandleToolCallResult: %v", err)
			}

			r := storedRun(t, store, "run-ended")
			if r.Status != ended || r.TokensIn != 100 || r.TokensOut != 50 || r.CostUSD != 4.8 {
				t.Errorf("run = %s cost %.2f tokens %d/%d, want %s 4.80 100/50", r.Status, r.CostUSD, r.TokensIn, r.TokensOut, ended)
			}
			if n := len(bc.snapshot()); n != 0 {
				t.Errorf("broadcasts = %d, want none for an ended run", n)
			}
			if len(queue.messages) != 0 {
				t.Errorf("published %d messages, want none (no stop)", len(queue.messages))
			}
			for _, a := range es.audits {
				t.Errorf("unexpected audit %s: %s", a.Action, a.Details)
			}
			if len(es.events) != 1 || es.events[0].Type != event.TypeToolCallResultEv || es.events[0].CostUSD != 0.5 {
				t.Errorf("events = %+v, want the call's usage event only", es.events)
			}
		})
	}
}

// TestHandleRunComplete_EndedRunKeepsTheWorkersTotals: after the control
// plane stopped a run, the worker's run.complete still carries its usage
// totals; the counters are raised to them (never lowered), nothing else.
func TestHandleRunComplete_EndedRunKeepsTheWorkersTotals(t *testing.T) {
	tests := []struct {
		name      string
		reported  messagequeue.RunCompletePayload
		wantCost  float64
		wantIn    int64
		wantOut   int64
		wantSteps int
	}{
		{
			name:     "higher totals are kept",
			reported: messagequeue.RunCompletePayload{Status: "cancelled", CostUSD: 1.4, TokensIn: 150, TokensOut: 70, StepCount: 4},
			wantCost: 1.4, wantIn: 150, wantOut: 70, wantSteps: 4,
		},
		{
			name:     "lower totals do not lower the counters",
			reported: messagequeue.RunCompletePayload{Status: "completed", CostUSD: 0.5, TokensIn: 10, TokensOut: 5, StepCount: 1, Output: "late"},
			wantCost: 1.0, wantIn: 100, wantOut: 50, wantSteps: 3,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, store, queue, bc, _ := newRunStateEnv()
			completions := 0
			svc.SetOnRunComplete(func(context.Context, string, run.Status) { completions++ })
			setStoredRun(store, &run.Run{
				ID: "run-stopped", TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1",
				PolicyProfile: "plan-readonly", Status: run.StatusCancelled, Error: "cancelled by user",
				CostUSD: 1.0, TokensIn: 100, TokensOut: 50, StepCount: 3,
			})

			payload := tc.reported
			payload.RunID = "run-stopped"
			if err := svc.HandleRunComplete(context.Background(), &payload); err != nil {
				t.Fatalf("HandleRunComplete: %v", err)
			}

			r := storedRun(t, store, "run-stopped")
			if r.Status != run.StatusCancelled || r.Error != "cancelled by user" || r.Output != "" {
				t.Errorf("run = %s %q %q, want the stop's record unchanged", r.Status, r.Error, r.Output)
			}
			if r.CostUSD != tc.wantCost || r.TokensIn != tc.wantIn || r.TokensOut != tc.wantOut || r.StepCount != tc.wantSteps {
				t.Errorf("usage = %.2f %d/%d %d steps, want %.2f %d/%d %d steps",
					r.CostUSD, r.TokensIn, r.TokensOut, r.StepCount, tc.wantCost, tc.wantIn, tc.wantOut, tc.wantSteps)
			}
			if completions != 0 || len(bc.snapshot()) != 0 || len(queue.messages) != 0 {
				t.Errorf("completion repeated: %d callbacks, %d broadcasts, %d messages", completions, len(bc.snapshot()), len(queue.messages))
			}
		})
	}
}

// TestQualityGateResult_KeepsTheWorkersOutcome: a gated run ends with the
// output, model and usage the worker reported, whether the gate passes or
// fails.
func TestQualityGateResult_KeepsTheWorkersOutcome(t *testing.T) {
	passed, failed := true, false
	tests := []struct {
		name       string
		result     messagequeue.QualityGateResultPayload
		wantStatus run.Status
	}{
		{name: "gate passes", result: messagequeue.QualityGateResultPayload{TestsPassed: &passed, LintPassed: &passed}, wantStatus: run.StatusCompleted},
		{name: "gate fails with rollback", result: messagequeue.QualityGateResultPayload{TestsPassed: &failed, LintPassed: &passed}, wantStatus: run.StatusFailed},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, store, _, _, _ := newRunStateEnv()
			ctx := context.Background()
			// The env configures no default gate commands: the project names them.
			store.projects[0].Config = map[string]string{project.ConfigTestCommand: "make test", project.ConfigLintCommand: "make lint"}
			setStoredRun(store, &run.Run{
				ID: "run-gated", TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1",
				PolicyProfile: "headless-safe-sandbox", Status: run.StatusRunning,
			})

			if err := svc.HandleRunComplete(ctx, &messagequeue.RunCompletePayload{
				RunID: "run-gated", Status: "completed", Output: "all done", Model: "model-x",
				CostUSD: 0.25, TokensIn: 100, TokensOut: 50, StepCount: 5,
			}); err != nil {
				t.Fatalf("HandleRunComplete: %v", err)
			}
			if r := storedRun(t, store, "run-gated"); r.Status != run.StatusQualityGate {
				t.Fatalf("run status = %s, want quality_gate", r.Status)
			}

			result := tc.result
			result.RunID = "run-gated"
			if err := svc.HandleQualityGateResult(ctx, &result); err != nil {
				t.Fatalf("HandleQualityGateResult: %v", err)
			}

			r := storedRun(t, store, "run-gated")
			if r.Status != tc.wantStatus || r.Output != "all done" || r.Model != "model-x" ||
				r.CostUSD != 0.25 || r.TokensIn != 100 || r.TokensOut != 50 || r.StepCount != 5 {
				t.Errorf("run = %s %q %q %.2f %d/%d %d steps, want %s %q %q 0.25 100/50 5 steps",
					r.Status, r.Output, r.Model, r.CostUSD, r.TokensIn, r.TokensOut, r.StepCount, tc.wantStatus, "all done", "model-x")
			}
		})
	}
}
