package service

import (
	"context"
	"strings"
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

func (m *mockQueueForHandoff) PublishWithDedup(_ context.Context, _ string, _ []byte, _ string) error {
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
	// A local target is not routed to A2A: it needs the run starter, which
	// this service has not (KI-15: the Go Core starts the target's run).
	err := svc.CreateHandoff(context.Background(), msg)
	if err == nil || !strings.Contains(err.Error(), "handoff runs are not configured") {
		t.Fatalf("CreateHandoff = %v, want the missing run starter", err)
	}
	if q.publishCount != 0 {
		t.Errorf("expected no NATS publish, got %d", q.publishCount)
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
