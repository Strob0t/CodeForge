package service_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/orchestration"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// handoffMockQueue captures published messages for verification.
type handoffMockQueue struct {
	subject string
	data    []byte
}

func (m *handoffMockQueue) Publish(_ context.Context, subject string, data []byte) error {
	m.subject = subject
	m.data = data
	return nil
}

func (m *handoffMockQueue) PublishWithDedup(ctx context.Context, subject string, data []byte, _ string) error {
	return m.Publish(ctx, subject, data)
}

func (m *handoffMockQueue) Subscribe(_ context.Context, _ string, _ messagequeue.Handler) (func(), error) {
	return func() {}, nil
}

func (m *handoffMockQueue) Drain() error      { return nil }
func (m *handoffMockQueue) Close() error      { return nil }
func (m *handoffMockQueue) IsConnected() bool { return true }

// handoffCtx is a context of tenant A, whose agents the handoff store knows.
func handoffCtx() context.Context {
	return tenantctx.WithTenant(context.Background(), handoffTenantA)
}

// TestHandoff_CreateHandoff: a handoff starts a run of the target agent
// (KI-15: it used to be published to handoff.request for the worker to
// start a run the Go Core did not know). A message without trust comes
// from an internal agent.
func TestHandoff_CreateHandoff(t *testing.T) {
	env := newHandoffEnv(t, false)
	ctx := handoffCtx()

	// 1. Valid handoff succeeds
	msg := &orchestration.HandoffMessage{
		ProjectID:     "proj-1",
		SourceAgentID: "agent-src",
		TargetAgentID: "agent-tgt",
		Context:       "Please review this implementation",
		PlanID:        "plan-1",
	}
	if err := env.svc.CreateHandoff(ctx, msg); err != nil {
		t.Fatalf("CreateHandoff: %v", err)
	}
	if len(env.runs.started) != 1 || env.runs.started[0].AgentID != "agent-tgt" {
		t.Fatalf("runs started = %+v, want agent-tgt's run", env.runs.started)
	}

	// Verify trust was auto-stamped
	if msg.Trust == nil {
		t.Fatal("expected trust annotation to be auto-stamped")
	}
	if msg.Trust.Origin != "internal" {
		t.Errorf("expected trust origin 'internal', got %q", msg.Trust.Origin)
	}
	if msg.Trust.SourceID != "agent-src" {
		t.Errorf("expected trust source_id 'agent-src', got %q", msg.Trust.SourceID)
	}

	// 2. Missing source agent fails validation
	err := env.svc.CreateHandoff(ctx, &orchestration.HandoffMessage{
		TargetAgentID: "agent-tgt",
		Context:       "some context",
	})
	if err == nil {
		t.Fatal("expected error for missing source_agent_id")
	}
	if !strings.Contains(err.Error(), "source_agent_id") {
		t.Errorf("expected error about source_agent_id, got: %s", err.Error())
	}

	// 3. Missing context fails validation
	err = env.svc.CreateHandoff(ctx, &orchestration.HandoffMessage{
		SourceAgentID: "agent-src",
		TargetAgentID: "agent-tgt",
	})
	if err == nil {
		t.Fatal("expected error for missing context")
	}
	if !strings.Contains(err.Error(), "context") {
		t.Errorf("expected error about context, got: %s", err.Error())
	}

	// 4. Missing target agent fails validation
	err = env.svc.CreateHandoff(ctx, &orchestration.HandoffMessage{
		SourceAgentID: "agent-src",
		Context:       "some context",
	})
	if err == nil {
		t.Fatal("expected error for missing target_agent_id")
	}
	if len(env.runs.started) != 1 {
		t.Errorf("invalid handoffs started runs: %+v", env.runs.started)
	}
}

// TestHandoffService_CreateHandoff_WithQuarantine verifies that a handoff
// of an internal agent (full trust) passes an enabled quarantine without
// being stored and starts its run.
func TestHandoffService_CreateHandoff_WithQuarantine(t *testing.T) {
	env := newHandoffEnv(t, true)
	msg := &orchestration.HandoffMessage{
		ProjectID:     "proj-1",
		SourceAgentID: "agent-src",
		TargetAgentID: "agent-tgt",
		Context:       "Review with quarantine enabled",
		PlanID:        "plan-q1",
	}
	if err := env.svc.CreateHandoff(handoffCtx(), msg); err != nil {
		t.Fatalf("CreateHandoff with quarantine: %v", err)
	}
	if len(env.runs.started) != 1 {
		t.Errorf("runs started = %d, want 1", len(env.runs.started))
	}
	if len(env.store.quarantined) != 0 {
		t.Errorf("quarantined = %d, want 0: full trust bypasses the quarantine", len(env.store.quarantined))
	}
}

// TestHandoffService_CreateHandoff_NilHub verifies that creating a handoff
// without a WS hub (nil) does not panic or error. This is the backward-
// compatible case where War Room broadcasting is not configured.
func TestHandoffService_CreateHandoff_NilHub(t *testing.T) {
	store := newHandoffStore()
	runs := &recordingRunStarter{}
	svc := service.NewHandoffService(store, &handoffMockQueue{})
	svc.SetRunStarter(runs)

	msg := &orchestration.HandoffMessage{
		ProjectID:     "proj-1",
		SourceAgentID: "agent-src",
		TargetAgentID: "agent-tgt",
		Context:       "Handoff without WS hub",
		PlanID:        "plan-nil-hub",
	}
	if err := svc.CreateHandoff(handoffCtx(), msg); err != nil {
		t.Fatalf("CreateHandoff with nil hub: %v", err)
	}
	if len(runs.started) != 1 || runs.started[0].AgentID != "agent-tgt" {
		t.Errorf("runs started = %+v, want agent-tgt's run", runs.started)
	}
}

// TestHandoffService_CreateHandoff_A2ATarget_NilService verifies that
// targeting an A2A agent (a2a:// prefix) when no A2A service is configured
// returns a descriptive error rather than panicking.
func TestHandoffService_CreateHandoff_A2ATarget_NilService(t *testing.T) {
	store := &runtimeMockStore{}
	queue := &handoffMockQueue{}

	// No A2A service set -- default is nil.
	svc := service.NewHandoffService(store, queue)

	ctx := context.Background()
	msg := &orchestration.HandoffMessage{
		SourceAgentID: "agent-local",
		TargetAgentID: "a2a://remote-agent-42",
		Context:       "Delegate to remote agent",
		PlanID:        "plan-a2a",
		StepID:        "step-1",
	}

	err := svc.CreateHandoff(ctx, msg)
	if err == nil {
		t.Fatal("expected error for a2a:// target with nil A2A service")
	}
	if !strings.Contains(err.Error(), "a2a service not configured") {
		t.Errorf("expected error about a2a service not configured, got: %s", err.Error())
	}
}

// TestHandoffService_CreateHandoff_ValidationError verifies that Validate()
// errors are surfaced correctly for each required field.
func TestHandoffService_CreateHandoff_ValidationError(t *testing.T) {
	store := &runtimeMockStore{}
	queue := &handoffMockQueue{}
	svc := service.NewHandoffService(store, queue)
	ctx := context.Background()

	tests := []struct {
		name    string
		msg     *orchestration.HandoffMessage
		wantErr string
	}{
		{
			name: "empty source_agent_id",
			msg: &orchestration.HandoffMessage{
				SourceAgentID: "",
				TargetAgentID: "agent-2",
				Context:       "some context",
			},
			wantErr: "source_agent_id",
		},
		{
			name: "empty target_agent_id",
			msg: &orchestration.HandoffMessage{
				SourceAgentID: "agent-1",
				TargetAgentID: "",
				Context:       "some context",
			},
			wantErr: "target_agent_id",
		},
		{
			name: "empty context",
			msg: &orchestration.HandoffMessage{
				SourceAgentID: "agent-1",
				TargetAgentID: "agent-2",
				Context:       "",
			},
			wantErr: "context",
		},
		{
			name: "all fields empty",
			msg: &orchestration.HandoffMessage{
				SourceAgentID: "",
				TargetAgentID: "",
				Context:       "",
			},
			wantErr: "source_agent_id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.CreateHandoff(ctx, tt.msg)
			if err == nil {
				t.Fatalf("expected validation error for %s", tt.name)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("expected error containing %q, got: %s", tt.wantErr, err.Error())
			}
		})
	}
}

// The handoff run is started in the caller's tenant; a message cannot move
// it to another one. Without a tenant in the context the message's tenant is
// used (KI-12).
func TestHandoffService_CreateHandoff_CarriesTenantFromContext(t *testing.T) {
	tests := []struct {
		name       string
		ctxTenant  string
		msgTenant  string
		wantTenant string
	}{
		{name: "tenant from context", ctxTenant: handoffTenantA, wantTenant: handoffTenantA},
		{name: "context wins over message", ctxTenant: handoffTenantA, msgTenant: handoffTenantB, wantTenant: handoffTenantA},
		{name: "no tenant in context keeps message tenant", msgTenant: handoffTenantA, wantTenant: handoffTenantA},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newHandoffEnv(t, false)
			ctx := context.Background()
			if tt.ctxTenant != "" {
				ctx = tenantctx.WithTenant(ctx, tt.ctxTenant)
			}
			msg := &orchestration.HandoffMessage{
				ProjectID:     "proj-1",
				SourceAgentID: "agent-src",
				TargetAgentID: "agent-tgt",
				Context:       "continue",
				TenantID:      tt.msgTenant,
			}
			if err := env.svc.CreateHandoff(ctx, msg); err != nil {
				t.Fatalf("CreateHandoff: %v", err)
			}
			if msg.TenantID != tt.wantTenant || len(env.runs.tenants) != 1 || env.runs.tenants[0] != tt.wantTenant {
				t.Errorf("message tenant %q, run tenants %v; want %q", msg.TenantID, env.runs.tenants, tt.wantTenant)
			}
		})
	}
}
