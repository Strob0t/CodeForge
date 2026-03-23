package service_test

import (
	"context"
	"sync"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/orchestration"
	"github.com/Strob0t/CodeForge/internal/service"
)

type handoffMockBroadcaster struct {
	mu     sync.Mutex
	events []broadcastCapture
}

type broadcastCapture struct {
	eventType string
	payload   any
}

func (b *handoffMockBroadcaster) BroadcastEvent(_ context.Context, eventType string, payload any) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, broadcastCapture{eventType: eventType, payload: payload})
}

func TestHandoff_BroadcastsToWSHub(t *testing.T) {
	store := &runtimeMockStore{}
	queue := &handoffMockQueue{}
	hub := &handoffMockBroadcaster{}
	svc := service.NewHandoffService(store, queue, hub)
	ctx := context.Background()
	msg := &orchestration.HandoffMessage{
		SourceAgentID: "agent-a",
		TargetAgentID: "agent-b",
		Context:       "Review the auth module",
		PlanID:        "plan-42",
		StepID:        "step-7",
	}
	if err := svc.CreateHandoff(ctx, msg); err != nil {
		t.Fatalf("CreateHandoff: %v", err)
	}
	hub.mu.Lock()
	defer hub.mu.Unlock()
	if len(hub.events) != 1 {
		t.Fatalf("expected 1 broadcast event, got %d", len(hub.events))
	}
	evt := hub.events[0]
	if evt.eventType != event.EventHandoffStatus {
		t.Errorf("expected event type %q, got %q", event.EventHandoffStatus, evt.eventType)
	}
	hsEvt, ok := evt.payload.(event.HandoffStatusEvent)
	if !ok {
		t.Fatalf("expected HandoffStatusEvent payload, got %T", evt.payload)
	}
	if hsEvt.SourceAgentID != "agent-a" {
		t.Errorf("expected source agent-a, got %q", hsEvt.SourceAgentID)
	}
	if hsEvt.TargetAgentID != "agent-b" {
		t.Errorf("expected target agent-b, got %q", hsEvt.TargetAgentID)
	}
	if hsEvt.Status != "initiated" {
		t.Errorf("expected status 'initiated', got %q", hsEvt.Status)
	}
	if hsEvt.PlanID != "plan-42" {
		t.Errorf("expected plan_id 'plan-42', got %q", hsEvt.PlanID)
	}
	if hsEvt.Context != "Review the auth module" {
		t.Errorf("expected context 'Review the auth module', got %q", hsEvt.Context)
	}
}

func TestHandoff_BackwardCompatibleWithoutHub(t *testing.T) {
	store := &runtimeMockStore{}
	queue := &handoffMockQueue{}
	svc := service.NewHandoffService(store, queue)
	ctx := context.Background()
	msg := &orchestration.HandoffMessage{
		SourceAgentID: "agent-1",
		TargetAgentID: "agent-2",
		Context:       "Some context",
	}
	if err := svc.CreateHandoff(ctx, msg); err != nil {
		t.Fatalf("CreateHandoff without hub: %v", err)
	}
	if queue.subject != "handoff.request" {
		t.Errorf("expected subject 'handoff.request', got %q", queue.subject)
	}
}
