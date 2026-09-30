package service_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
)

// A run the control plane stops ends with the stop's status and reason: the
// worker's own completion, which the runs.cancel triggers, only contributes
// its usage totals (review 2, finding 6). A stopped run keeps the outcome it
// already has, e.g. the output of a run waiting for its quality gate
// (review 2, finding 9).

// workerCompletesOnCancel plays a worker that answers runs.cancel at once
// with its own completion, before the stop records the run's end.
type workerCompletesOnCancel struct {
	runtimeMockQueue
	svc    *service.RuntimeService
	totals messagequeue.RunCompletePayload
}

func (q *workerCompletesOnCancel) Publish(ctx context.Context, subject string, data []byte) error {
	if err := q.runtimeMockQueue.Publish(ctx, subject, data); err != nil {
		return err
	}
	if subject == messagequeue.SubjectRunCancel {
		payload := q.totals
		if err := q.svc.HandleRunComplete(ctx, &payload); err != nil {
			return err
		}
	}
	return nil
}

func newStoppingEnv(t *testing.T, r *run.Run, totals *messagequeue.RunCompletePayload) (*service.RuntimeService, *runtimeMockStore, *[]run.Status) {
	t.Helper()
	_, store, _, bc := newRuntimeTestEnv()
	queue := &workerCompletesOnCancel{totals: *totals}
	svc := service.NewRuntimeService(store, queue, bc, &runtimeMockEventStore{}, service.NewPolicyService("headless-safe-sandbox", nil), &config.Runtime{})
	queue.svc = svc
	var mu sync.Mutex
	var completions []run.Status
	svc.SetOnRunComplete(func(_ context.Context, _ string, status run.Status) {
		mu.Lock()
		completions = append(completions, status)
		mu.Unlock()
	})
	setStoredRun(store, r)
	return svc, store, &completions
}

// TestStop_WorkerCompletionDuringTheStop: the worker's "cancelled" completion
// arrives while the stop is under way; the run ends with the stop's status
// and reason and the worker's usage totals, and the stop succeeds.
func TestStop_WorkerCompletionDuringTheStop(t *testing.T) {
	workerTotals := messagequeue.RunCompletePayload{
		RunID: "run-stopping", Status: "cancelled", Error: "cancelled by control plane",
		CostUSD: 0.9, TokensIn: 300, TokensOut: 120, StepCount: 6,
	}
	newRun := func() *run.Run {
		return &run.Run{
			ID: "run-stopping", TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1",
			PolicyProfile: "headless-safe-sandbox", Status: run.StatusRunning, CostUSD: 0.5, StepCount: 4,
		}
	}

	t.Run("user cancel", func(t *testing.T) {
		svc, store, completions := newStoppingEnv(t, newRun(), &workerTotals)
		if err := svc.CancelRun(context.Background(), "run-stopping"); err != nil {
			t.Fatalf("CancelRun: %v", err)
		}
		r := storedRun(t, store, "run-stopping")
		if r.Status != run.StatusCancelled || r.Error != "cancelled by user" {
			t.Errorf("run = %s %q, want cancelled by user", r.Status, r.Error)
		}
		if r.CostUSD != 0.9 || r.TokensIn != 300 || r.TokensOut != 120 || r.StepCount != 6 {
			t.Errorf("usage = %.2f %d/%d %d steps, want the worker's totals 0.90 300/120 6", r.CostUSD, r.TokensIn, r.TokensOut, r.StepCount)
		}
		if len(*completions) != 1 || (*completions)[0] != run.StatusCancelled {
			t.Errorf("completions = %v, want one cancelled", *completions)
		}
	})

	t.Run("budget stop", func(t *testing.T) {
		expensive := newRun()
		expensive.CostUSD = 4.8 // headless-safe-sandbox: MaxCost 5.0
		totals := workerTotals
		totals.CostUSD = 5.5
		svc, store, completions := newStoppingEnv(t, expensive, &totals)
		// The call's cost pushes the run over its budget: the control plane
		// stops it as timed out, not with the worker's "cancelled".
		if err := svc.HandleToolCallResult(context.Background(), toolResult("run-stopping", "c-expensive", 0.5)); err != nil {
			t.Fatalf("HandleToolCallResult: %v", err)
		}
		r := storedRun(t, store, "run-stopping")
		if r.Status != run.StatusTimeout || !strings.HasPrefix(r.Error, "budget exceeded after tool execution") {
			t.Errorf("run = %s %q, want timeout with the budget reason", r.Status, r.Error)
		}
		if r.CostUSD != 5.5 {
			t.Errorf("cost = %.2f, want the worker's total 5.50", r.CostUSD)
		}
		if len(*completions) != 1 || (*completions)[0] != run.StatusTimeout {
			t.Errorf("completions = %v, want one timeout", *completions)
		}
	})
}

// TestCancelRun_ConcurrentCancels: two cancels of the same run both succeed;
// the run is completed once.
func TestCancelRun_ConcurrentCancels(t *testing.T) {
	_, mock, queue, bc := newRuntimeTestEnv()
	svc := service.NewRuntimeService(slowCompleteStore{mock}, queue, bc, &runtimeMockEventStore{}, service.NewPolicyService("headless-safe-sandbox", nil), &config.Runtime{})
	var mu sync.Mutex
	completions := 0
	svc.SetOnRunComplete(func(context.Context, string, run.Status) {
		mu.Lock()
		completions++
		mu.Unlock()
	})
	setStoredRun(mock, &run.Run{
		ID: "run-twice", TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1",
		PolicyProfile: "headless-safe-sandbox", Status: run.StatusRunning,
	})

	errs := make(chan error, 2)
	for range 2 {
		go func() { errs <- svc.CancelRun(context.Background(), "run-twice") }()
	}
	for range 2 {
		if err := <-errs; err != nil {
			t.Errorf("CancelRun: %v", err)
		}
	}
	if r := storedRun(t, mock, "run-twice"); r.Status != run.StatusCancelled {
		t.Errorf("run = %s, want cancelled", r.Status)
	}
	if completions != 1 {
		t.Errorf("completions = %d, want 1", completions)
	}
}

// TestCancelRun_QualityGateRunKeepsItsOutcome: cancelling a run that waits
// for its quality gate keeps the worker's output, model and usage on the run
// and in the task's result.
func TestCancelRun_QualityGateRunKeepsItsOutcome(t *testing.T) {
	svc, store, _, _ := newRuntimeTestEnv()
	ctx := context.Background()
	setStoredRun(store, &run.Run{
		ID: "run-gated-cancel", TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1",
		PolicyProfile: "headless-safe-sandbox", Status: run.StatusRunning,
	})
	if err := svc.HandleRunComplete(ctx, &messagequeue.RunCompletePayload{
		RunID: "run-gated-cancel", Status: "completed", Output: "all done", Model: "model-x",
		CostUSD: 0.25, TokensIn: 100, TokensOut: 50, StepCount: 5,
	}); err != nil {
		t.Fatalf("HandleRunComplete: %v", err)
	}
	if r := storedRun(t, store, "run-gated-cancel"); r.Status != run.StatusQualityGate {
		t.Fatalf("run = %s, want quality_gate", r.Status)
	}

	if err := svc.CancelRun(ctx, "run-gated-cancel"); err != nil {
		t.Fatalf("CancelRun: %v", err)
	}
	r := storedRun(t, store, "run-gated-cancel")
	if r.Status != run.StatusCancelled || r.Output != "all done" || r.Model != "model-x" ||
		r.CostUSD != 0.25 || r.TokensIn != 100 || r.TokensOut != 50 || r.StepCount != 5 {
		t.Errorf("run = %s %q %q %.2f %d/%d %d steps, want cancelled with the worker's outcome",
			r.Status, r.Output, r.Model, r.CostUSD, r.TokensIn, r.TokensOut, r.StepCount)
	}
	tsk, err := store.GetTask(ctx, "task-1")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if tsk.Result == nil || tsk.Result.Output != "all done" {
		t.Errorf("task result = %+v, want the worker's output", tsk.Result)
	}
}

// failingEndStore fails the first write that ends a run.
type failingEndStore struct {
	*runtimeMockStore
	mu     sync.Mutex
	failed bool
}

func (s *failingEndStore) CompleteRun(ctx context.Context, req *run.CompletionRequest) error {
	s.mu.Lock()
	first := !s.failed
	s.failed = true
	s.mu.Unlock()
	if first {
		return errors.New("database unavailable")
	}
	return s.runtimeMockStore.CompleteRun(ctx, req)
}

// TestStop_FailedEndWriteUsesTheWorkersCompletion: the stop could not record
// the run's end, and the worker's completion, which arrived during the stop,
// was reduced to its usage: the run stayed running (KI-76). The completion
// now ends the run once the stop failed.
func TestStop_FailedEndWriteUsesTheWorkersCompletion(t *testing.T) {
	_, mock, _, bc := newRuntimeTestEnv()
	store := &failingEndStore{runtimeMockStore: mock}
	queue := &workerCompletesOnCancel{totals: messagequeue.RunCompletePayload{
		RunID: "run-stop-fails", Status: "cancelled", Error: "cancelled by control plane", CostUSD: 0.7, StepCount: 3,
	}}
	svc := service.NewRuntimeService(store, queue, bc, &runtimeMockEventStore{}, service.NewPolicyService("headless-safe-sandbox", nil), &config.Runtime{})
	queue.svc = svc
	setStoredRun(mock, &run.Run{
		ID: "run-stop-fails", TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1",
		PolicyProfile: "headless-safe-sandbox", Status: run.StatusRunning,
	})

	if err := svc.CancelRun(context.Background(), "run-stop-fails"); err != nil {
		t.Fatalf("CancelRun: %v", err)
	}

	r := storedRun(t, mock, "run-stop-fails")
	if r.Status != run.StatusCancelled {
		t.Fatalf("run = %s %q, want cancelled by the worker's completion", r.Status, r.Error)
	}
	if r.CostUSD != 0.7 || r.StepCount != 3 {
		t.Errorf("usage = %.2f %d steps, want the worker's totals", r.CostUSD, r.StepCount)
	}
}

// TestStop_FailedEndWriteWithoutCompletionReportsTheError: without a
// completion from the worker the failed stop is reported, and a completion
// that arrives later ends the run normally.
func TestStop_FailedEndWriteWithoutCompletionReportsTheError(t *testing.T) {
	_, mock, queue, bc := newRuntimeTestEnv()
	store := &failingEndStore{runtimeMockStore: mock}
	svc := service.NewRuntimeService(store, queue, bc, &runtimeMockEventStore{}, service.NewPolicyService("headless-safe-sandbox", nil), &config.Runtime{})
	setStoredRun(mock, &run.Run{
		ID: "run-stop-later", TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1",
		PolicyProfile: "headless-safe-sandbox", Status: run.StatusRunning,
	})

	if err := svc.CancelRun(context.Background(), "run-stop-later"); err == nil {
		t.Fatal("CancelRun succeeded although the run's end was not recorded")
	}
	if err := svc.HandleRunComplete(context.Background(), &messagequeue.RunCompletePayload{RunID: "run-stop-later", Status: "cancelled"}); err != nil {
		t.Fatalf("HandleRunComplete: %v", err)
	}
	if r := storedRun(t, mock, "run-stop-later"); r.Status != run.StatusCancelled {
		t.Fatalf("run = %s, want cancelled", r.Status)
	}
}

// usageBlockingStore holds the first usage raise until it is released.
type usageBlockingStore struct {
	*failingEndStore
	once    sync.Once
	entered chan struct{}
	release chan struct{}
}

func (s *usageBlockingStore) RaiseRunUsage(ctx context.Context, runID string, totals *run.Usage) error {
	s.once.Do(func() {
		s.entered <- struct{}{}
		<-s.release
	})
	return s.failingEndStore.RaiseRunUsage(ctx, runID, totals)
}

// workerCompletesConcurrently plays a worker whose completion is handled
// concurrently with the stop: Publish returns once the completion reached
// the store's usage raise.
type workerCompletesConcurrently struct {
	runtimeMockQueue
	svc     *service.RuntimeService
	entered chan struct{}
	done    chan error
	totals  messagequeue.RunCompletePayload
}

func (q *workerCompletesConcurrently) Publish(ctx context.Context, subject string, data []byte) error {
	if err := q.runtimeMockQueue.Publish(ctx, subject, data); err != nil {
		return err
	}
	if subject == messagequeue.SubjectRunCancel {
		payload := q.totals
		go func() { q.done <- q.svc.HandleRunComplete(context.WithoutCancel(ctx), &payload) }()
		<-q.entered
	}
	return nil
}

// TestStop_WorkerCompletionRacingAFailedStop (S2-F review, F2): the worker's
// completion saw the run stopping and wrote its usage; meanwhile the stop
// could not record the run's end and ended, then the completion was deferred
// to a stop that no longer existed and dropped: the run stayed running. The
// completion is now deferred in the same step that sees the stop, so the
// failed stop ends the run with it.
func TestStop_WorkerCompletionRacingAFailedStop(t *testing.T) {
	_, mock, _, bc := newRuntimeTestEnv()
	entered := make(chan struct{})
	store := &usageBlockingStore{failingEndStore: &failingEndStore{runtimeMockStore: mock}, entered: entered, release: make(chan struct{})}
	queue := &workerCompletesConcurrently{entered: entered, done: make(chan error, 1), totals: messagequeue.RunCompletePayload{
		RunID: "run-stop-race", Status: "cancelled", Error: "cancelled by control plane", CostUSD: 0.7, StepCount: 3,
	}}
	svc := service.NewRuntimeService(store, queue, bc, &runtimeMockEventStore{}, service.NewPolicyService("headless-safe-sandbox", nil), &config.Runtime{})
	queue.svc = svc
	setStoredRun(mock, &run.Run{
		ID: "run-stop-race", TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1",
		PolicyProfile: "headless-safe-sandbox", Status: run.StatusRunning,
	})

	cancelErr := svc.CancelRun(context.Background(), "run-stop-race")
	close(store.release)
	if err := <-queue.done; err != nil {
		t.Fatalf("HandleRunComplete: %v", err)
	}
	if cancelErr != nil {
		t.Errorf("CancelRun: %v, want the worker's completion to end the run", cancelErr)
	}
	r := storedRun(t, mock, "run-stop-race")
	if r.Status != run.StatusCancelled {
		t.Fatalf("run = %s %q, want cancelled by the worker's completion", r.Status, r.Error)
	}
	if r.CostUSD != 0.7 || r.StepCount != 3 {
		t.Errorf("usage = %.2f %d steps, want the worker's totals", r.CostUSD, r.StepCount)
	}
}
