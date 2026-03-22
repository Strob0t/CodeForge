package service_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/orchestration"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
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

func TestHandoff_CreateHandoff(t *testing.T) {
	store := &runtimeMockStore{}
	queue := &handoffMockQueue{}
	svc := service.NewHandoffService(store, queue)
	ctx := context.Background()

	// 1. Valid handoff succeeds
	msg := &orchestration.HandoffMessage{
		SourceAgentID: "agent-1",
		TargetAgentID: "agent-2",
		Context:       "Please review this implementation",
		PlanID:        "plan-1",
	}
	if err := svc.CreateHandoff(ctx, msg); err != nil {
		t.Fatalf("CreateHandoff: %v", err)
	}

	// Verify message was published to correct subject
	if queue.subject != "handoff.request" {
		t.Errorf("expected subject 'handoff.request', got %q", queue.subject)
	}

	// Verify trust was auto-stamped
	var published orchestration.HandoffMessage
	if err := json.Unmarshal(queue.data, &published); err != nil {
		t.Fatalf("unmarshal published: %v", err)
	}
	if published.Trust == nil {
		t.Fatal("expected trust annotation to be auto-stamped")
	}
	if published.Trust.Origin != "internal" {
		t.Errorf("expected trust origin 'internal', got %q", published.Trust.Origin)
	}
	if published.Trust.SourceID != "agent-1" {
		t.Errorf("expected trust source_id 'agent-1', got %q", published.Trust.SourceID)
	}

	// 2. Missing source agent fails validation
	err := svc.CreateHandoff(ctx, &orchestration.HandoffMessage{
		TargetAgentID: "agent-2",
		Context:       "some context",
	})
	if err == nil {
		t.Fatal("expected error for missing source_agent_id")
	}
	if !strings.Contains(err.Error(), "source_agent_id") {
		t.Errorf("expected error about source_agent_id, got: %s", err.Error())
	}

	// 3. Missing context fails validation
	err = svc.CreateHandoff(ctx, &orchestration.HandoffMessage{
		SourceAgentID: "agent-1",
		TargetAgentID: "agent-2",
	})
	if err == nil {
		t.Fatal("expected error for missing context")
	}
	if !strings.Contains(err.Error(), "context") {
		t.Errorf("expected error about context, got: %s", err.Error())
	}

	// 4. Missing target agent fails validation
	err = svc.CreateHandoff(ctx, &orchestration.HandoffMessage{
		SourceAgentID: "agent-1",
		Context:       "some context",
	})
	if err == nil {
		t.Fatal("expected error for missing target_agent_id")
	}
}

// TestHandoffService_CreateHandoff_WithQuarantine verifies that when a
// QuarantineService is attached and enabled, the handoff still proceeds
// when quarantine evaluation allows through (internal trust bypasses
// quarantine thresholds). The quarantine code path is exercised without
// blocking the handoff.
func TestHandoffService_CreateHandoff_WithQuarantine(t *testing.T) {
	store := &runtimeMockStore{}
	queue := &handoffMockQueue{}
	hub := &handoffMockBroadcaster{}

	svc := service.NewHandoffService(store, queue, hub)

	// Create a quarantine service with quarantine enabled. Internal messages
	// auto-stamp LevelFull trust, which meets the "verified" MinTrustBypass
	// threshold, so the quarantine evaluator allows through without error.
	qsCfg := config.Quarantine{
		Enabled:             true,
		QuarantineThreshold: 0.7,
		BlockThreshold:      0.95,
		MinTrustBypass:      "verified",
		ExpiryHours:         72,
	}
	qs := service.NewQuarantineService(store, queue, hub, qsCfg)
	svc.SetQuarantineService(qs)

	ctx := context.Background()
	msg := &orchestration.HandoffMessage{
		SourceAgentID: "agent-1",
		TargetAgentID: "agent-2",
		Context:       "Review with quarantine enabled",
		PlanID:        "plan-q1",
	}

	// Handoff should succeed; quarantine evaluates but bypasses due to
	// full trust level.
	if err := svc.CreateHandoff(ctx, msg); err != nil {
		t.Fatalf("CreateHandoff with quarantine: %v", err)
	}
	if queue.subject != "handoff.request" {
		t.Errorf("expected subject 'handoff.request', got %q", queue.subject)
	}
}

// TestHandoffService_CreateHandoff_NilHub verifies that creating a handoff
// without a WS hub (nil) does not panic or error. This is the backward-
// compatible case where War Room broadcasting is not configured.
func TestHandoffService_CreateHandoff_NilHub(t *testing.T) {
	store := &runtimeMockStore{}
	queue := &handoffMockQueue{}

	// Pass no hub argument -- hub field stays nil.
	svc := service.NewHandoffService(store, queue)

	ctx := context.Background()
	msg := &orchestration.HandoffMessage{
		SourceAgentID: "agent-x",
		TargetAgentID: "agent-y",
		Context:       "Handoff without WS hub",
		PlanID:        "plan-nil-hub",
	}

	// Should succeed without panic despite nil hub.
	if err := svc.CreateHandoff(ctx, msg); err != nil {
		t.Fatalf("CreateHandoff with nil hub: %v", err)
	}
	if queue.subject != "handoff.request" {
		t.Errorf("expected subject 'handoff.request', got %q", queue.subject)
	}

	// Verify the message was actually published (not silently dropped).
	var published orchestration.HandoffMessage
	if err := json.Unmarshal(queue.data, &published); err != nil {
		t.Fatalf("unmarshal published: %v", err)
	}
	if published.SourceAgentID != "agent-x" {
		t.Errorf("expected source 'agent-x', got %q", published.SourceAgentID)
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
