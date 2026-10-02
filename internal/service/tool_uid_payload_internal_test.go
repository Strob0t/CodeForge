package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/agent"
	"github.com/Strob0t/CodeForge/internal/domain/tenant"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// KI-96: the payloads that start tool processes carry the tenant's tool UID
// with workspace.tool_acls: required, and omit it with off.

func TestAgentDispatch_SendsTheToolUID(t *testing.T) {
	for _, tc := range []struct {
		name     string
		required bool
		want     int
	}{{"required", true, tenant.ToolUIDMin}, {"off", false, 0}} {
		t.Run(tc.name, func(t *testing.T) {
			probe := registerExecutionProbe(t)
			svc := NewAgentService(dispatchStore("/data/workspaces/proj-1"), &mockQueue{}, &mockBroadcaster{})
			svc.SetToolUIDs(NewToolUIDService(&fakeToolUIDStore{next: tenant.ToolUIDMin}, tc.required))
			ctx := tenantctx.WithTenant(context.Background(), "tenant-1")
			if err := svc.Dispatch(ctx, "agent-1", "task-1"); err != nil {
				t.Fatalf("Dispatch: %v", err)
			}
			got := probe.reset()
			if len(got) != 1 || got[0].ToolUID != tc.want {
				t.Fatalf("executions = %+v, want tool uid %d", got, tc.want)
			}
			if payload := messagequeue.NewTaskAgentPayload(&got[0], "aider"); payload.ToolUID != tc.want {
				t.Fatalf("payload tool_uid = %d, want %d", payload.ToolUID, tc.want)
			}
		})
	}
}

func TestAgentDispatch_ExhaustedToolUIDsStartNothing(t *testing.T) {
	probe := registerExecutionProbe(t)
	store := dispatchStore("/data/workspaces/proj-1")
	svc := NewAgentService(store, &mockQueue{}, &mockBroadcaster{})
	svc.SetToolUIDs(NewToolUIDService(&fakeToolUIDStore{err: tenant.ErrToolUIDRangeExhausted}, true))
	err := svc.Dispatch(tenantctx.WithTenant(context.Background(), "tenant-1"), "agent-1", "task-1")
	if !errors.Is(err, tenant.ErrToolUIDRangeExhausted) {
		t.Fatalf("Dispatch = %v, want ErrToolUIDRangeExhausted", err)
	}
	if got := probe.reset(); len(got) != 0 {
		t.Fatalf("a backend started: %+v", got)
	}
	if store.agents[0].Status != agent.StatusIdle {
		t.Fatalf("agent status = %s, want idle", store.agents[0].Status)
	}
}

func TestRunWorkspaceTest_SendsTheToolUID(t *testing.T) {
	svc, q, _ := newWorkspaceTestEnv(t, func(req *messagequeue.WorkspaceTestRequestPayload) *messagequeue.WorkspaceTestResultPayload {
		return &messagequeue.WorkspaceTestResultPayload{RequestID: req.RequestID, TenantID: req.TenantID, Passed: passedPtr(true), Output: "1 passed"}
	})
	svc.SetToolUIDs(NewToolUIDService(&fakeToolUIDStore{next: 20042}, true))
	if _, err := svc.runWorkspaceTest(tenantctx.WithTenant(context.Background(), "tenant-1"), "proj-1", "conv-1", "test_feature.py"); err != nil {
		t.Fatalf("runWorkspaceTest: %v", err)
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.requests) != 1 || q.requests[0].ToolUID != 20042 {
		t.Fatalf("requests = %+v, want tool uid 20042", q.requests)
	}
}
