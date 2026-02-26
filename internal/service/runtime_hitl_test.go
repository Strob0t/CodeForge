package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/policy"
)

// hitlMockBroadcaster is a minimal Broadcaster mock for HITL tests.
type hitlMockBroadcaster struct {
	mu     sync.Mutex
	events []hitlBroadcastedEvent
}

type hitlBroadcastedEvent struct {
	EventType string
	Data      any
}

func (m *hitlMockBroadcaster) BroadcastEvent(_ context.Context, eventType string, data any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, hitlBroadcastedEvent{EventType: eventType, Data: data})
}

// newHITLTestService creates a minimal RuntimeService with only the fields
// required for the HITL approval flow: hub (Broadcaster) and runtimeCfg.
func newHITLTestService(approvalTimeoutSec int) (*RuntimeService, *hitlMockBroadcaster) {
	bc := &hitlMockBroadcaster{}
	cfg := &config.Runtime{
		ApprovalTimeoutSeconds: approvalTimeoutSec,
	}
	svc := &RuntimeService{
		hub:        bc,
		runtimeCfg: cfg,
	}
	return svc, bc
}

// TestHITL_ApproveUnblocksWait verifies that calling ResolveApproval with
// "allow" unblocks a waiting waitForApproval call and returns DecisionAllow.
func TestHITL_ApproveUnblocksWait(t *testing.T) {
	t.Parallel()

	svc, _ := newHITLTestService(30)
	ctx := context.Background()
	runID := "run-approve"
	callID := "call-1"

	resultCh := make(chan policy.Decision, 1)

	// Start waitForApproval in a goroutine; it blocks until resolved.
	go func() {
		resultCh <- svc.waitForApproval(ctx, runID, callID, "Bash", "rm -rf /tmp/test", "")
	}()

	// Give the goroutine a moment to register the channel.
	time.Sleep(50 * time.Millisecond)

	// Resolve with "allow".
	ok := svc.ResolveApproval(runID, callID, "allow")
	if !ok {
		t.Fatal("ResolveApproval returned false; expected pending approval to exist")
	}

	select {
	case decision := <-resultCh:
		if decision != policy.DecisionAllow {
			t.Errorf("expected DecisionAllow, got %q", decision)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("waitForApproval did not unblock within 5s after ResolveApproval")
	}
}

// TestHITL_DenyUnblocksWait verifies that calling ResolveApproval with "deny"
// unblocks waitForApproval and returns DecisionDeny.
func TestHITL_DenyUnblocksWait(t *testing.T) {
	t.Parallel()

	svc, _ := newHITLTestService(30)
	ctx := context.Background()
	runID := "run-deny"
	callID := "call-2"

	resultCh := make(chan policy.Decision, 1)

	go func() {
		resultCh <- svc.waitForApproval(ctx, runID, callID, "Edit", "", "/etc/passwd")
	}()

	time.Sleep(50 * time.Millisecond)

	ok := svc.ResolveApproval(runID, callID, "deny")
	if !ok {
		t.Fatal("ResolveApproval returned false; expected pending approval to exist")
	}

	select {
	case decision := <-resultCh:
		if decision != policy.DecisionDeny {
			t.Errorf("expected DecisionDeny, got %q", decision)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("waitForApproval did not unblock within 5s after ResolveApproval(deny)")
	}
}

// TestHITL_TimeoutReturnsDeny verifies that when no one calls ResolveApproval,
// waitForApproval times out and returns DecisionDeny.
func TestHITL_TimeoutReturnsDeny(t *testing.T) {
	t.Parallel()

	// Use a 1-second timeout so the test completes quickly.
	svc, _ := newHITLTestService(1)
	ctx := context.Background()
	runID := "run-timeout"
	callID := "call-3"

	start := time.Now()
	decision := svc.waitForApproval(ctx, runID, callID, "Bash", "danger", "")
	elapsed := time.Since(start)

	if decision != policy.DecisionDeny {
		t.Errorf("expected DecisionDeny on timeout, got %q", decision)
	}

	// Verify it actually waited (at least 900ms to account for timing jitter).
	if elapsed < 900*time.Millisecond {
		t.Errorf("expected timeout around 1s, but returned in %v", elapsed)
	}
	// Verify it didn't wait far too long.
	if elapsed > 3*time.Second {
		t.Errorf("expected timeout around 1s, but took %v", elapsed)
	}
}

// TestHITL_ContextCancelReturnsDeny verifies that cancelling the context
// unblocks waitForApproval and returns DecisionDeny.
func TestHITL_ContextCancelReturnsDeny(t *testing.T) {
	t.Parallel()

	svc, _ := newHITLTestService(30)
	ctx, cancel := context.WithCancel(context.Background())
	runID := "run-ctx-cancel"
	callID := "call-4"

	resultCh := make(chan policy.Decision, 1)

	go func() {
		resultCh <- svc.waitForApproval(ctx, runID, callID, "Bash", "ls", "")
	}()

	time.Sleep(50 * time.Millisecond)

	// Cancel the context; waitForApproval should return DecisionDeny.
	cancel()

	select {
	case decision := <-resultCh:
		if decision != policy.DecisionDeny {
			t.Errorf("expected DecisionDeny on context cancel, got %q", decision)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("waitForApproval did not unblock within 5s after context cancel")
	}
}

// TestHITL_ResolveNonExistentReturnsFalse verifies that calling ResolveApproval
// for a key that does not exist returns false.
func TestHITL_ResolveNonExistentReturnsFalse(t *testing.T) {
	t.Parallel()

	svc, _ := newHITLTestService(30)

	ok := svc.ResolveApproval("no-such-run", "no-such-call", "allow")
	if ok {
		t.Error("expected false for non-existent approval, got true")
	}
}

// TestHITL_BroadcastEventFired verifies that waitForApproval broadcasts
// an AGUIPermissionRequest event to connected clients.
func TestHITL_BroadcastEventFired(t *testing.T) {
	t.Parallel()

	svc, bc := newHITLTestService(1)
	ctx := context.Background()
	runID := "run-broadcast"
	callID := "call-5"

	// Let it time out; we just want to verify the broadcast happened.
	_ = svc.waitForApproval(ctx, runID, callID, "Bash", "echo hello", "/tmp")

	bc.mu.Lock()
	defer bc.mu.Unlock()

	if len(bc.events) == 0 {
		t.Fatal("expected at least one broadcast event, got none")
	}
	if bc.events[0].EventType != "agui.permission_request" {
		t.Errorf("expected event type %q, got %q", "agui.permission_request", bc.events[0].EventType)
	}
}

// TestHITL_PendingApprovalCleanedUpAfterResolve verifies that after
// waitForApproval returns, the entry is removed from pendingApprovals.
func TestHITL_PendingApprovalCleanedUpAfterResolve(t *testing.T) {
	t.Parallel()

	svc, _ := newHITLTestService(30)
	ctx := context.Background()
	runID := "run-cleanup"
	callID := "call-6"

	done := make(chan struct{})
	go func() {
		_ = svc.waitForApproval(ctx, runID, callID, "Read", "", "/etc/hosts")
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)

	svc.ResolveApproval(runID, callID, "allow")

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("waitForApproval did not return")
	}

	// The deferred Delete in waitForApproval should have cleaned up the map.
	key := approvalKey(runID, callID)
	if _, loaded := svc.pendingApprovals.Load(key); loaded {
		t.Error("expected pendingApprovals entry to be cleaned up, but it still exists")
	}
}

// TestHITL_DoubleResolveReturnsFalse verifies that resolving the same
// approval twice returns false the second time (the channel is consumed).
func TestHITL_DoubleResolveReturnsFalse(t *testing.T) {
	t.Parallel()

	svc, _ := newHITLTestService(30)
	ctx := context.Background()
	runID := "run-double"
	callID := "call-7"

	done := make(chan struct{})
	go func() {
		_ = svc.waitForApproval(ctx, runID, callID, "Bash", "test", "")
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)

	ok1 := svc.ResolveApproval(runID, callID, "allow")
	if !ok1 {
		t.Fatal("first ResolveApproval should return true")
	}

	<-done

	// Second resolve: the entry was already consumed by LoadAndDelete.
	ok2 := svc.ResolveApproval(runID, callID, "allow")
	if ok2 {
		t.Error("second ResolveApproval should return false (already consumed)")
	}
}
