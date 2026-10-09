package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/policy"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
)

// KI-28: the gate request carries the configured gate timeout, and runs that
// wait in quality_gate longer than the gate can take are failed through the
// gate's completion path by the stuck-work watchdog.

func newWatchdogConfig(gateTimeout time.Duration) *config.Runtime {
	return &config.Runtime{
		StallThreshold:     5,
		QualityGateTimeout: gateTimeout,
		DefaultTestCommand: "go test ./...",
		DefaultLintCommand: "golangci-lint run ./...",
	}
}

func newWatchdogEnv(gateTimeout time.Duration) (*gateDeliveryEnv, *runtimeMockBroadcaster) {
	_, store, queue, _ := newRuntimeTestEnv()
	bc := &runtimeMockBroadcaster{}
	policySvc := service.NewPolicyService("headless-safe-sandbox", []policy.PolicyProfile{gateNoRollback})
	svc := service.NewRuntimeService(store, queue, bc, &runtimeMockEventStore{}, policySvc, newWatchdogConfig(gateTimeout))
	env := &gateDeliveryEnv{svc: svc, store: store, queue: queue, deliverer: &recordingDeliverer{store: store}, checkpoints: &recordingCheckpointer{}}
	svc.SetDeliverService(env.deliverer)
	svc.SetCheckpointService(env.checkpoints)
	return env, bc
}

func TestEnterQualityGate_SendsTheConfiguredTimeout(t *testing.T) {
	tests := []struct {
		timeout time.Duration
		want    int
	}{
		{timeout: 60 * time.Second, want: 60},
		{timeout: 90 * time.Second, want: 90},
		{timeout: time.Second, want: 1},
		{timeout: 1500 * time.Millisecond, want: 2}, // never shorter than configured
		{timeout: 10 * time.Minute, want: 600},
	}
	for _, tc := range tests {
		t.Run(tc.timeout.String(), func(t *testing.T) {
			env, _ := newWatchdogEnv(tc.timeout)
			env.addRun("run-timeout", "headless-safe-sandbox", run.StatusRunning, run.DeliverModeNone)

			completeRun(t, env, "run-timeout")

			msg, ok := env.queue.lastMessage(messagequeue.SubjectQualityGateRequest)
			if !ok {
				t.Fatal("no gate request")
			}
			var raw map[string]json.RawMessage
			if err := json.Unmarshal(msg.Data, &raw); err != nil {
				t.Fatal(err)
			}
			if got := string(raw["timeout_seconds"]); got != itoa(tc.want) {
				t.Fatalf("timeout_seconds = %s, want %d", got, tc.want)
			}
		})
	}
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

// setRunUpdatedAt ages a stored run: the watchdog finds stuck runs by the
// time of their last update.
func setRunUpdatedAt(store *runtimeMockStore, id string, at time.Time) {
	store.mu.Lock()
	defer store.mu.Unlock()
	for i := range store.runs {
		if store.runs[i].ID == id {
			store.runs[i].UpdatedAt = at
		}
	}
}

func TestFailStuckQualityGates(t *testing.T) {
	env, bc := newWatchdogEnv(time.Minute) // no backlog probe: only the hard cap (1h) applies
	ctx := context.Background()
	long := time.Now().Add(-2 * time.Hour) // beyond the hard cap (1h here)
	for _, r := range []struct {
		id      string
		status  run.Status
		updated time.Time
	}{
		{"run-stuck", run.StatusQualityGate, long},
		{"run-gate-fresh", run.StatusQualityGate, time.Now()},
		{"run-gate-within-deadline", run.StatusQualityGate, time.Now().Add(-2 * time.Minute)},
		{"run-running", run.StatusRunning, long},
		{"run-done", run.StatusCompleted, long},
	} {
		env.addRun(r.id, "headless-safe-sandbox", r.status, run.DeliverModeCommitLocal)
		setRunUpdatedAt(env.store, r.id, r.updated)
	}
	env.store.mu.Lock()
	env.store.runs[0].TenantID = "tenant-stuck"
	env.store.mu.Unlock()

	ended, err := env.svc.FailStuckQualityGates(ctx)
	if err != nil {
		t.Fatalf("FailStuckQualityGates: %v", err)
	}
	if ended != 1 {
		t.Fatalf("ended %d runs, want 1", ended)
	}

	stuck := storedRun(t, env.store, "run-stuck")
	if stuck.Status != run.StatusFailed || !strings.Contains(stuck.Error, "quality gate failed") || !strings.Contains(stuck.Error, "no quality gate result") {
		t.Fatalf("stuck run = %s %q, want failed with a missing gate result", stuck.Status, stuck.Error)
	}
	if stuck.Output != "stored output" {
		t.Errorf("stuck run output = %q, want the worker's output kept", stuck.Output)
	}
	for id, want := range map[string]run.Status{
		"run-gate-fresh": run.StatusQualityGate, "run-gate-within-deadline": run.StatusQualityGate,
		"run-running": run.StatusRunning, "run-done": run.StatusCompleted,
	} {
		if got := storedRun(t, env.store, id).Status; got != want {
			t.Errorf("%s status = %s, want %s", id, got, want)
		}
	}
	if runs, _ := env.deliverer.deliveries(); len(runs) != 0 {
		t.Fatalf("delivered %v", runs)
	}
	// A lost gate is no failed check: the workspace keeps the run's work
	// (S3 review, finding 5).
	if rw := env.checkpoints.rewound(); len(rw) != 0 {
		t.Fatalf("rollbacks = %v, want none for a gate that never reported", rw)
	}
	for _, ev := range bc.snapshot() {
		if se, ok := ev.Data.(event.RunStatusEvent); ok && se.RunID == "run-stuck" && ev.Tenant != "tenant-stuck" {
			t.Fatalf("run status event of the stuck run sent to tenant %q, want tenant-stuck", ev.Tenant)
		}
	}

	// A gate result that arrives after the watchdog changes nothing.
	passed := true
	if err := env.svc.HandleQualityGateResult(ctx, &messagequeue.QualityGateResultPayload{
		RunID: "run-stuck", TenantID: "tenant-stuck", TestsPassed: &passed, LintPassed: &passed,
	}); err != nil {
		t.Fatalf("late HandleQualityGateResult: %v", err)
	}
	if got := storedRun(t, env.store, "run-stuck").Status; got != run.StatusFailed {
		t.Fatalf("late gate result moved the run to %s", got)
	}
	if runs, _ := env.deliverer.deliveries(); len(runs) != 0 {
		t.Fatalf("late gate result delivered %v", runs)
	}
}

func TestFailStuckQualityGates_EndsARunOnce(t *testing.T) {
	env, _ := newWatchdogEnv(time.Minute)
	env.addRun("run-stuck", "headless-safe-sandbox", run.StatusQualityGate, run.DeliverModeNone)
	setRunUpdatedAt(env.store, "run-stuck", time.Now().Add(-2*time.Hour))
	var completions atomic.Int32
	env.svc.SetOnRunComplete(func(context.Context, string, run.Status) { completions.Add(1) })

	// Two replicas (or two sweeps) find the same stuck run.
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := env.svc.FailStuckQualityGates(context.Background()); err != nil {
				t.Errorf("FailStuckQualityGates: %v", err)
			}
		}()
	}
	wg.Wait()

	if got := completions.Load(); got != 1 {
		t.Fatalf("run completed %d times, want once", got)
	}
}

// runEndedElsewhereStore refuses to complete runs: another path (a passed
// gate that delivered, a cancel, another replica's watchdog) ended the run
// after it was loaded.
type runEndedElsewhereStore struct {
	*runtimeMockStore
}

func (s runEndedElsewhereStore) CompleteRun(_ context.Context, req *run.CompletionRequest) error {
	return fmt.Errorf("complete run %s: %w", req.ID, domain.ErrConflict)
}

// TestFailedGate_RollsBackOnlyTheRunItEnded: the workspace is rolled back
// only by the path that writes the failed run's record, never for a run that
// another path ended meanwhile (its workspace may hold a delivered change).
func TestFailedGate_RollsBackOnlyTheRunItEnded(t *testing.T) {
	failed := false
	for _, ended := range []bool{false, true} {
		t.Run(fmt.Sprintf("ended elsewhere %t", ended), func(t *testing.T) {
			env, bc := newWatchdogEnv(time.Minute)
			if ended {
				store := runEndedElsewhereStore{env.store}
				env.svc = service.NewRuntimeService(store, env.queue, bc, &runtimeMockEventStore{},
					service.NewPolicyService("headless-safe-sandbox", nil), &config.Runtime{StallThreshold: 5})
				env.svc.SetCheckpointService(env.checkpoints)
				env.svc.SetDeliverService(env.deliverer)
			}
			env.addRun("run-gate", "headless-safe-sandbox", run.StatusQualityGate, run.DeliverModeCommitLocal)

			if err := env.svc.HandleQualityGateResult(context.Background(), &messagequeue.QualityGateResultPayload{
				RunID: "run-gate", TestsPassed: &failed, LintPassed: &failed,
			}); err != nil {
				t.Fatalf("HandleQualityGateResult: %v", err)
			}

			if rewound := env.checkpoints.rewound(); ended != (len(rewound) == 0) {
				t.Fatalf("rollbacks = %v with the run ended elsewhere %t", rewound, ended)
			}
		})
	}
}

func TestStuckWorkWatchdog_RunOnceRunsEveryCheck(t *testing.T) {
	var calls []string
	w := service.NewStuckWorkWatchdog(time.Hour,
		service.StuckWorkCheck{Name: "failing", EndStuck: func(context.Context) (int, error) {
			calls = append(calls, "failing")
			return 0, errors.New("store down")
		}},
		service.StuckWorkCheck{Name: "working", EndStuck: func(context.Context) (int, error) {
			calls = append(calls, "working")
			return 2, nil
		}},
	)

	w.RunOnce(context.Background())

	if strings.Join(calls, ",") != "failing,working" {
		t.Fatalf("checks run = %v, want both, a failing check does not stop the others", calls)
	}
}

func TestStuckWorkWatchdog_StartRunsChecksUntilStopped(t *testing.T) {
	ran := make(chan struct{}, 16)
	w := service.NewStuckWorkWatchdog(5*time.Millisecond, service.StuckWorkCheck{Name: "count", EndStuck: func(ctx context.Context) (int, error) {
		select {
		case ran <- struct{}{}:
		default:
		}
		return 0, ctx.Err()
	}})

	stop := w.Start(context.Background())
	for range 2 {
		select {
		case <-ran:
		case <-time.After(5 * time.Second):
			t.Fatal("the watchdog did not run its check")
		}
	}
	stop()

	for len(ran) > 0 { // drop runs that were under way when stop was called
		<-ran
	}
	time.Sleep(20 * time.Millisecond)
	if len(ran) != 0 {
		t.Fatal("the watchdog ran a check after it was stopped")
	}
}
