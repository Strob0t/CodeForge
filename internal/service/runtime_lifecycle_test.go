package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/agent"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/policy"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// ============================================================================
// Table-driven tests for runtime lifecycle functions
// ============================================================================

// --- checkTermination ---

func TestCheckTermination(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		run         *run.Run
		profile     *policy.PolicyProfile
		runtimeCfg  *config.Runtime
		setupHB     func(svc *RuntimeService) // optional heartbeat setup
		wantEmpty   bool                      // true = expect "" (no termination)
		wantContain string                    // substring expected in reason
	}{
		{
			name: "max steps reached exactly",
			run:  &run.Run{StepCount: 50, StartedAt: time.Now()},
			profile: &policy.PolicyProfile{
				Termination: policy.TerminationCondition{MaxSteps: 50},
			},
			wantContain: "steps",
		},
		{
			name: "max steps exceeded",
			run:  &run.Run{StepCount: 55, StartedAt: time.Now()},
			profile: &policy.PolicyProfile{
				Termination: policy.TerminationCondition{MaxSteps: 50},
			},
			wantContain: "steps",
		},
		{
			name: "max steps not reached",
			run:  &run.Run{StepCount: 49, StartedAt: time.Now()},
			profile: &policy.PolicyProfile{
				Termination: policy.TerminationCondition{MaxSteps: 50},
			},
			wantEmpty: true,
		},
		{
			name: "max steps zero means disabled",
			run:  &run.Run{StepCount: 999, StartedAt: time.Now()},
			profile: &policy.PolicyProfile{
				Termination: policy.TerminationCondition{MaxSteps: 0},
			},
			wantEmpty: true,
		},
		{
			name: "max cost reached exactly",
			run:  &run.Run{CostUSD: 10.0, StartedAt: time.Now()},
			profile: &policy.PolicyProfile{
				Termination: policy.TerminationCondition{MaxCost: 10.0},
			},
			wantContain: "cost",
		},
		{
			name: "max cost exceeded",
			run:  &run.Run{CostUSD: 10.01, StartedAt: time.Now()},
			profile: &policy.PolicyProfile{
				Termination: policy.TerminationCondition{MaxCost: 10.0},
			},
			wantContain: "cost",
		},
		{
			name: "max cost not reached",
			run:  &run.Run{CostUSD: 9.99, StartedAt: time.Now()},
			profile: &policy.PolicyProfile{
				Termination: policy.TerminationCondition{MaxCost: 10.0},
			},
			wantEmpty: true,
		},
		{
			name: "max cost zero means disabled",
			run:  &run.Run{CostUSD: 999.99, StartedAt: time.Now()},
			profile: &policy.PolicyProfile{
				Termination: policy.TerminationCondition{MaxCost: 0},
			},
			wantEmpty: true,
		},
		{
			name: "timeout exceeded",
			run:  &run.Run{StartedAt: time.Now().Add(-120 * time.Second)},
			profile: &policy.PolicyProfile{
				Termination: policy.TerminationCondition{TimeoutSeconds: 60},
			},
			wantContain: "timeout",
		},
		{
			name: "timeout not reached",
			run:  &run.Run{StartedAt: time.Now().Add(-10 * time.Second)},
			profile: &policy.PolicyProfile{
				Termination: policy.TerminationCondition{TimeoutSeconds: 60},
			},
			wantEmpty: true,
		},
		{
			name: "timeout zero means disabled",
			run:  &run.Run{StartedAt: time.Now().Add(-10 * time.Minute)},
			profile: &policy.PolicyProfile{
				Termination: policy.TerminationCondition{TimeoutSeconds: 0},
			},
			wantEmpty: true,
		},
		{
			name: "heartbeat timeout triggered",
			run:  &run.Run{ID: "run-hb-trigger", StartedAt: time.Now()},
			profile: &policy.PolicyProfile{
				Termination: policy.TerminationCondition{},
			},
			runtimeCfg: &config.Runtime{HeartbeatTimeout: 30 * time.Second},
			setupHB: func(svc *RuntimeService) {
				svc.state.SetHeartbeat("run-hb-trigger", time.Now().Add(-2*time.Minute))
			},
			wantContain: "heartbeat",
		},
		{
			name: "heartbeat recent - no timeout",
			run:  &run.Run{ID: "run-hb-recent", StartedAt: time.Now()},
			profile: &policy.PolicyProfile{
				Termination: policy.TerminationCondition{},
			},
			runtimeCfg: &config.Runtime{HeartbeatTimeout: 30 * time.Second},
			setupHB: func(svc *RuntimeService) {
				svc.state.SetHeartbeat("run-hb-recent", time.Now())
			},
			wantEmpty: true,
		},
		{
			name: "heartbeat timeout disabled when zero",
			run:  &run.Run{ID: "run-hb-disabled", StartedAt: time.Now()},
			profile: &policy.PolicyProfile{
				Termination: policy.TerminationCondition{},
			},
			runtimeCfg: &config.Runtime{HeartbeatTimeout: 0},
			setupHB: func(svc *RuntimeService) {
				svc.state.SetHeartbeat("run-hb-disabled", time.Now().Add(-10*time.Minute))
			},
			wantEmpty: true,
		},
		{
			name: "no heartbeat stored - no heartbeat timeout",
			run:  &run.Run{ID: "run-hb-none", StartedAt: time.Now()},
			profile: &policy.PolicyProfile{
				Termination: policy.TerminationCondition{},
			},
			runtimeCfg: &config.Runtime{HeartbeatTimeout: 30 * time.Second},
			wantEmpty:  true,
		},
		{
			name: "all limits zero - no termination",
			run: &run.Run{
				StepCount: 1000,
				CostUSD:   500.0,
				StartedAt: time.Now().Add(-10 * time.Minute),
			},
			profile: &policy.PolicyProfile{
				Termination: policy.TerminationCondition{
					MaxSteps:       0,
					MaxCost:        0,
					TimeoutSeconds: 0,
				},
			},
			wantEmpty: true,
		},
		{
			name: "absolute timeout safety net fires",
			run:  &run.Run{StartedAt: time.Now().Add(-2 * time.Hour)},
			profile: &policy.PolicyProfile{
				Termination: policy.TerminationCondition{},
			},
			wantContain: "absolute execution timeout",
		},
		{
			name: "max steps takes priority over timeout",
			run:  &run.Run{StepCount: 10, StartedAt: time.Now().Add(-5 * time.Minute)},
			profile: &policy.PolicyProfile{
				Termination: policy.TerminationCondition{
					MaxSteps:       10,
					TimeoutSeconds: 60,
				},
			},
			wantContain: "steps",
		},
		{
			name: "max cost takes priority over timeout when steps not hit",
			run:  &run.Run{StepCount: 5, CostUSD: 20.0, StartedAt: time.Now().Add(-5 * time.Minute)},
			profile: &policy.PolicyProfile{
				Termination: policy.TerminationCondition{
					MaxSteps:       100,
					MaxCost:        20.0,
					TimeoutSeconds: 60,
				},
			},
			wantContain: "cost",
		},
		{
			name: "zero step count with limit",
			run:  &run.Run{StepCount: 0, StartedAt: time.Now()},
			profile: &policy.PolicyProfile{
				Termination: policy.TerminationCondition{MaxSteps: 10},
			},
			wantEmpty: true,
		},
		{
			name: "zero cost with limit",
			run:  &run.Run{CostUSD: 0, StartedAt: time.Now()},
			profile: &policy.PolicyProfile{
				Termination: policy.TerminationCondition{MaxCost: 5.0},
			},
			wantEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := tt.runtimeCfg
			if cfg == nil {
				cfg = &config.Runtime{}
			}
			svc := &RuntimeService{runtimeCfg: cfg, state: NewRunStateManager()}

			if tt.setupHB != nil {
				tt.setupHB(svc)
			}

			got := svc.checkTermination(tt.run, tt.profile)

			if tt.wantEmpty {
				if got != "" {
					t.Errorf("expected no termination, got %q", got)
				}
				return
			}

			if got == "" {
				t.Fatalf("expected termination reason containing %q, got empty", tt.wantContain)
			}
			if !strings.Contains(got, tt.wantContain) {
				t.Errorf("expected reason containing %q, got %q", tt.wantContain, got)
			}
		})
	}
}

// --- cancelRunWithReason ---

func TestCancelRunWithReason(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		runs           map[string]*run.Run
		runID          string
		reason         string
		wantErr        bool
		wantErrContain string
		wantStatus     run.Status // expected status in completedRuns
		wantSkipped    bool       // run should NOT appear in completedRuns
		wantNATSCancel bool       // expect a cancel message on NATS
		wantBroadcast  bool       // expect WS broadcast
	}{
		{
			name: "running run is cancelled with timeout",
			runs: map[string]*run.Run{
				"run-1": {
					ID: "run-1", TaskID: "t1", AgentID: "a1", ProjectID: "p1",
					Status: run.StatusRunning, StartedAt: time.Now(),
				},
			},
			runID:          "run-1",
			reason:         "max steps reached (50/50)",
			wantStatus:     run.StatusTimeout,
			wantNATSCancel: true,
			wantBroadcast:  true,
		},
		{
			name: "pending run is cancelled",
			runs: map[string]*run.Run{
				"run-2": {
					ID: "run-2", TaskID: "t1", AgentID: "a1", ProjectID: "p1",
					Status: run.StatusPending, StartedAt: time.Now(),
				},
			},
			runID:          "run-2",
			reason:         "budget exceeded",
			wantStatus:     run.StatusTimeout,
			wantNATSCancel: true,
			wantBroadcast:  true,
		},
		{
			name: "completed run is skipped",
			runs: map[string]*run.Run{
				"run-done": {
					ID: "run-done", Status: run.StatusCompleted, StartedAt: time.Now(),
				},
			},
			runID:       "run-done",
			reason:      "test",
			wantSkipped: true,
		},
		{
			name: "failed run is skipped",
			runs: map[string]*run.Run{
				"run-fail": {
					ID: "run-fail", Status: run.StatusFailed, StartedAt: time.Now(),
				},
			},
			runID:       "run-fail",
			reason:      "test",
			wantSkipped: true,
		},
		{
			name: "cancelled run is skipped",
			runs: map[string]*run.Run{
				"run-cancel": {
					ID: "run-cancel", Status: run.StatusCancelled, StartedAt: time.Now(),
				},
			},
			runID:       "run-cancel",
			reason:      "test",
			wantSkipped: true,
		},
		{
			name:           "nonexistent run returns error",
			runs:           map[string]*run.Run{},
			runID:          "ghost",
			reason:         "test",
			wantErr:        true,
			wantErrContain: "get run",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			store := newLifecycleTestStore()
			store.runs = tt.runs

			q := &internalMockQueue{}
			bc := &internalMockBroadcaster{}
			es := &mockEventStore{}

			svc := &RuntimeService{
				store:      store,
				queue:      q,
				hub:        bc,
				events:     es,
				runtimeCfg: &config.Runtime{},
				state:      NewRunStateManager(),
			}

			err := svc.cancelRunWithReason(context.Background(), tt.runID, tt.reason)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.wantErrContain != "" && !strings.Contains(err.Error(), tt.wantErrContain) {
					t.Errorf("expected error containing %q, got %q", tt.wantErrContain, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantSkipped {
				if _, ok := store.completedRuns[tt.runID]; ok {
					t.Error("expected run to be skipped, but it was completed")
				}
				return
			}

			got, ok := store.completedRuns[tt.runID]
			if !ok {
				t.Fatal("expected run to be completed, but it was not")
			}
			if got != tt.wantStatus {
				t.Errorf("expected status %q, got %q", tt.wantStatus, got)
			}

			if tt.wantNATSCancel {
				q.mu.Lock()
				found := false
				for _, msg := range q.messages {
					if msg.subject == messagequeue.SubjectRunCancel {
						found = true
						break
					}
				}
				q.mu.Unlock()
				if !found {
					t.Error("expected cancel message on NATS")
				}
			}

			if tt.wantBroadcast {
				bc.mu.Lock()
				count := len(bc.events)
				bc.mu.Unlock()
				if count == 0 {
					t.Error("expected at least one WS broadcast event")
				}
			}
		})
	}
}

func TestCancelRunWithReason_CallsOnRunComplete(t *testing.T) {
	t.Parallel()

	store := newLifecycleTestStore()
	store.runs = map[string]*run.Run{
		"run-cb": {
			ID: "run-cb", TaskID: "t1", AgentID: "a1", ProjectID: "p1",
			Status: run.StatusRunning, StartedAt: time.Now(),
		},
	}

	q := &internalMockQueue{}
	bc := &internalMockBroadcaster{}
	es := &mockEventStore{}

	var callbackRunID string
	var callbackStatus run.Status

	svc := &RuntimeService{
		store:      store,
		queue:      q,
		hub:        bc,
		events:     es,
		runtimeCfg: &config.Runtime{},
		state:      NewRunStateManager(),
		onRunComplete: func(_ context.Context, runID string, status run.Status) {
			callbackRunID = runID
			callbackStatus = status
		},
	}

	if err := svc.cancelRunWithReason(context.Background(), "run-cb", "test"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if callbackRunID != "run-cb" {
		t.Errorf("expected onRunComplete called with run-cb, got %q", callbackRunID)
	}
	if callbackStatus != run.StatusTimeout {
		t.Errorf("expected onRunComplete status timeout, got %q", callbackStatus)
	}
}

// --- finalizeRun ---

// lifecycleTestStoreEx extends lifecycleTestStore with agent/task tracking
// for the more complex finalizeRun tests.
type lifecycleTestStoreEx struct {
	lifecycleTestStore
	agents     map[string]*agent.Agent
	tasks      map[string]*task.Task
	projects   map[string]string // id -> workspace_path
	agentStats map[string]struct {
		cost    float64
		success bool
	}
	taskResults   map[string]task.Result
	taskStatuses  map[string]task.Status
	agentStatuses map[string]agent.Status
}

func newLifecycleTestStoreEx() *lifecycleTestStoreEx {
	return &lifecycleTestStoreEx{
		lifecycleTestStore: *newLifecycleTestStore(),
		agents:             make(map[string]*agent.Agent),
		tasks:              make(map[string]*task.Task),
		projects:           make(map[string]string),
		agentStats: make(map[string]struct {
			cost    float64
			success bool
		}),
		taskResults:   make(map[string]task.Result),
		taskStatuses:  make(map[string]task.Status),
		agentStatuses: make(map[string]agent.Status),
	}
}

func (s *lifecycleTestStoreEx) UpdateAgentStatus(_ context.Context, id string, status agent.Status) error {
	s.agentStatuses[id] = status
	return nil
}

func (s *lifecycleTestStoreEx) UpdateTaskStatus(_ context.Context, id string, status task.Status) error {
	s.taskStatuses[id] = status
	return nil
}

func (s *lifecycleTestStoreEx) UpdateTaskResult(_ context.Context, id string, result task.Result, _ float64) error {
	s.taskResults[id] = result
	return nil
}

func (s *lifecycleTestStoreEx) IncrementAgentStats(_ context.Context, id string, costDelta float64, success bool) error {
	s.agentStats[id] = struct {
		cost    float64
		success bool
	}{costDelta, success}
	return nil
}

func TestFinalizeRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		run               *run.Run
		status            run.Status
		payload           *messagequeue.RunCompletePayload
		wantRunStatus     run.Status
		wantTaskStatus    task.Status
		wantAgentStatus   agent.Status
		wantAgentSuccess  bool
		wantBroadcastMin  int  // minimum number of broadcast events
		wantCallbackFired bool // onRunComplete called
	}{
		{
			name: "successful completion",
			run: &run.Run{
				ID: "run-ok", TaskID: "t1", AgentID: "a1", ProjectID: "p1",
				Status: run.StatusRunning, StartedAt: time.Now(),
			},
			status: run.StatusCompleted,
			payload: &messagequeue.RunCompletePayload{
				RunID:     "run-ok",
				Status:    "completed",
				Output:    "done",
				StepCount: 5,
				CostUSD:   0.01,
				TokensIn:  100,
				TokensOut: 50,
				Model:     "gpt-4",
			},
			wantRunStatus:     run.StatusCompleted,
			wantTaskStatus:    task.StatusCompleted,
			wantAgentStatus:   agent.StatusIdle,
			wantAgentSuccess:  true,
			wantBroadcastMin:  3, // run.status + agent.status + agui.run_finished
			wantCallbackFired: true,
		},
		{
			name: "failed run",
			run: &run.Run{
				ID: "run-fail", TaskID: "t2", AgentID: "a2", ProjectID: "p2",
				Status: run.StatusRunning, StartedAt: time.Now(),
			},
			status: run.StatusFailed,
			payload: &messagequeue.RunCompletePayload{
				RunID:     "run-fail",
				Status:    "failed",
				Error:     "something went wrong",
				StepCount: 3,
				CostUSD:   0.005,
				TokensIn:  80,
				TokensOut: 30,
				Model:     "gpt-4",
			},
			wantRunStatus:     run.StatusFailed,
			wantTaskStatus:    task.StatusFailed,
			wantAgentStatus:   agent.StatusIdle,
			wantAgentSuccess:  false,
			wantBroadcastMin:  3,
			wantCallbackFired: true,
		},
		{
			name: "timeout run",
			run: &run.Run{
				ID: "run-to", TaskID: "t3", AgentID: "a3", ProjectID: "p3",
				Status: run.StatusRunning, StartedAt: time.Now(),
			},
			status: run.StatusTimeout,
			payload: &messagequeue.RunCompletePayload{
				RunID:     "run-to",
				Status:    "timeout",
				Error:     "exceeded time limit",
				StepCount: 50,
				CostUSD:   1.5,
				TokensIn:  5000,
				TokensOut: 2000,
				Model:     "claude-3",
			},
			wantRunStatus:     run.StatusTimeout,
			wantTaskStatus:    task.StatusFailed,
			wantAgentStatus:   agent.StatusIdle,
			wantAgentSuccess:  false,
			wantBroadcastMin:  3,
			wantCallbackFired: true,
		},
		{
			name: "cancelled run",
			run: &run.Run{
				ID: "run-cx", TaskID: "t4", AgentID: "a4", ProjectID: "p4",
				Status: run.StatusRunning, StartedAt: time.Now(),
			},
			status: run.StatusCancelled,
			payload: &messagequeue.RunCompletePayload{
				RunID:     "run-cx",
				Status:    "cancelled",
				StepCount: 1,
			},
			wantRunStatus:     run.StatusCancelled,
			wantTaskStatus:    task.StatusCompleted, // cancelled is not failed/timeout
			wantAgentStatus:   agent.StatusIdle,
			wantAgentSuccess:  false,
			wantBroadcastMin:  3,
			wantCallbackFired: true,
		},
		{
			name: "zero cost and steps",
			run: &run.Run{
				ID: "run-zero", TaskID: "t5", AgentID: "a5", ProjectID: "p5",
				Status: run.StatusRunning, StartedAt: time.Now(),
			},
			status: run.StatusCompleted,
			payload: &messagequeue.RunCompletePayload{
				RunID:  "run-zero",
				Status: "completed",
				Output: "empty run",
			},
			wantRunStatus:     run.StatusCompleted,
			wantTaskStatus:    task.StatusCompleted,
			wantAgentStatus:   agent.StatusIdle,
			wantAgentSuccess:  true,
			wantBroadcastMin:  3,
			wantCallbackFired: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			store := newLifecycleTestStoreEx()
			store.runs[tt.run.ID] = tt.run

			bc := &internalMockBroadcaster{}
			es := &mockEventStore{}

			var callbackRunID string
			var callbackStatus run.Status
			callbackFired := false

			svc := &RuntimeService{
				store:      store,
				hub:        bc,
				events:     es,
				runtimeCfg: &config.Runtime{},
				state:      NewRunStateManager(),
				onRunComplete: func(_ context.Context, runID string, status run.Status) {
					callbackFired = true
					callbackRunID = runID
					callbackStatus = status
				},
			}

			err := svc.finalizeRun(context.Background(), tt.run, tt.status, tt.payload)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Verify run completed with correct status.
			if got, ok := store.completedRuns[tt.run.ID]; !ok {
				t.Error("expected run to be completed")
			} else if got != tt.wantRunStatus {
				t.Errorf("expected run status %q, got %q", tt.wantRunStatus, got)
			}

			// Verify task status.
			if got, ok := store.taskStatuses[tt.run.TaskID]; !ok {
				t.Error("expected task status to be updated")
			} else if got != tt.wantTaskStatus {
				t.Errorf("expected task status %q, got %q", tt.wantTaskStatus, got)
			}

			// Verify agent set to idle.
			if got, ok := store.agentStatuses[tt.run.AgentID]; !ok {
				t.Error("expected agent status to be updated")
			} else if got != tt.wantAgentStatus {
				t.Errorf("expected agent status %q, got %q", tt.wantAgentStatus, got)
			}

			// Verify agent stats incremented.
			if stats, ok := store.agentStats[tt.run.AgentID]; !ok {
				t.Error("expected agent stats to be incremented")
			} else if stats.success != tt.wantAgentSuccess {
				t.Errorf("expected agent stats success=%v, got %v", tt.wantAgentSuccess, stats.success)
			}

			// Verify WS broadcasts (run.status + agent.status + agui.run_finished).
			bc.mu.Lock()
			broadcastCount := len(bc.events)
			bc.mu.Unlock()
			if broadcastCount < tt.wantBroadcastMin {
				t.Errorf("expected at least %d broadcasts, got %d", tt.wantBroadcastMin, broadcastCount)
			}

			// Verify broadcast event types.
			bc.mu.Lock()
			var hasRunStatus, hasAgentStatus, hasAGUI bool
			for _, ev := range bc.events {
				switch ev.eventType {
				case event.EventRunStatus:
					hasRunStatus = true
				case event.EventAgentStatus:
					hasAgentStatus = true
				case event.AGUIRunFinished:
					hasAGUI = true
				}
			}
			bc.mu.Unlock()
			if !hasRunStatus {
				t.Error("expected run.status broadcast event")
			}
			if !hasAgentStatus {
				t.Error("expected agent.status broadcast event")
			}
			if !hasAGUI {
				t.Error("expected agui.run_finished broadcast event")
			}

			// Verify onRunComplete callback.
			if tt.wantCallbackFired && !callbackFired {
				t.Error("expected onRunComplete callback to fire")
			}
			if callbackFired {
				if callbackRunID != tt.run.ID {
					t.Errorf("callback run ID: expected %q, got %q", tt.run.ID, callbackRunID)
				}
				if callbackStatus != tt.status {
					t.Errorf("callback status: expected %q, got %q", tt.status, callbackStatus)
				}
			}

			// Verify event store received events.
			if len(es.events) == 0 {
				t.Error("expected at least one event appended")
			}
		})
	}
}

func TestFinalizeRun_NoCallback(t *testing.T) {
	t.Parallel()

	store := newLifecycleTestStoreEx()
	r := &run.Run{
		ID: "run-nocb", TaskID: "t1", AgentID: "a1", ProjectID: "p1",
		Status: run.StatusRunning, StartedAt: time.Now(),
	}
	store.runs[r.ID] = r

	svc := &RuntimeService{
		store:      store,
		hub:        &internalMockBroadcaster{},
		events:     &mockEventStore{},
		runtimeCfg: &config.Runtime{},
		state:      NewRunStateManager(),
		// onRunComplete intentionally nil
	}

	payload := &messagequeue.RunCompletePayload{
		RunID:  r.ID,
		Status: "completed",
		Output: "done",
	}

	// Should not panic when onRunComplete is nil.
	err := svc.finalizeRun(context.Background(), r, run.StatusCompleted, payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFinalizeRun_CleansUpRunState(t *testing.T) {
	t.Parallel()

	store := newLifecycleTestStoreEx()
	r := &run.Run{
		ID: "run-cleanup", TaskID: "t1", AgentID: "a1", ProjectID: "p1",
		Status: run.StatusRunning, StartedAt: time.Now(),
	}
	store.runs[r.ID] = r

	svc := &RuntimeService{
		store:      store,
		hub:        &internalMockBroadcaster{},
		events:     &mockEventStore{},
		runtimeCfg: &config.Runtime{},
		state:      NewRunStateManager(),
	}

	// Pre-populate state that should be cleaned.
	svc.state.SetHeartbeat(r.ID, time.Now())
	svc.state.StoreBudgetAlert(r.ID + ":80")
	svc.state.StoreBudgetAlert(r.ID + ":90")
	ch := make(chan string, 1)
	svc.state.SetPendingApproval(r.ID+":call-x", ch)

	payload := &messagequeue.RunCompletePayload{
		RunID:  r.ID,
		Status: "completed",
	}

	if err := svc.finalizeRun(context.Background(), r, run.StatusCompleted, payload); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify cleanup happened.
	if _, ok := svc.state.GetHeartbeat(r.ID); ok {
		t.Error("heartbeat not cleaned up after finalize")
	}
	if alreadySent := svc.state.StoreBudgetAlert(r.ID + ":80"); alreadySent {
		t.Error("budget alert 80% not cleaned up after finalize")
	}
	svc.state.DeleteBudgetAlert(r.ID + ":80")
	if alreadySent := svc.state.StoreBudgetAlert(r.ID + ":90"); alreadySent {
		t.Error("budget alert 90% not cleaned up after finalize")
	}
	svc.state.DeleteBudgetAlert(r.ID + ":90")
	if _, ok := svc.state.LoadAndDeletePendingApproval(r.ID + ":call-x"); ok {
		t.Error("pending approval not cleaned up after finalize")
	}
}

func TestFinalizeRun_AGUIStatusMapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status   run.Status
		wantAGUI string
	}{
		{run.StatusCompleted, "completed"},
		{run.StatusFailed, "failed"},
		{run.StatusTimeout, "failed"},
		{run.StatusCancelled, "cancelled"},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			t.Parallel()

			store := newLifecycleTestStoreEx()
			r := &run.Run{
				ID: "run-agui-" + string(tt.status), TaskID: "t1", AgentID: "a1", ProjectID: "p1",
				Status: run.StatusRunning, StartedAt: time.Now(),
			}
			store.runs[r.ID] = r

			bc := &internalMockBroadcaster{}
			svc := &RuntimeService{
				store:      store,
				hub:        bc,
				events:     &mockEventStore{},
				runtimeCfg: &config.Runtime{},
				state:      NewRunStateManager(),
			}

			payload := &messagequeue.RunCompletePayload{RunID: r.ID, Status: string(tt.status)}
			if err := svc.finalizeRun(context.Background(), r, tt.status, payload); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			bc.mu.Lock()
			defer bc.mu.Unlock()

			for _, ev := range bc.events {
				if ev.eventType != event.AGUIRunFinished {
					continue
				}
				aguiEv, ok := ev.data.(event.AGUIRunFinishedEvent)
				if !ok {
					t.Fatalf("expected AGUIRunFinishedEvent, got %T", ev.data)
				}
				if aguiEv.Status != tt.wantAGUI {
					t.Errorf("AGUI status: expected %q, got %q", tt.wantAGUI, aguiEv.Status)
				}
				return
			}
			t.Error("AGUIRunFinished event not found in broadcasts")
		})
	}
}
