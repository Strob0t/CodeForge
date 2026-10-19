package service_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/policy"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
)

// Review of S3 (findings 5 and 7): only a check that ran and failed rolls the
// workspace back and counts against the agent; a gate that could not run
// fails the run and leaves both alone. Gate audit entries, events and
// broadcasts come only from the path that ended the run.

// gateOutcomeStore records agent statistics and can let another path end a
// run just before CompleteRun (a cancel racing the gate result).
type gateOutcomeStore struct {
	*runtimeMockStore

	mu             sync.Mutex
	stats          []bool // success flag of each IncrementAgentStats call
	endedElsewhere run.Status
}

func (s *gateOutcomeStore) IncrementAgentStats(_ context.Context, _ string, _ float64, success bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stats = append(s.stats, success)
	return nil
}

func (s *gateOutcomeStore) CompleteRun(ctx context.Context, req *run.CompletionRequest) error {
	s.mu.Lock()
	elsewhere := s.endedElsewhere
	s.mu.Unlock()
	if elsewhere != "" {
		setStoredStatus(s.runtimeMockStore, req.ID, elsewhere)
	}
	return s.runtimeMockStore.CompleteRun(ctx, req)
}

func (s *gateOutcomeStore) agentStats() []bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]bool(nil), s.stats...)
}

// recordedGate returns the audit actions and run event types the handlers
// (synchronous here) recorded.
func recordedGate(s *recordingEventStore) (actions, types []string) {
	for i := range s.audits {
		actions = append(actions, s.audits[i].Action)
	}
	for i := range s.events {
		types = append(types, string(s.events[i].Type))
	}
	return actions, types
}

type gateOutcomeEnv struct {
	svc         *service.RuntimeService
	store       *gateOutcomeStore
	events      *recordingEventStore
	bc          *runtimeMockBroadcaster
	checkpoints *recordingCheckpointer
}

func newGateOutcomeEnv(cfg *config.Runtime) *gateOutcomeEnv {
	_, mockStore, queue, bc := newRuntimeTestEnv()
	store := &gateOutcomeStore{runtimeMockStore: mockStore}
	events := &recordingEventStore{}
	policySvc := service.NewPolicyService("headless-safe-sandbox", []policy.PolicyProfile{gateNoRollback})
	svc := service.NewRuntimeService(store, queue, bc, events, policySvc, cfg)
	checkpoints := &recordingCheckpointer{}
	svc.SetCheckpointService(checkpoints)
	svc.SetDeliverService(&recordingDeliverer{store: mockStore, checkpoints: checkpoints})
	return &gateOutcomeEnv{svc: svc, store: store, events: events, bc: bc, checkpoints: checkpoints}
}

func defaultGateConfig() *config.Runtime {
	return &config.Runtime{StallThreshold: 5, DefaultTestCommand: "go test ./...", DefaultLintCommand: "golangci-lint run ./..."}
}

func TestQualityGate_OnlyAFailedCheckRollsBackAndCountsAgainstTheAgent(t *testing.T) {
	passed, failed := true, false
	tests := []struct {
		name          string
		profile       string
		result        messagequeue.QualityGateResultPayload
		wantError     string
		wantAgentFail bool // rolled back (the profile rolls back) and counted as the agent's failure
	}{
		{name: "tests failed", result: messagequeue.QualityGateResultPayload{TestsPassed: &failed, LintPassed: &passed}, wantError: "tests failed", wantAgentFail: true},
		{name: "lint failed", result: messagequeue.QualityGateResultPayload{TestsPassed: &passed, LintPassed: &failed}, wantError: "lint failed", wantAgentFail: true},
		{name: "tests failed, lint could not run", result: messagequeue.QualityGateResultPayload{TestsPassed: &failed, Error: "lint: command not found"}, wantError: "tests failed", wantAgentFail: true},
		{name: "gate error", result: messagequeue.QualityGateResultPayload{Error: "runner crashed"}, wantError: "runner crashed"},
		{name: "check could not run", result: messagequeue.QualityGateResultPayload{LintPassed: &passed, Error: "test: command not allowed"}, wantError: "command not allowed"},
		{name: "required check without result", result: messagequeue.QualityGateResultPayload{LintPassed: &passed}, wantError: "no test result"},
		{name: "watchdog", result: messagequeue.QualityGateResultPayload{Error: "no quality gate result within 21m0s"}, wantError: "no quality gate result"},
		{name: "unknown profile", profile: "deleted-profile", result: messagequeue.QualityGateResultPayload{TestsPassed: &failed}, wantError: "unknown policy profile"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := newGateOutcomeEnv(defaultGateConfig())
			profile := tc.profile
			if profile == "" {
				profile = "headless-safe-sandbox" // tests and lint, rollback on gate failure
			}
			setStoredRun(env.store.runtimeMockStore, &run.Run{
				ID: "run-gate", TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1",
				PolicyProfile: profile, Status: run.StatusQualityGate, Output: "stored output",
			})
			result := tc.result
			result.RunID = "run-gate"

			if err := env.svc.HandleQualityGateResult(context.Background(), &result); err != nil {
				t.Fatalf("HandleQualityGateResult: %v", err)
			}

			r := storedRun(t, env.store.runtimeMockStore, "run-gate")
			if r.Status != run.StatusFailed || !strings.Contains(r.Error, tc.wantError) {
				t.Fatalf("run = %s %q, want failed with %q", r.Status, r.Error, tc.wantError)
			}
			rewound := env.checkpoints.rewound()
			stats := env.store.agentStats()
			if tc.wantAgentFail {
				if len(rewound) != 1 {
					t.Errorf("rollbacks = %v, want the failed check rolled back", rewound)
				}
				if len(stats) != 1 || stats[0] {
					t.Errorf("agent stats = %v, want one failure", stats)
				}
				return
			}
			if len(rewound) != 0 {
				t.Errorf("rollbacks = %v, want none: no check ran and failed", rewound)
			}
			if len(stats) != 0 {
				t.Errorf("agent stats = %v, want none: the gate, not the agent, failed", stats)
			}
		})
	}
}

func TestEnterQualityGate_InfrastructureFailureLeavesWorkspaceAndAgentAlone(t *testing.T) {
	tests := []struct {
		name      string
		projectID string
		cfg       *config.Runtime
		wantError string
	}{
		{name: "project unavailable", projectID: "proj-missing", cfg: defaultGateConfig(), wantError: "project unavailable"},
		{name: "no gate commands", projectID: "proj-1", cfg: &config.Runtime{StallThreshold: 5}, wantError: "no command for the required checks"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := newGateOutcomeEnv(tc.cfg)
			setStoredRun(env.store.runtimeMockStore, &run.Run{
				ID: "run-gate", TaskID: "task-1", AgentID: "agent-1", ProjectID: tc.projectID,
				PolicyProfile: "headless-safe-sandbox", Status: run.StatusRunning,
			})

			if err := env.svc.HandleRunComplete(context.Background(), &messagequeue.RunCompletePayload{RunID: "run-gate", Status: "completed", Output: "done"}); err != nil {
				t.Fatalf("HandleRunComplete: %v", err)
			}

			r := storedRun(t, env.store.runtimeMockStore, "run-gate")
			if r.Status != run.StatusFailed || !strings.Contains(r.Error, tc.wantError) {
				t.Fatalf("run = %s %q, want failed with %q", r.Status, r.Error, tc.wantError)
			}
			if rw := env.checkpoints.rewound(); len(rw) != 0 {
				t.Errorf("rollbacks = %v, want none", rw)
			}
			if stats := env.store.agentStats(); len(stats) != 0 {
				t.Errorf("agent stats = %v, want none", stats)
			}
		})
	}
}

func TestQualityGateResult_AnnouncedOnlyByThePathThatEndsTheRun(t *testing.T) {
	passed, failed := true, false
	results := map[string]messagequeue.QualityGateResultPayload{
		"passed": {TestsPassed: &passed, LintPassed: &passed},
		"failed": {TestsPassed: &failed, LintPassed: &passed},
		"error":  {Error: "runner crashed"},
	}
	for name, res := range results {
		t.Run(name, func(t *testing.T) {
			env := newGateOutcomeEnv(defaultGateConfig())
			setStoredRun(env.store.runtimeMockStore, &run.Run{
				ID: "run-gate", TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1",
				PolicyProfile: "headless-safe-sandbox", Status: run.StatusQualityGate,
			})
			// A cancel ends the run while the result is handled.
			env.store.endedElsewhere = run.StatusCancelled
			result := res
			result.RunID = "run-gate"

			if err := env.svc.HandleQualityGateResult(context.Background(), &result); err != nil {
				t.Fatalf("HandleQualityGateResult: %v", err)
			}

			if r := storedRun(t, env.store.runtimeMockStore, "run-gate"); r.Status != run.StatusCancelled {
				t.Fatalf("run status = %s, want the cancel's", r.Status)
			}
			actions, types := recordedGate(env.events)
			for _, a := range actions {
				if strings.HasPrefix(a, "qualitygate.") || strings.HasPrefix(a, "run.") {
					t.Errorf("audit entry %q from the path that lost the run's end", a)
				}
			}
			for _, typ := range types {
				if strings.Contains(typ, "quality_gate") || typ == string(event.TypeRunCompleted) {
					t.Errorf("run event %q from the path that lost the run's end", typ)
				}
			}
			for _, e := range env.bc.snapshot() {
				if e.EventType == event.EventQualityGate {
					t.Errorf("broadcast %s %+v from the path that lost the run's end", e.EventType, e.Data)
				}
			}
			if rw := env.checkpoints.rewound(); len(rw) != 0 {
				t.Errorf("rollbacks = %v by the path that lost the run's end", rw)
			}
		})
	}
}

func TestQualityGateResult_WinningPathAnnounces(t *testing.T) {
	passed, failed := true, false
	tests := []struct {
		name       string
		result     messagequeue.QualityGateResultPayload
		wantAction string
	}{
		{name: "passed", result: messagequeue.QualityGateResultPayload{TestsPassed: &passed, LintPassed: &passed}, wantAction: "qualitygate.passed"},
		{name: "failed", result: messagequeue.QualityGateResultPayload{TestsPassed: &failed, LintPassed: &passed}, wantAction: "qualitygate.failed"},
		{name: "error", result: messagequeue.QualityGateResultPayload{Error: "runner crashed"}, wantAction: "qualitygate.failed"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := newGateOutcomeEnv(defaultGateConfig())
			setStoredRun(env.store.runtimeMockStore, &run.Run{
				ID: "run-gate", TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1",
				PolicyProfile: "headless-safe-sandbox", Status: run.StatusQualityGate,
			})
			result := tc.result
			result.RunID = "run-gate"
			if err := env.svc.HandleQualityGateResult(context.Background(), &result); err != nil {
				t.Fatalf("HandleQualityGateResult: %v", err)
			}
			actions, _ := recordedGate(env.events)
			if n := countOf(actions, tc.wantAction); n != 1 {
				t.Fatalf("audit %q recorded %d times (%v), want once", tc.wantAction, n, actions)
			}
			gateBroadcasts := 0
			for _, e := range env.bc.snapshot() {
				if e.EventType == event.EventQualityGate {
					gateBroadcasts++
				}
			}
			if gateBroadcasts != 1 {
				t.Fatalf("quality gate broadcasts = %d, want 1", gateBroadcasts)
			}
		})
	}
}

func countOf(items []string, want string) int {
	n := 0
	for _, it := range items {
		if it == want {
			n++
		}
	}
	return n
}
