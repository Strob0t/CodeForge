package service_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/policy"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/git"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
)

// S3 follow-up 1b: a file-modifying call of a run that may have to be rolled
// back or delivered needs the run's checkpoint. When checkpointing fails in
// a git workspace the call is denied (fail closed); a workspace without git
// has no rollback base, which is recorded once per run.

// failingCheckpointer fails every checkpoint with err.
type failingCheckpointer struct {
	err error

	mu    sync.Mutex
	calls int
}

func (c *failingCheckpointer) CreateCheckpoint(_ context.Context, _, _, _, _ string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	return c.err
}
func (c *failingCheckpointer) CleanupCheckpoints(_ context.Context, _, _ string) error { return nil }
func (c *failingCheckpointer) RewindToFirst(_ context.Context, _, _ string) error      { return nil }

// editingProfile allows Edit, Write and Bash; rollback sets rollback_on_gate_fail.
func editingProfile(name string, rollback bool) policy.PolicyProfile {
	var rules []policy.PermissionRule
	for _, tool := range []string{"Edit", "Write", "Bash", "Read"} {
		rules = append(rules, policy.PermissionRule{Specifier: policy.ToolSpecifier{Tool: tool}, Decision: policy.DecisionAllow})
	}
	return policy.PolicyProfile{
		Name: name, Mode: policy.ModeDefault, Rules: rules,
		QualityGate: policy.QualityGate{RequireTestsPass: rollback, RollbackOnGateFail: rollback},
	}
}

type checkpointFailureEnv struct {
	svc         *service.RuntimeService
	queue       *runtimeMockQueue
	events      *recordingEventStore
	checkpoints *failingCheckpointer
}

func newCheckpointFailureEnv(cpErr error) *checkpointFailureEnv {
	_, store, queue, bc := newRuntimeTestEnv()
	events := &recordingEventStore{}
	policySvc := service.NewPolicyService("edit-rollback", []policy.PolicyProfile{
		editingProfile("edit-rollback", true), editingProfile("edit-no-rollback", false),
	})
	svc := service.NewRuntimeService(store, queue, bc, events, policySvc, &config.Runtime{StallThreshold: 5})
	checkpoints := &failingCheckpointer{err: cpErr}
	svc.SetCheckpointService(checkpoints)
	for _, r := range []struct {
		id, profile string
		deliver     run.DeliverMode
	}{
		{"run-rollback", "edit-rollback", run.DeliverModeNone},
		{"run-plain", "edit-no-rollback", run.DeliverModeNone},
		{"run-delivers", "edit-no-rollback", run.DeliverModePatch},
	} {
		setStoredRun(store, &run.Run{ID: r.id, TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1",
			PolicyProfile: r.profile, Status: run.StatusRunning, DeliverMode: r.deliver})
	}
	return &checkpointFailureEnv{svc: svc, queue: queue, events: events, checkpoints: checkpoints}
}

func (e *checkpointFailureEnv) call(t *testing.T, runID, tool, callID string) (decision, reason string) {
	t.Helper()
	return toolCallDecision(t, e.svc, e.queue, &messagequeue.ToolCallRequestPayload{
		RunID: runID, CallID: callID, Tool: tool, Path: "main.go", Command: "go build ./...",
	})
}

func TestToolCall_FailedCheckpointDeniesWhenTheRunNeedsItsBase(t *testing.T) {
	gitFailure := errors.New("checkpoint: git write-tree: fatal: unable to write new index file")
	unsafeRepo := fmt.Errorf("checkpoint: %w", git.ErrUnsafeRepository)
	tests := []struct {
		name      string
		cpErr     error
		runID     string
		tool      string
		wantAllow bool
	}{
		{name: "rollback configured, git failure", cpErr: gitFailure, runID: "run-rollback", tool: "Edit"},
		{name: "rollback configured, unsafe repository", cpErr: unsafeRepo, runID: "run-rollback", tool: "Bash"},
		{name: "delivery configured, git failure", cpErr: gitFailure, runID: "run-delivers", tool: "Write"},
		{name: "neither rollback nor delivery", cpErr: gitFailure, runID: "run-plain", tool: "Edit", wantAllow: true},
		{name: "a read needs no checkpoint", cpErr: gitFailure, runID: "run-rollback", tool: "Read", wantAllow: true},
		{name: "checkpoint created", runID: "run-rollback", tool: "Edit", wantAllow: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := newCheckpointFailureEnv(tc.cpErr)
			decision, reason := env.call(t, tc.runID, tc.tool, "call-1")
			if tc.wantAllow {
				if decision != string(policy.DecisionAllow) {
					t.Fatalf("decision = %s (%s), want allow", decision, reason)
				}
				return
			}
			if decision != string(policy.DecisionDeny) || !strings.Contains(reason, "checkpoint") {
				t.Fatalf("decision = %s (%q), want deny with a checkpoint reason", decision, reason)
			}
			actions, types := recordedGate(env.events)
			if !slicesContain(actions, "policy.denied") || !slicesContain(types, "run.toolcall.denied") {
				t.Fatalf("audit %v / events %v, want the denial recorded", actions, types)
			}
		})
	}
}

func TestToolCall_WorkspaceWithoutGitProceedsAndRecordsOnce(t *testing.T) {
	env := newCheckpointFailureEnv(fmt.Errorf("checkpoint: %w", git.ErrNotRepository))

	for i := range 3 {
		decision, reason := env.call(t, "run-rollback", "Edit", fmt.Sprintf("call-%d", i))
		if decision != string(policy.DecisionAllow) {
			t.Fatalf("call %d: decision = %s (%s), want allow in a workspace without git", i, decision, reason)
		}
	}
	actions, _ := recordedGate(env.events)
	if n := countOf(actions, "checkpoint.unavailable"); n != 1 {
		t.Fatalf("checkpoint.unavailable recorded %d times (%v), want once per run", n, actions)
	}
	if env.checkpoints.calls != 3 {
		t.Fatalf("checkpoints tried %d times, want every call", env.checkpoints.calls)
	}
}

func slicesContain(list []string, want string) bool {
	return countOf(list, want) > 0
}
