package service_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

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
