package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// KI-57: the web UI's approval page (linked from approval emails) shows the
// tool call awaiting a decision - only to the tenant whose run asks, and
// only while it is pending.
func TestPendingApproval_VisibleToTheRunsTenantWhilePending(t *testing.T) {
	svc, _ := newHITLTestService(30)
	runCtx := tenantctx.WithTenant(context.Background(), "tenant-a")
	req := &event.AGUIPermissionRequestEvent{RunID: "run-1", CallID: "call-1", Tool: "Bash", Command: "make deploy", Profile: "supervised"}

	done := make(chan struct{})
	go func() {
		defer close(done)
		svc.waitForApproval(runCtx, req)
	}()

	deadline := time.Now().Add(5 * time.Second)
	for {
		got, err := svc.PendingApproval(tenantctx.WithTenant(context.Background(), "tenant-a"), "run-1", "call-1")
		if err == nil {
			if got.Tool != "Bash" || got.Command != "make deploy" || got.Profile != "supervised" {
				t.Fatalf("pending approval = %+v", got)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("PendingApproval: %v", err)
		}
		time.Sleep(5 * time.Millisecond)
	}

	if _, err := svc.PendingApproval(tenantctx.WithTenant(context.Background(), "tenant-b"), "run-1", "call-1"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("other tenant: %v, want ErrNotFound", err)
	}
	if _, err := svc.PendingApproval(tenantctx.WithTenant(context.Background(), "tenant-a"), "run-1", "call-2"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown call: %v, want ErrNotFound", err)
	}

	if !svc.ResolveApproval(tenantctx.WithTenant(context.Background(), "tenant-a"), "run-1", "call-1", "deny") {
		t.Fatal("ResolveApproval found nothing")
	}
	<-done
	if _, err := svc.PendingApproval(tenantctx.WithTenant(context.Background(), "tenant-a"), "run-1", "call-1"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("after the decision: %v, want ErrNotFound", err)
	}
}

func TestPendingApproval_GoneWhenTheRunEnds(t *testing.T) {
	svc, _ := newHITLTestService(30)
	runCtx := tenantctx.WithTenant(context.Background(), "tenant-a")
	done := make(chan struct{})
	go func() {
		defer close(done)
		svc.waitForApproval(runCtx, &event.AGUIPermissionRequestEvent{RunID: "run-2", CallID: "call-1", Tool: "Edit"})
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := svc.PendingApproval(runCtx, "run-2", "call-1"); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("approval never pending")
		}
		time.Sleep(5 * time.Millisecond)
	}
	svc.state.CleanupRun("run-2")
	<-done
	if _, err := svc.PendingApproval(runCtx, "run-2", "call-1"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("after the run ended: %v, want ErrNotFound", err)
	}
}
