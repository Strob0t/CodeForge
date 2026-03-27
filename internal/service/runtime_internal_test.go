package service

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/config"
	cfcontext "github.com/Strob0t/CodeForge/internal/domain/context"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/policy"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/port/eventstore"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// ============================================================================
// Tests for unexported functions in runtime_lifecycle.go
// ============================================================================

func TestIsFileModifyingTool(t *testing.T) {
	tests := []struct {
		tool     string
		expected bool
	}{
		{"Edit", true},
		{"Write", true},
		{"Bash", true},
		{"execute", true},
		{"write_file", true},
		{"edit_file", true},
		{"Read", false},
		{"Search", false},
		{"Glob", false},
		{"ListDir", false},
		{"", false},
		{"edit", false}, // case-sensitive
	}

	for _, tc := range tests {
		t.Run(tc.tool, func(t *testing.T) {
			got := isFileModifyingTool(tc.tool)
			if got != tc.expected {
				t.Errorf("isFileModifyingTool(%q) = %v, want %v", tc.tool, got, tc.expected)
			}
		})
	}
}

func TestCancelAll(t *testing.T) {
	var callOrder []int
	fns := []func(){
		func() { callOrder = append(callOrder, 1) },
		func() { callOrder = append(callOrder, 2) },
		func() { callOrder = append(callOrder, 3) },
	}

	cancelAll(fns)

	if len(callOrder) != 3 {
		t.Fatalf("expected 3 calls, got %d", len(callOrder))
	}
	for i, v := range callOrder {
		if v != i+1 {
			t.Errorf("expected call order %d at index %d, got %d", i+1, i, v)
		}
	}
}

func TestCancelAll_Empty(t *testing.T) {
	// Should not panic on empty slice
	cancelAll(nil)
	cancelAll([]func(){})
}

func TestToContextEntryPayloads(t *testing.T) {
	entries := []cfcontext.ContextEntry{
		{
			Kind:     cfcontext.EntryFile,
			Path:     "main.go",
			Content:  "package main",
			Tokens:   10,
			Priority: 80,
		},
		{
			Kind:     cfcontext.EntrySnippet,
			Path:     "util.go",
			Content:  "func helper() {}",
			Tokens:   5,
			Priority: 50,
		},
	}

	payloads := toContextEntryPayloads(entries)

	if len(payloads) != 2 {
		t.Fatalf("expected 2 payloads, got %d", len(payloads))
	}

	// Check first entry
	if payloads[0].Kind != "file" {
		t.Errorf("expected kind 'file', got %q", payloads[0].Kind)
	}
	if payloads[0].Path != "main.go" {
		t.Errorf("expected path 'main.go', got %q", payloads[0].Path)
	}
	if payloads[0].Content != "package main" {
		t.Errorf("expected content 'package main', got %q", payloads[0].Content)
	}
	if payloads[0].Tokens != 10 {
		t.Errorf("expected tokens 10, got %d", payloads[0].Tokens)
	}
	if payloads[0].Priority != 80 {
		t.Errorf("expected priority 80, got %d", payloads[0].Priority)
	}

	// Check second entry
	if payloads[1].Kind != "snippet" {
		t.Errorf("expected kind 'snippet', got %q", payloads[1].Kind)
	}
	if payloads[1].Path != "util.go" {
		t.Errorf("expected path 'util.go', got %q", payloads[1].Path)
	}
}

func TestToContextEntryPayloads_Empty(t *testing.T) {
	payloads := toContextEntryPayloads(nil)
	if len(payloads) != 0 {
		t.Fatalf("expected 0 payloads for nil input, got %d", len(payloads))
	}

	payloads = toContextEntryPayloads([]cfcontext.ContextEntry{})
	if len(payloads) != 0 {
		t.Fatalf("expected 0 payloads for empty input, got %d", len(payloads))
	}
}

func TestCheckTermination_NoLimits(t *testing.T) {
	svc := &RuntimeService{
		runtimeCfg: &config.Runtime{},
	}

	profile := &policy.PolicyProfile{
		Termination: policy.TerminationCondition{},
	}

	r := &run.Run{
		StepCount: 999,
		CostUSD:   999,
		StartedAt: time.Now().Add(-10 * time.Minute), // within absolute timeout
	}

	reason := svc.checkTermination(r, profile)
	if reason != "" {
		t.Fatalf("expected no termination with empty limits, got %q", reason)
	}
}

func TestCheckTermination_AbsoluteTimeout(t *testing.T) {
	svc := &RuntimeService{
		runtimeCfg: &config.Runtime{},
	}

	profile := &policy.PolicyProfile{
		Termination: policy.TerminationCondition{},
	}

	// Run started well over 1 hour ago should be terminated
	r := &run.Run{
		StartedAt: time.Now().Add(-2 * time.Hour),
	}

	reason := svc.checkTermination(r, profile)
	if reason == "" {
		t.Fatal("expected absolute timeout termination, got empty")
	}
	if !strings.Contains(reason, "absolute execution timeout") {
		t.Fatalf("expected absolute timeout message, got %q", reason)
	}
}

func TestCheckTermination_MaxSteps(t *testing.T) {
	svc := &RuntimeService{
		runtimeCfg: &config.Runtime{},
	}

	profile := &policy.PolicyProfile{
		Termination: policy.TerminationCondition{
			MaxSteps: 10,
		},
	}

	tests := []struct {
		name      string
		stepCount int
		shouldEnd bool
	}{
		{"under_limit", 5, false},
		{"at_limit", 10, true},
		{"over_limit", 15, true},
		{"zero", 0, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := &run.Run{StepCount: tc.stepCount, StartedAt: time.Now()}
			reason := svc.checkTermination(r, profile)
			if tc.shouldEnd && reason == "" {
				t.Error("expected termination reason, got empty")
			}
			if !tc.shouldEnd && reason != "" {
				t.Errorf("expected no termination, got %q", reason)
			}
		})
	}
}

func TestCheckTermination_MaxCost(t *testing.T) {
	svc := &RuntimeService{
		runtimeCfg: &config.Runtime{},
	}

	profile := &policy.PolicyProfile{
		Termination: policy.TerminationCondition{
			MaxCost: 5.0,
		},
	}

	tests := []struct {
		name      string
		costUSD   float64
		shouldEnd bool
	}{
		{"under_limit", 3.0, false},
		{"at_limit", 5.0, true},
		{"over_limit", 5.01, true},
		{"zero", 0, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := &run.Run{CostUSD: tc.costUSD, StartedAt: time.Now()}
			reason := svc.checkTermination(r, profile)
			if tc.shouldEnd && reason == "" {
				t.Error("expected termination reason, got empty")
			}
			if !tc.shouldEnd && reason != "" {
				t.Errorf("expected no termination, got %q", reason)
			}
		})
	}
}

func TestCheckTermination_Timeout(t *testing.T) {
	svc := &RuntimeService{
		runtimeCfg: &config.Runtime{},
	}

	profile := &policy.PolicyProfile{
		Termination: policy.TerminationCondition{
			TimeoutSeconds: 60,
		},
	}

	// Run started 2 minutes ago should be timed out
	r := &run.Run{StartedAt: time.Now().Add(-2 * time.Minute)}
	reason := svc.checkTermination(r, profile)
	if reason == "" {
		t.Error("expected timeout termination for old run")
	}

	// Run started 10 seconds ago should not be timed out
	r2 := &run.Run{StartedAt: time.Now().Add(-10 * time.Second)}
	reason2 := svc.checkTermination(r2, profile)
	if reason2 != "" {
		t.Errorf("expected no timeout for recent run, got %q", reason2)
	}
}

func TestCheckTermination_HeartbeatTimeout(t *testing.T) {
	svc := &RuntimeService{
		runtimeCfg: &config.Runtime{
			HeartbeatTimeout: 30 * time.Second,
		},
	}

	profile := &policy.PolicyProfile{}
	r := &run.Run{ID: "run-hb", StartedAt: time.Now()}

	// No heartbeat recorded — should not trigger
	reason := svc.checkTermination(r, profile)
	if reason != "" {
		t.Errorf("expected no termination without heartbeat, got %q", reason)
	}

	// Old heartbeat — should trigger
	svc.heartbeats.Store("run-hb", time.Now().Add(-2*time.Minute))
	reason = svc.checkTermination(r, profile)
	if reason == "" {
		t.Error("expected heartbeat timeout termination for old heartbeat")
	}

	// Recent heartbeat — should not trigger
	svc.heartbeats.Store("run-hb", time.Now())
	reason = svc.checkTermination(r, profile)
	if reason != "" {
		t.Errorf("expected no heartbeat timeout for recent heartbeat, got %q", reason)
	}
}

func TestCleanupRunState_AllFields(t *testing.T) {
	svc := &RuntimeService{
		runtimeCfg: &config.Runtime{},
	}

	runID := "run-cleanup-all"

	// Populate all fields that cleanupRunState should clean
	svc.heartbeats.Store(runID, time.Now())
	svc.stallTrackers.Store(runID, run.NewStallTracker(5, 2))

	cancelCalled := false
	ctx, cancel := context.WithCancel(context.Background())
	_ = ctx
	svc.runTimeouts.Store(runID, context.CancelFunc(func() {
		cancelCalled = true
		cancel()
	}))
	svc.budgetAlerts.Store(runID+":80", true)
	svc.budgetAlerts.Store(runID+":90", true)

	// Add a pending approval channel
	ch := make(chan string, 1)
	svc.pendingApprovals.Store(runID+":call-1", ch)

	svc.cleanupRunState(runID)

	// Verify all cleaned
	if _, ok := svc.heartbeats.Load(runID); ok {
		t.Error("heartbeat not cleaned")
	}
	if _, ok := svc.stallTrackers.Load(runID); ok {
		t.Error("stall tracker not cleaned")
	}
	if _, ok := svc.runTimeouts.Load(runID); ok {
		t.Error("run timeout not cleaned")
	}
	if !cancelCalled {
		t.Error("timeout cancel function not called")
	}
	if _, ok := svc.budgetAlerts.Load(runID + ":80"); ok {
		t.Error("budget alert 80 not cleaned")
	}
	if _, ok := svc.budgetAlerts.Load(runID + ":90"); ok {
		t.Error("budget alert 90 not cleaned")
	}
	if _, ok := svc.pendingApprovals.Load(runID + ":call-1"); ok {
		t.Error("pending approval not cleaned")
	}

	// The channel should have received "deny"
	select {
	case msg := <-ch:
		if msg != "deny" {
			t.Errorf("expected 'deny' on approval channel, got %q", msg)
		}
	default:
		t.Error("expected 'deny' message on approval channel")
	}
}

func TestCleanupRunState_OtherRunsUnaffected(t *testing.T) {
	svc := &RuntimeService{
		runtimeCfg: &config.Runtime{},
	}

	// Set up state for two runs
	svc.heartbeats.Store("run-a", time.Now())
	svc.heartbeats.Store("run-b", time.Now())
	svc.budgetAlerts.Store("run-a:80", true)
	svc.budgetAlerts.Store("run-b:80", true)
	ch := make(chan string, 1)
	svc.pendingApprovals.Store("run-a:call-1", ch)
	chB := make(chan string, 1)
	svc.pendingApprovals.Store("run-b:call-1", chB)

	// Clean up only run-a
	svc.cleanupRunState("run-a")

	// run-b should be unaffected
	if _, ok := svc.heartbeats.Load("run-b"); !ok {
		t.Error("run-b heartbeat should not be cleaned")
	}
	if _, ok := svc.budgetAlerts.Load("run-b:80"); !ok {
		t.Error("run-b budget alert should not be cleaned")
	}
	if _, ok := svc.pendingApprovals.Load("run-b:call-1"); !ok {
		t.Error("run-b pending approval should not be cleaned")
	}
}

// --- Minimal mock types for internal tests ---

type internalMockBroadcaster struct {
	mu     sync.Mutex
	events []struct {
		eventType string
		data      any
	}
}

func (m *internalMockBroadcaster) BroadcastEvent(_ context.Context, eventType string, data any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, struct {
		eventType string
		data      any
	}{eventType, data})
}

type internalMockQueue struct {
	mu       sync.Mutex
	messages []struct {
		subject string
		data    []byte
	}
}

func (m *internalMockQueue) Publish(_ context.Context, subject string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, struct {
		subject string
		data    []byte
	}{subject, data})
	return nil
}
func (m *internalMockQueue) PublishWithDedup(ctx context.Context, subject string, data []byte, _ string) error {
	return m.Publish(ctx, subject, data)
}
func (m *internalMockQueue) Subscribe(_ context.Context, _ string, _ messagequeue.Handler) (func(), error) {
	return func() {}, nil
}
func (m *internalMockQueue) Drain() error      { return nil }
func (m *internalMockQueue) Close() error      { return nil }
func (m *internalMockQueue) IsConnected() bool { return true }

// TestSendToolCallResponse_BasicPath verifies the basic tool call response path.
func TestSendToolCallResponse_BasicPath(t *testing.T) {
	queue := &internalMockQueue{}
	svc := &RuntimeService{
		queue:      queue,
		runtimeCfg: &config.Runtime{},
	}

	err := svc.sendToolCallResponse(context.Background(), "run-1", "call-1", "allow", "")
	if err != nil {
		t.Fatalf("sendToolCallResponse: %v", err)
	}

	queue.mu.Lock()
	defer queue.mu.Unlock()
	if len(queue.messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(queue.messages))
	}
	if queue.messages[0].subject != messagequeue.SubjectRunToolCallResponse {
		t.Errorf("expected subject %q, got %q", messagequeue.SubjectRunToolCallResponse, queue.messages[0].subject)
	}
}

// TestApprovalKey verifies the approval key format.
func TestApprovalKey(t *testing.T) {
	key := approvalKey("run-123", "call-456")
	if key != "run-123:call-456" {
		t.Errorf("expected 'run-123:call-456', got %q", key)
	}
}

// TestWaitForApproval_DefaultTimeout verifies default 60s timeout when config is 0.
func TestWaitForApproval_DefaultTimeout(t *testing.T) {
	t.Parallel()

	bc := &internalMockBroadcaster{}
	svc := &RuntimeService{
		hub:        bc,
		runtimeCfg: &config.Runtime{ApprovalTimeoutSeconds: 0}, // 0 = use default 60s
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// Context cancel should fire before the default 60s timeout
	decision := svc.waitForApproval(ctx, "run-def", "call-def", "Read", "", "")
	if decision != policy.DecisionDeny {
		t.Errorf("expected deny on context cancel, got %s", decision)
	}
}

// TestWaitForApproval_ResolveBeforeTimeout verifies that ResolveApproval
// properly unblocks waitForApproval.
func TestWaitForApproval_ResolveBeforeTimeout(t *testing.T) {
	t.Parallel()

	bc := &internalMockBroadcaster{}
	svc := &RuntimeService{
		hub:        bc,
		runtimeCfg: &config.Runtime{ApprovalTimeoutSeconds: 30},
	}

	resultCh := make(chan policy.Decision, 1)
	go func() {
		resultCh <- svc.waitForApproval(context.Background(), "run-resolve", "call-resolve", "Bash", "test", "")
	}()

	// Give goroutine time to register the channel
	time.Sleep(50 * time.Millisecond)

	ok := svc.ResolveApproval("run-resolve", "call-resolve", "allow")
	if !ok {
		t.Fatal("ResolveApproval returned false")
	}

	select {
	case d := <-resultCh:
		if d != policy.DecisionAllow {
			t.Errorf("expected allow, got %s", d)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("waitForApproval did not unblock")
	}
}

// --- Handler-capturing mock queue for trajectory tests ---

type handlerCapturingQueue struct {
	internalMockQueue
	handlerMu sync.Mutex
	handlers  map[string]messagequeue.Handler
}

func newHandlerCapturingQueue() *handlerCapturingQueue {
	return &handlerCapturingQueue{
		handlers: make(map[string]messagequeue.Handler),
	}
}

func (q *handlerCapturingQueue) Subscribe(_ context.Context, subject string, handler messagequeue.Handler) (func(), error) {
	q.handlerMu.Lock()
	defer q.handlerMu.Unlock()
	q.handlers[subject] = handler
	return func() {}, nil
}

func (q *handlerCapturingQueue) getHandler(subject string) (messagequeue.Handler, bool) {
	q.handlerMu.Lock()
	defer q.handlerMu.Unlock()
	h, ok := q.handlers[subject]
	return h, ok
}

// --- Minimal event store mock for trajectory tests ---

type trajectoryMockEventStore struct{}

func (m *trajectoryMockEventStore) Append(_ context.Context, _ *event.AgentEvent) error {
	return nil
}
func (m *trajectoryMockEventStore) LoadByTask(_ context.Context, _ string) ([]event.AgentEvent, error) {
	return nil, nil
}
func (m *trajectoryMockEventStore) LoadByAgent(_ context.Context, _ string) ([]event.AgentEvent, error) {
	return nil, nil
}
func (m *trajectoryMockEventStore) LoadByRun(_ context.Context, _ string) ([]event.AgentEvent, error) {
	return nil, nil
}
func (m *trajectoryMockEventStore) LoadTrajectory(_ context.Context, _ string, _ eventstore.TrajectoryFilter, _ string, _ int) (*eventstore.TrajectoryPage, error) {
	return &eventstore.TrajectoryPage{}, nil
}
func (m *trajectoryMockEventStore) TrajectoryStats(_ context.Context, _ string) (*eventstore.TrajectorySummary, error) {
	return &eventstore.TrajectorySummary{}, nil
}
func (m *trajectoryMockEventStore) LoadEventsRange(_ context.Context, _, _, _ string) ([]event.AgentEvent, error) {
	return nil, nil
}
func (m *trajectoryMockEventStore) ListCheckpoints(_ context.Context, _ string) ([]event.AgentEvent, error) {
	return nil, nil
}
func (m *trajectoryMockEventStore) AppendAudit(_ context.Context, _ *event.AuditEntry) error {
	return nil
}
func (m *trajectoryMockEventStore) LoadAudit(_ context.Context, _ *event.AuditFilter, _ string, _ int) (*event.AuditPage, error) {
	return nil, nil
}

// TestTrajectoryEvent_RoadmapProposed_BroadcastsAGUI verifies that an
// agent.roadmap_proposed trajectory event is broadcast as an agui.roadmap_proposal
// WebSocket event with all fields correctly populated.
func TestTrajectoryEvent_RoadmapProposed_BroadcastsAGUI(t *testing.T) {
	t.Parallel()

	bc := &internalMockBroadcaster{}
	queue := newHandlerCapturingQueue()
	es := &trajectoryMockEventStore{}

	svc := &RuntimeService{
		hub:        bc,
		queue:      queue,
		events:     es,
		runtimeCfg: &config.Runtime{},
	}

	cancels, err := svc.StartSubscribers(context.Background())
	if err != nil {
		t.Fatalf("StartSubscribers: %v", err)
	}
	defer func() {
		for _, c := range cancels {
			c()
		}
	}()

	handler, ok := queue.getHandler(messagequeue.SubjectTrajectoryEvent)
	if !ok {
		t.Fatal("no handler registered for trajectory event subject")
	}

	// Build a roadmap_proposed trajectory event payload.
	payload := map[string]any{
		"event_type": "agent.roadmap_proposed",
		"run_id":     "run-roadmap-1",
		"project_id": "proj-1",
		"data": map[string]any{
			"proposal_id":           "prop-rm-1",
			"action":                "create_milestone",
			"milestone_title":       "Authentication",
			"milestone_description": "Implement OAuth2 login",
			"milestone_sort_order":  1,
			"step_title":            "Add JWT middleware",
			"step_description":      "Validate tokens on protected routes",
			"step_sort_order":       2,
			"step_complexity":       "medium",
			"step_model_tier":       "strong",
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	if err := handler(context.Background(), messagequeue.SubjectTrajectoryEvent, data); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	// Verify broadcasts: expect the generic trajectory event + the AGUI roadmap proposal.
	bc.mu.Lock()
	defer bc.mu.Unlock()

	var found bool
	for _, ev := range bc.events {
		if ev.eventType != event.AGUIRoadmapProposal {
			continue
		}
		found = true
		rp, ok := ev.data.(event.AGUIRoadmapProposalEvent)
		if !ok {
			t.Fatalf("expected AGUIRoadmapProposalEvent, got %T", ev.data)
		}
		if rp.RunID != "run-roadmap-1" {
			t.Errorf("RunID = %q, want %q", rp.RunID, "run-roadmap-1")
		}
		if rp.ProposalID != "prop-rm-1" {
			t.Errorf("ProposalID = %q, want %q", rp.ProposalID, "prop-rm-1")
		}
		if rp.Action != "create_milestone" {
			t.Errorf("Action = %q, want %q", rp.Action, "create_milestone")
		}
		if rp.MilestoneTitle != "Authentication" {
			t.Errorf("MilestoneTitle = %q, want %q", rp.MilestoneTitle, "Authentication")
		}
		if rp.MilestoneDescription != "Implement OAuth2 login" {
			t.Errorf("MilestoneDescription = %q, want %q", rp.MilestoneDescription, "Implement OAuth2 login")
		}
		if rp.MilestoneSortOrder != 1 {
			t.Errorf("MilestoneSortOrder = %d, want %d", rp.MilestoneSortOrder, 1)
		}
		if rp.StepTitle != "Add JWT middleware" {
			t.Errorf("StepTitle = %q, want %q", rp.StepTitle, "Add JWT middleware")
		}
		if rp.StepDescription != "Validate tokens on protected routes" {
			t.Errorf("StepDescription = %q, want %q", rp.StepDescription, "Validate tokens on protected routes")
		}
		if rp.StepSortOrder != 2 {
			t.Errorf("StepSortOrder = %d, want %d", rp.StepSortOrder, 2)
		}
		if rp.StepComplexity != "medium" {
			t.Errorf("StepComplexity = %q, want %q", rp.StepComplexity, "medium")
		}
		if rp.StepModelTier != "strong" {
			t.Errorf("StepModelTier = %q, want %q", rp.StepModelTier, "strong")
		}
		break
	}
	if !found {
		t.Error("expected agui.roadmap_proposal broadcast event, but none found")
	}
}
