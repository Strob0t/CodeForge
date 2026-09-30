package service_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/policy"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
)

// KI-26: a run that completes delivers (deliver_mode honored) whether or not
// its policy has quality gates, a failed gate fails the run and never
// delivers, and completions of runs that already ended or wait for their gate
// change nothing.

// recordingDeliverer records deliveries and the stored status of the run at
// the moment it is delivered.
type recordingDeliverer struct {
	store *runtimeMockStore

	mu       sync.Mutex
	runs     []string
	statuses []run.Status
}

func (d *recordingDeliverer) Deliver(ctx context.Context, r *run.Run, _ string) (*service.DeliveryResult, error) {
	stored, err := d.store.GetRun(ctx, r.ID)
	if err != nil {
		return nil, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.runs = append(d.runs, r.ID)
	d.statuses = append(d.statuses, stored.Status)
	return &service.DeliveryResult{Mode: r.DeliverMode}, nil
}

func (d *recordingDeliverer) deliveries() ([]string, []run.Status) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.runs...), append([]run.Status(nil), d.statuses...)
}

// recordingCheckpointer records rollbacks and cleanups.
type recordingCheckpointer struct {
	mu        sync.Mutex
	rewinds   []string
	cleanups  []string
	rewindErr error
}

func (c *recordingCheckpointer) CreateCheckpoint(_ context.Context, _, _, _, _ string) error {
	return nil
}

func (c *recordingCheckpointer) CleanupCheckpoints(_ context.Context, runID, _ string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cleanups = append(c.cleanups, runID)
	return nil
}

func (c *recordingCheckpointer) RewindToFirst(_ context.Context, runID, _ string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rewinds = append(c.rewinds, runID)
	return c.rewindErr
}

func (c *recordingCheckpointer) rewound() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.rewinds...)
}

// gateNoRollback is a profile with a test gate and no rollback.
var gateNoRollback = policy.PolicyProfile{
	Name:        "gate-no-rollback",
	Mode:        policy.ModeDefault,
	QualityGate: policy.QualityGate{RequireTestsPass: true},
}

type gateDeliveryEnv struct {
	svc         *service.RuntimeService
	store       *runtimeMockStore
	queue       *runtimeMockQueue
	deliverer   *recordingDeliverer
	checkpoints *recordingCheckpointer
}

func newGateDeliveryEnv() *gateDeliveryEnv {
	_, store, queue, bc := newRuntimeTestEnv()
	policySvc := service.NewPolicyService("headless-safe-sandbox", []policy.PolicyProfile{gateNoRollback})
	svc := service.NewRuntimeService(store, queue, bc, &runtimeMockEventStore{}, policySvc, &config.Runtime{
		StallThreshold:     5,
		DefaultTestCommand: "go test ./...",
		DefaultLintCommand: "golangci-lint run ./...",
	})
	env := &gateDeliveryEnv{
		svc: svc, store: store, queue: queue,
		deliverer:   &recordingDeliverer{store: store},
		checkpoints: &recordingCheckpointer{},
	}
	svc.SetDeliverService(env.deliverer)
	svc.SetCheckpointService(env.checkpoints)
	return env
}

func (e *gateDeliveryEnv) addRun(id, profile string, status run.Status, deliver run.DeliverMode) {
	setStoredRun(e.store, &run.Run{
		ID: id, TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1",
		PolicyProfile: profile, Status: status, DeliverMode: deliver, Output: "stored output",
	})
}

func TestHandleRunComplete_RunWithoutGatesDelivers(t *testing.T) {
	tests := []struct {
		name          string
		profile       string
		deliver       run.DeliverMode
		workerStatus  string
		workerError   string
		wantStatus    run.Status
		wantDelivered bool
	}{
		{name: "plan-readonly delivers", profile: "plan-readonly", deliver: run.DeliverModeCommitLocal, workerStatus: "completed", wantStatus: run.StatusCompleted, wantDelivered: true},
		{name: "trusted-mount-autonomous delivers", profile: "trusted-mount-autonomous", deliver: run.DeliverModeBranch, workerStatus: "completed", wantStatus: run.StatusCompleted, wantDelivered: true},
		{name: "supervised-ask-all delivers", profile: "supervised-ask-all", deliver: run.DeliverModePatch, workerStatus: "completed", wantStatus: run.StatusCompleted, wantDelivered: true},
		{name: "no status and no error delivers", profile: "plan-readonly", deliver: run.DeliverModePR, workerStatus: "", wantStatus: run.StatusCompleted, wantDelivered: true},
		{name: "no deliver mode", profile: "plan-readonly", deliver: run.DeliverModeNone, workerStatus: "completed", wantStatus: run.StatusCompleted},
		{name: "failed run", profile: "plan-readonly", deliver: run.DeliverModeCommitLocal, workerStatus: "failed", workerError: "boom", wantStatus: run.StatusFailed},
		{name: "error without status", profile: "plan-readonly", deliver: run.DeliverModeCommitLocal, workerError: "boom", wantStatus: run.StatusFailed},
		{name: "cancelled by the worker", profile: "plan-readonly", deliver: run.DeliverModeCommitLocal, workerStatus: "cancelled", wantStatus: run.StatusCancelled},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := newGateDeliveryEnv()
			env.addRun("run-nogate", tc.profile, run.StatusRunning, tc.deliver)

			if err := env.svc.HandleRunComplete(context.Background(), &messagequeue.RunCompletePayload{
				RunID: "run-nogate", Status: tc.workerStatus, Error: tc.workerError, Output: "done",
			}); err != nil {
				t.Fatalf("HandleRunComplete: %v", err)
			}

			if r := storedRun(t, env.store, "run-nogate"); r.Status != tc.wantStatus {
				t.Fatalf("run status = %s, want %s", r.Status, tc.wantStatus)
			}
			if _, gated := env.queue.lastMessage(messagequeue.SubjectQualityGateRequest); gated {
				t.Fatal("a quality gate was requested for a policy without gates")
			}
			runs, statuses := env.deliverer.deliveries()
			if !tc.wantDelivered {
				if len(runs) != 0 {
					t.Fatalf("delivered %v, want no delivery", runs)
				}
				return
			}
			if len(runs) != 1 || statuses[0] != run.StatusCompleted {
				t.Fatalf("deliveries = %v with stored statuses %v, want one delivery of the completed run", runs, statuses)
			}
		})
	}
}

func TestHandleQualityGateResult_PassedGateDeliversOnce(t *testing.T) {
	env := newGateDeliveryEnv()
	ctx := context.Background()
	env.addRun("run-pass", "headless-safe-sandbox", run.StatusQualityGate, run.DeliverModeCommitLocal)

	passed := true
	result := &messagequeue.QualityGateResultPayload{RunID: "run-pass", TestsPassed: &passed, LintPassed: &passed}
	for range 2 { // the result subject is delivered at least once
		if err := env.svc.HandleQualityGateResult(ctx, result); err != nil {
			t.Fatalf("HandleQualityGateResult: %v", err)
		}
	}

	if r := storedRun(t, env.store, "run-pass"); r.Status != run.StatusCompleted {
		t.Fatalf("run status = %s, want completed", r.Status)
	}
	runs, statuses := env.deliverer.deliveries()
	if len(runs) != 1 || statuses[0] != run.StatusCompleted {
		t.Fatalf("deliveries = %v with stored statuses %v, want one delivery of the completed run", runs, statuses)
	}
	if rw := env.checkpoints.rewound(); len(rw) != 0 {
		t.Fatalf("rolled back %v after a passed gate", rw)
	}
}

func TestHandleQualityGateResult_FailedGateFailsTheRunAndNeverDelivers(t *testing.T) {
	passed, failed := true, false
	tests := []struct {
		name         string
		profile      string
		result       messagequeue.QualityGateResultPayload
		wantRollback bool
		wantError    string
	}{
		{name: "tests fail, rollback", profile: "headless-safe-sandbox", result: messagequeue.QualityGateResultPayload{TestsPassed: &failed, LintPassed: &passed}, wantRollback: true, wantError: "quality gate failed"},
		{name: "lint fails, rollback", profile: "headless-safe-sandbox", result: messagequeue.QualityGateResultPayload{TestsPassed: &passed, LintPassed: &failed}, wantRollback: true, wantError: "quality gate failed"},
		{name: "gate error, rollback", profile: "headless-safe-sandbox", result: messagequeue.QualityGateResultPayload{Error: "runner crashed"}, wantRollback: true, wantError: "runner crashed"},
		{name: "tests fail, no rollback", profile: gateNoRollback.Name, result: messagequeue.QualityGateResultPayload{TestsPassed: &failed}, wantError: "quality gate failed"},
		{name: "preset without rollback", profile: "headless-permissive-sandbox", result: messagequeue.QualityGateResultPayload{TestsPassed: &failed}, wantError: "quality gate failed"},
		{name: "gate error, no rollback", profile: gateNoRollback.Name, result: messagequeue.QualityGateResultPayload{Error: "runner crashed"}, wantError: "runner crashed"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := newGateDeliveryEnv()
			env.addRun("run-fail", tc.profile, run.StatusQualityGate, run.DeliverModeCommitLocal)

			result := tc.result
			result.RunID = "run-fail"
			if err := env.svc.HandleQualityGateResult(context.Background(), &result); err != nil {
				t.Fatalf("HandleQualityGateResult: %v", err)
			}

			r := storedRun(t, env.store, "run-fail")
			if r.Status != run.StatusFailed {
				t.Fatalf("run status = %s, want failed", r.Status)
			}
			if !strings.Contains(r.Error, tc.wantError) {
				t.Errorf("run error = %q, want it to contain %q", r.Error, tc.wantError)
			}
			if r.Output != "stored output" {
				t.Errorf("run output = %q, want the worker's output kept", r.Output)
			}
			if runs, _ := env.deliverer.deliveries(); len(runs) != 0 {
				t.Fatalf("delivered %v after a failed gate", runs)
			}
			rewound := env.checkpoints.rewound()
			if tc.wantRollback != (len(rewound) == 1) {
				t.Fatalf("rollbacks = %v, want rollback %t", rewound, tc.wantRollback)
			}
		})
	}
}

func TestHandleRunComplete_IgnoresRunsThatAlreadyEndedOrWaitForTheirGate(t *testing.T) {
	statuses := []run.Status{run.StatusCompleted, run.StatusFailed, run.StatusCancelled, run.StatusTimeout, run.StatusQualityGate}
	completions := []messagequeue.RunCompletePayload{
		{Status: "completed", Output: "late output"},
		{Status: "failed", Error: "late failure"},
		{Error: "late error"},
	}
	for _, status := range statuses {
		for _, profile := range []string{"headless-safe-sandbox", "plan-readonly"} {
			for _, completion := range completions {
				t.Run(string(status)+"/"+profile+"/"+completion.Status+completion.Error, func(t *testing.T) {
					env := newGateDeliveryEnv()
					env.addRun("run-ended", profile, status, run.DeliverModeCommitLocal)

					payload := completion
					payload.RunID = "run-ended"
					if err := env.svc.HandleRunComplete(context.Background(), &payload); err != nil {
						t.Fatalf("HandleRunComplete: %v", err)
					}

					r := storedRun(t, env.store, "run-ended")
					if r.Status != status || r.Output != "stored output" || r.Error != "" {
						t.Fatalf("run = %s %q %q, want %s with its stored outcome", r.Status, r.Output, r.Error, status)
					}
					if runs, _ := env.deliverer.deliveries(); len(runs) != 0 {
						t.Fatalf("delivered %v on a late completion", runs)
					}
					if _, gated := env.queue.lastMessage(messagequeue.SubjectQualityGateRequest); gated {
						t.Fatal("a late completion requested a quality gate")
					}
				})
			}
		}
	}
}
