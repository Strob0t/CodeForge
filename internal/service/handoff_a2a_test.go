package service

import (
	"context"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/orchestration"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

type mockQueueForHandoff struct {
	publishCount int
	publishErr   error
}

func (m *mockQueueForHandoff) Publish(_ context.Context, _ string, _ []byte) error {
	m.publishCount++
	return m.publishErr
}

func (m *mockQueueForHandoff) Subscribe(_ context.Context, _ string, _ messagequeue.Handler) (func(), error) {
	return func() {}, nil
}

func (m *mockQueueForHandoff) Drain() error      { return nil }
func (m *mockQueueForHandoff) Close() error      { return nil }
func (m *mockQueueForHandoff) IsConnected() bool { return true }

func TestHandoff_A2ATarget_NilService(t *testing.T) {
	q := &mockQueueForHandoff{}
	ms := &mockStoreForA2A{}
	svc := NewHandoffService(ms, q)

	msg := &orchestration.HandoffMessage{
		SourceAgentID: "agent-1",
		TargetAgentID: "a2a://remote-agent-1",
		PlanID:        "plan-1",
		StepID:        "step-1",
		Context:       "test handoff",
	}
	err := svc.CreateHandoff(context.Background(), msg)
	if err == nil {
		t.Fatal("expected error when A2A service is nil for a2a:// target")
	}
}

func TestHandoff_NormalTarget_NoA2A(t *testing.T) {
	q := &mockQueueForHandoff{}
	ms := &mockStoreForA2A{}
	svc := NewHandoffService(ms, q)

	msg := &orchestration.HandoffMessage{
		SourceAgentID: "agent-1",
		TargetAgentID: "agent-2",
		PlanID:        "plan-1",
		StepID:        "step-1",
		Context:       "normal handoff",
	}
	err := svc.CreateHandoff(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.publishCount != 1 {
		t.Errorf("expected 1 NATS publish, got %d", q.publishCount)
	}
}

func TestHandoff_A2ATarget_EmptyRemoteID(t *testing.T) {
	q := &mockQueueForHandoff{}
	ms := &mockStoreForA2A{}
	svc := NewHandoffService(ms, q)
	a2aSvc := NewA2AService(ms, q)
	svc.SetA2AService(a2aSvc)

	msg := &orchestration.HandoffMessage{
		SourceAgentID: "agent-1",
		TargetAgentID: "a2a://",
		PlanID:        "plan-1",
		StepID:        "step-1",
		Context:       "empty remote",
	}
	err := svc.CreateHandoff(context.Background(), msg)
	if err == nil {
		t.Fatal("expected error for empty remote agent ID")
	}
}
