package service_test

import (
	"context"
	"sync"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/orchestration"
	"github.com/Strob0t/CodeForge/internal/service"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

type handoffMockBroadcaster struct {
	mu     sync.Mutex
	events []broadcastCapture
}

type broadcastCapture struct {
	eventType string
	payload   any
	tenant    string
}

func (b *handoffMockBroadcaster) BroadcastEvent(ctx context.Context, eventType string, payload any) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, broadcastCapture{eventType: eventType, payload: payload, tenant: tenantctx.FromContext(ctx)})
}

func TestHandoff_BroadcastsToWSHub(t *testing.T) {
	env := newHandoffEnv(t, false)
	hub := env.hub
	msg := &orchestration.HandoffMessage{
		ProjectID:     "proj-1",
		SourceAgentID: "agent-src",
		TargetAgentID: "agent-tgt",
		Context:       "Review the auth module",
		PlanID:        "plan-42",
		StepID:        "step-7",
	}
	if err := env.svc.CreateHandoff(handoffCtx(), msg); err != nil {
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
	if hsEvt.SourceAgentID != "agent-src" {
		t.Errorf("expected source agent-src, got %q", hsEvt.SourceAgentID)
	}
	if hsEvt.TargetAgentID != "agent-tgt" {
		t.Errorf("expected target agent-tgt, got %q", hsEvt.TargetAgentID)
	}
	if hsEvt.RunID == "" {
		t.Error("expected the target agent's run in the event")
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
	runs := &recordingRunStarter{}
	svc := service.NewHandoffService(newHandoffStore(), &handoffMockQueue{})
	svc.SetRunStarter(runs)
	msg := &orchestration.HandoffMessage{
		ProjectID:     "proj-1",
		SourceAgentID: "agent-src",
		TargetAgentID: "agent-tgt",
		Context:       "Some context",
	}
	if err := svc.CreateHandoff(handoffCtx(), msg); err != nil {
		t.Fatalf("CreateHandoff without hub: %v", err)
	}
	if len(runs.started) != 1 {
		t.Errorf("runs started = %d, want 1", len(runs.started))
	}
}
