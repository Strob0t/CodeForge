package service_test

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
)

// Review of S3 (findings 4 and 11): the watchdog must not fail gates that
// are merely queued behind other gates or redelivered, it must fail lost
// gates, and stopping it must not abort a run it is ending.

// fakeBacklog reports a fixed number of unsettled messages per subject.
type fakeBacklog struct {
	mu       sync.Mutex
	pending  int
	err      error
	subjects []string
}

func (f *fakeBacklog) Backlog(_ context.Context, subject string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.subjects = append(f.subjects, subject)
	return f.pending, f.err
}

func (f *fakeBacklog) probed() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.subjects...)
}

func TestFailStuckQualityGates_QueuedAndLostGates(t *testing.T) {
	tests := []struct {
		name    string
		backlog *fakeBacklog // nil: no probe configured
		silent  time.Duration
		wantEnd bool
	}{
		// Nothing is queued anywhere: a gate the worker has not reported on
		// for a few heartbeats is lost (request or result lost, worker gone,
		// dead-lettered).
		{name: "nothing queued, silent 3m", backlog: &fakeBacklog{}, silent: 3 * time.Minute, wantEnd: true},
		{name: "nothing queued, reported 30s ago", backlog: &fakeBacklog{}, silent: 30 * time.Second},
		// Gates are queued or being redelivered: this one may be among them,
		// until the hard cap.
		{name: "gates queued, silent 20m", backlog: &fakeBacklog{pending: 3}, silent: 20 * time.Minute},
		{name: "gates queued, silent 50m", backlog: &fakeBacklog{pending: 1}, silent: 50 * time.Minute},
		{name: "gates queued, silent 2h", backlog: &fakeBacklog{pending: 3}, silent: 2 * time.Hour, wantEnd: true},
		// Without a probe (or when it fails) only the cap applies.
		{name: "probe fails, silent 20m", backlog: &fakeBacklog{err: errors.New("nats down")}, silent: 20 * time.Minute},
		{name: "probe fails, silent 2h", backlog: &fakeBacklog{err: errors.New("nats down")}, silent: 2 * time.Hour, wantEnd: true},
		{name: "no probe, silent 20m", silent: 20 * time.Minute},
		{name: "no probe, silent 2h", silent: 2 * time.Hour, wantEnd: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env, _ := newWatchdogEnv(time.Minute) // a gate runs at most 2 x 1m
			if tc.backlog != nil {
				env.svc.SetBacklogProbe(tc.backlog)
			}
			env.addRun("run-gate", "headless-safe-sandbox", run.StatusQualityGate, run.DeliverModeNone)
			setRunUpdatedAt(env.store, "run-gate", time.Now().Add(-tc.silent))

			ended, err := env.svc.FailStuckQualityGates(context.Background())
			if err != nil {
				t.Fatalf("FailStuckQualityGates: %v", err)
			}
			status := storedRun(t, env.store, "run-gate").Status
			if tc.wantEnd != (status == run.StatusFailed) || tc.wantEnd != (ended == 1) {
				t.Fatalf("run %s (ended %d), want ended %t", status, ended, tc.wantEnd)
			}
			if tc.backlog != nil && tc.backlog.err == nil {
				probed := tc.backlog.probed()
				for _, subject := range []string{messagequeue.SubjectQualityGateRequest, messagequeue.SubjectQualityGateResult} {
					if !slices.Contains(probed, subject) {
						t.Errorf("backlog of %s not probed (probed %v)", subject, probed)
					}
				}
			}
		})
	}
}

func TestHandleHeartbeat_GateHeartbeatKeepsTheGateAlive(t *testing.T) {
	tests := []struct {
		name      string
		status    run.Status
		phase     string
		wantTouch bool
	}{
		{name: "gate heartbeat", status: run.StatusQualityGate, phase: service.HeartbeatPhaseQualityGate, wantTouch: true},
		{name: "agent heartbeat of a gated run", status: run.StatusQualityGate},
		{name: "gate heartbeat after the run ended", status: run.StatusFailed, phase: service.HeartbeatPhaseQualityGate},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env, _ := newWatchdogEnv(time.Minute)
			env.addRun("run-gate", "headless-safe-sandbox", tc.status, run.DeliverModeNone)
			old := time.Now().Add(-time.Hour)
			setRunUpdatedAt(env.store, "run-gate", old)

			if err := env.svc.HandleHeartbeat(context.Background(), &messagequeue.RunHeartbeatPayload{
				RunID: "run-gate", TenantID: "tenant-1", Phase: tc.phase, Timestamp: time.Now().UTC().Format(time.RFC3339),
			}); err != nil {
				t.Fatalf("HandleHeartbeat: %v", err)
			}

			touched := storedRun(t, env.store, "run-gate").UpdatedAt.After(old)
			if touched != tc.wantTouch {
				t.Fatalf("run touched %t, want %t", touched, tc.wantTouch)
			}
		})
	}
}

func TestEnterQualityGate_AsksForGateHeartbeats(t *testing.T) {
	env, _ := newWatchdogEnv(time.Minute)
	env.addRun("run-hb", "headless-safe-sandbox", run.StatusRunning, run.DeliverModeNone)

	completeRun(t, env, "run-hb")

	req, ok := gateRequest(t, env)
	if !ok {
		t.Fatal("no gate request")
	}
	if req.HeartbeatSeconds != 30 {
		t.Fatalf("heartbeat_seconds = %d, want 30", req.HeartbeatSeconds)
	}
}

// cancelOnCompleteStore stops the watchdog (cancels the sweep's context)
// while the first stuck run's record is written.
type cancelOnCompleteStore struct {
	*runtimeMockStore
	cancel context.CancelFunc
}

func (s cancelOnCompleteStore) CompleteRun(ctx context.Context, req *run.CompletionRequest) error {
	s.cancel()
	return s.runtimeMockStore.CompleteRun(ctx, req)
}

func TestFailStuckQualityGates_StopDoesNotAbortARunBeingEnded(t *testing.T) {
	env, bc := newWatchdogEnv(time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc := service.NewRuntimeService(cancelOnCompleteStore{env.store, cancel}, env.queue, bc, &runtimeMockEventStore{},
		service.NewPolicyService("headless-safe-sandbox", nil), newWatchdogConfig(time.Minute))
	svc.SetBacklogProbe(&fakeBacklog{})
	var mu sync.Mutex
	var completedCtxErrs []error
	svc.SetOnRunComplete(func(ctx context.Context, _ string, _ run.Status) {
		mu.Lock()
		defer mu.Unlock()
		completedCtxErrs = append(completedCtxErrs, ctx.Err())
	})
	for _, id := range []string{"run-first", "run-second"} {
		env.addRun(id, "headless-safe-sandbox", run.StatusQualityGate, run.DeliverModeNone)
	}
	setRunUpdatedAt(env.store, "run-first", time.Now().Add(-3*time.Hour))
	setRunUpdatedAt(env.store, "run-second", time.Now().Add(-2*time.Hour))

	ended, err := svc.FailStuckQualityGates(ctx)
	if err != nil {
		t.Fatalf("FailStuckQualityGates: %v", err)
	}

	if ended != 1 {
		t.Fatalf("ended %d runs, want the one under way when the watchdog stopped", ended)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(completedCtxErrs) != 1 || completedCtxErrs[0] != nil {
		t.Fatalf("run end finished with context errors %v, want it to finish with a live context", completedCtxErrs)
	}
	if got := storedRun(t, env.store, "run-second").Status; got != run.StatusQualityGate {
		t.Fatalf("run-second = %s, want it left for the next sweep after the stop", got)
	}
}

// blockingCompleteStore holds CompleteRun until released, so a test can stop
// the watchdog while a stuck run is being ended.
type blockingCompleteStore struct {
	*runtimeMockStore
	entered chan struct{}
	release chan struct{}
}

func (s blockingCompleteStore) CompleteRun(ctx context.Context, req *run.CompletionRequest) error {
	s.entered <- struct{}{}
	<-s.release
	return s.runtimeMockStore.CompleteRun(ctx, req)
}

// TestStuckWorkWatchdog_StopWaitsForARunBeingEnded goes through the shared
// periodic runner (startPeriodic): stop waits for the run under way, which
// ends with a live context, and no check runs after the stop.
func TestStuckWorkWatchdog_StopWaitsForARunBeingEnded(t *testing.T) {
	env, bc := newWatchdogEnv(time.Minute)
	store := blockingCompleteStore{env.store, make(chan struct{}, 1), make(chan struct{})}
	svc := service.NewRuntimeService(store, env.queue, bc, &runtimeMockEventStore{},
		service.NewPolicyService("headless-safe-sandbox", nil), newWatchdogConfig(time.Minute))
	svc.SetBacklogProbe(&fakeBacklog{})
	endErrs := make(chan error, 1)
	svc.SetOnRunComplete(func(ctx context.Context, _ string, _ run.Status) { endErrs <- ctx.Err() })
	env.addRun("run-stuck", "headless-safe-sandbox", run.StatusQualityGate, run.DeliverModeNone)
	setRunUpdatedAt(env.store, "run-stuck", time.Now().Add(-3*time.Hour))
	var mu sync.Mutex
	stoppedChecks := 0
	w := service.NewStuckWorkWatchdog(5*time.Millisecond,
		service.StuckWorkCheck{Name: "quality gates", EndStuck: svc.FailStuckQualityGates},
		service.StuckWorkCheck{Name: "after", EndStuck: func(ctx context.Context) (int, error) {
			if ctx.Err() != nil {
				mu.Lock()
				stoppedChecks++
				mu.Unlock()
			}
			return 0, nil
		}},
	)

	stop := w.Start(context.Background())
	select {
	case <-store.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the watchdog did not end the stuck run")
	}
	stopped := make(chan struct{})
	go func() {
		stop()
		close(stopped)
	}()
	select {
	case <-stopped:
		t.Fatal("stop returned while a run was being ended")
	case <-time.After(100 * time.Millisecond):
	}
	close(store.release)
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("stop did not return after the run was ended")
	}

	if err := <-endErrs; err != nil {
		t.Fatalf("run end finished with context error %v, want a live context", err)
	}
	if got := storedRun(t, env.store, "run-stuck").Status; got != run.StatusFailed {
		t.Fatalf("run-stuck = %s, want failed", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if stoppedChecks != 0 {
		t.Fatalf("%d checks ran with the stopped context, want none", stoppedChecks)
	}
}
