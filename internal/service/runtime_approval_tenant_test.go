package service

import (
	"context"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/policy"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

const (
	approvalTenantA = "aaaaaaaa-0000-0000-0000-000000000001"
	approvalTenantB = "bbbbbbbb-0000-0000-0000-000000000002"
)

// waitPending starts a HITL wait for runID/callID in the run's tenant and
// waits until the approval is pending.
func waitPending(t *testing.T, svc *RuntimeService, tenant, runID, callID string) <-chan policy.Decision {
	t.Helper()
	decided := make(chan policy.Decision, 1)
	ctx := tenantctx.WithTenant(context.Background(), tenant)
	go func() {
		decided <- svc.waitForApproval(ctx, &event.AGUIPermissionRequestEvent{RunID: runID, CallID: callID, Tool: "Bash", Command: "make"})
	}()
	deadline := time.Now().Add(2 * time.Second)
	for !svc.hasPendingApproval(tenant, runID, callID) {
		if time.Now().After(deadline) {
			t.Fatal("approval never became pending")
		}
		time.Sleep(5 * time.Millisecond)
	}
	return decided
}

// TestResolveApproval_OnlyTheRunsTenant: knowing another tenant's run and
// call IDs must not be enough to approve or deny its tool call (KI-63).
func TestResolveApproval_OnlyTheRunsTenant(t *testing.T) {
	svc, _ := newHITLTestService(30)
	decided := waitPending(t, svc, approvalTenantA, "run-1", "call-1")

	otherTenant := tenantctx.WithTenant(context.Background(), approvalTenantB)
	if svc.ResolveApproval(otherTenant, "run-1", "call-1", "allow") {
		t.Fatal("another tenant resolved the approval")
	}
	if !svc.hasPendingApproval(approvalTenantA, "run-1", "call-1") {
		t.Fatal("another tenant's attempt consumed the pending approval")
	}

	owner := tenantctx.WithTenant(context.Background(), approvalTenantA)
	if !svc.ResolveApproval(owner, "run-1", "call-1", "deny") {
		t.Fatal("the run's tenant could not resolve its approval")
	}
	select {
	case d := <-decided:
		if d != policy.DecisionDeny {
			t.Fatalf("decision = %q, want deny", d)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("wait did not end")
	}
}

// TestResolveApproval_CallerWithoutTenantIsTheDefaultTenant: a context
// without tenant is the default tenant, which does not own another tenant's
// approval.
func TestResolveApproval_CallerWithoutTenantIsTheDefaultTenant(t *testing.T) {
	svc, _ := newHITLTestService(30)
	decided := waitPending(t, svc, approvalTenantA, "run-2", "call-2")

	if svc.ResolveApproval(context.Background(), "run-2", "call-2", "allow") {
		t.Fatal("a caller without tenant resolved another tenant's approval")
	}
	owner := tenantctx.WithTenant(context.Background(), approvalTenantA)
	if !svc.ResolveApproval(owner, "run-2", "call-2", "allow") {
		t.Fatal("owner could not resolve")
	}
	<-decided
}

// hasPendingApproval reports whether tenant's approval for runID/callID is pending.
func (s *RuntimeService) hasPendingApproval(tenant, runID, callID string) bool {
	want := approvalKey(tenant, runID, callID)
	found := false
	s.state.RangePendingApprovals(func(key string, _ chan string) bool {
		found = key == want
		return !found
	})
	return found
}
