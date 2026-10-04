package service_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/conversation"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/policy"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/review"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// Worker messages reach the Go core over NATS without a request tenant.
// Every WebSocket event they cause must be scoped to the run's tenant
// (KI-12): the tenant the worker echoes, or the stored run's tenant.

const (
	runTenantA = "aaaaaaaa-1111-0000-0000-000000000001"
	runTenantB = "bbbbbbbb-2222-0000-0000-000000000002"
)

func assertRunEventsScoped(t *testing.T, bc *runtimeMockBroadcaster, want string) {
	t.Helper()
	events := bc.snapshot()
	if len(events) == 0 {
		t.Fatal("no event was broadcast")
	}
	for _, ev := range events {
		if ev.Tenant != want {
			t.Errorf("event %s scoped to tenant %q, want %q", ev.EventType, ev.Tenant, want)
		}
	}
}

func addTestRun(store *runtimeMockStore, id, tenantID string, status run.Status) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.runs = append(store.runs, run.Run{
		ID:            id,
		TenantID:      tenantID,
		TaskID:        "task-1",
		AgentID:       "agent-1",
		ProjectID:     "proj-1",
		PolicyProfile: "headless-safe-sandbox",
		Status:        status,
		StartedAt:     time.Now(),
	})
}

func TestRuntimeWorkerMessages_ScopeBroadcastsToRunTenant(t *testing.T) {
	passed := true
	toolCallResult := func(svc *service.RuntimeService, tenant string) error {
		return svc.HandleToolCallResult(context.Background(), &messagequeue.ToolCallResultPayload{
			RunID: "run-scope", CallID: "call-1", Tool: "Read", Success: true, TenantID: tenant,
		})
	}
	tests := []struct {
		name          string
		storedTenant  string
		payloadTenant string
		status        run.Status
		handle        func(*service.RuntimeService, string) error
		wantTenant    string
	}{
		{
			name: "tool call request", payloadTenant: runTenantA, status: run.StatusRunning,
			handle: func(svc *service.RuntimeService, tenant string) error {
				return svc.HandleToolCallRequest(context.Background(), &messagequeue.ToolCallRequestPayload{
					RunID: "run-scope", CallID: "call-1", Tool: "Read", Path: "main.go", TenantID: tenant,
				})
			},
			wantTenant: runTenantA,
		},
		{
			name: "tool call result", payloadTenant: runTenantA, status: run.StatusRunning,
			handle: toolCallResult, wantTenant: runTenantA,
		},
		{
			name: "run complete", payloadTenant: runTenantA, status: run.StatusRunning,
			handle: func(svc *service.RuntimeService, tenant string) error {
				return svc.HandleRunComplete(context.Background(), &messagequeue.RunCompletePayload{
					RunID: "run-scope", TaskID: "task-1", ProjectID: "proj-1", Status: "completed", TenantID: tenant,
				})
			},
			wantTenant: runTenantA,
		},
		{
			name: "quality gate result", payloadTenant: runTenantA, status: run.StatusQualityGate,
			handle: func(svc *service.RuntimeService, tenant string) error {
				return svc.HandleQualityGateResult(context.Background(), &messagequeue.QualityGateResultPayload{
					RunID: "run-scope", TestsPassed: &passed, LintPassed: &passed, TenantID: tenant,
				})
			},
			wantTenant: runTenantA,
		},
		{
			name: "stored run tenant when the worker sends none", storedTenant: runTenantB, status: run.StatusRunning,
			handle: toolCallResult, wantTenant: runTenantB,
		},
		{
			name: "no tenant anywhere stays unscoped", status: run.StatusRunning,
			handle: toolCallResult, wantTenant: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, store, _, bc := newRuntimeTestEnv()
			addTestRun(store, "run-scope", tt.storedTenant, tt.status)

			if err := tt.handle(svc, tt.payloadTenant); err != nil {
				t.Fatalf("handle: %v", err)
			}
			assertRunEventsScoped(t, bc, tt.wantTenant)
		})
	}
}

func TestQualityGateRequest_CarriesRunTenant(t *testing.T) {
	svc, store, queue, _ := newRuntimeTestEnv() // headless-safe-sandbox requires tests and lint
	addTestRun(store, "run-gate", "", run.StatusRunning)

	err := svc.HandleRunComplete(context.Background(), &messagequeue.RunCompletePayload{
		RunID: "run-gate", TaskID: "task-1", ProjectID: "proj-1", Status: "completed", TenantID: runTenantA,
	})
	if err != nil {
		t.Fatalf("HandleRunComplete: %v", err)
	}

	msg, ok := queue.lastMessage(messagequeue.SubjectQualityGateRequest)
	if !ok {
		t.Fatal("no quality gate request published")
	}
	var req messagequeue.QualityGateRequestPayload
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if req.TenantID != runTenantA {
		t.Fatalf("quality gate request tenant_id = %q, want %q", req.TenantID, runTenantA)
	}
}

func TestStartRun_ContextTimeoutKeepsTenant(t *testing.T) {
	// The profile belongs to the run's tenant (KI-68).
	ctx := tenantctx.WithTenant(context.Background(), runTenantB)
	policySvc := service.NewPolicyService("one-second", nil)
	if err := policySvc.SaveProfile(ctx, &policy.PolicyProfile{
		Name: "one-second", Mode: policy.ModeDefault,
		Termination: policy.TerminationCondition{TimeoutSeconds: 1, MaxSteps: 10},
	}); err != nil {
		t.Fatal(err)
	}
	svc, _, _, bc := newRuntimeTestEnvWithPolicy(policySvc)

	if _, err := svc.StartRun(ctx, &run.StartRequest{TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1"}); err != nil {
		t.Fatalf("StartRun: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for !hasRunStatus(bc, run.StatusTimeout) {
		if time.Now().After(deadline) {
			t.Fatal("no timeout status was broadcast")
		}
		time.Sleep(20 * time.Millisecond)
	}
	assertRunEventsScoped(t, bc, runTenantB)
}

// The worker echoes the run start's tenant_id in conversation.run.complete;
// without it the run_finished event of a simple chat would be dropped.
func TestSimpleConversationRun_CarriesTenant(t *testing.T) {
	store := &convMockStore{}
	store.projects = []project.Project{{ID: "proj-1", Name: "Test", WorkspacePath: "/tmp/test"}}
	svc := service.NewConversationService(store, &runtimeMockBroadcaster{}, "gpt-4o", service.NewModeService())
	q := &captureQueue{}
	svc.SetQueue(q)
	svc.SetAgentConfig(&config.Agent{DefaultModel: "gpt-4o"})
	ctx := tenantctx.WithTenant(context.Background(), runTenantA)
	conv, err := svc.Create(ctx, conversation.CreateRequest{ProjectID: "proj-1", Title: "tenant"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, err := svc.SendMessage(ctx, conv.ID, &conversation.SendMessageRequest{Content: "Hello"}); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	subject, data := q.snapshot()
	if subject != messagequeue.SubjectConversationRunStart {
		t.Fatalf("published %q, want %q", subject, messagequeue.SubjectConversationRunStart)
	}
	var payload messagequeue.ConversationRunStartPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload.TenantID != runTenantA {
		t.Fatalf("run start tenant_id = %q, want %q", payload.TenantID, runTenantA)
	}
}

// A review triggered without a request tenant (cron scheduler) runs in the
// policy's tenant, so its status event reaches only that tenant.
func TestReviewTrigger_ScopesToPolicyTenant(t *testing.T) {
	svc, _, hub := newReviewTestEnvWithHub()
	p, err := svc.CreatePolicy(context.Background(), "proj-1", runTenantB, &review.CreatePolicyRequest{
		Name: "nightly", TriggerType: review.TriggerCommitCount, CommitThreshold: 100,
	})
	if err != nil {
		t.Fatalf("CreatePolicy: %v", err)
	}

	if _, err := svc.ManualTrigger(context.Background(), p.ID); err != nil {
		t.Fatalf("ManualTrigger: %v", err)
	}

	var reviewEvents int
	for _, ev := range hub.snapshot() {
		if ev.EventType != event.EventReviewStatus {
			continue
		}
		reviewEvents++
		if ev.Tenant != runTenantB {
			t.Errorf("review status scoped to %q, want the policy tenant %q", ev.Tenant, runTenantB)
		}
	}
	if reviewEvents == 0 {
		t.Fatal("no review status event was broadcast")
	}
}

func hasRunStatus(bc *runtimeMockBroadcaster, status run.Status) bool {
	for _, ev := range bc.snapshot() {
		if st, ok := ev.Data.(event.RunStatusEvent); ok && st.Status == string(status) {
			return true
		}
	}
	return false
}
