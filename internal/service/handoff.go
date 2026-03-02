package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/Strob0t/CodeForge/internal/adapter/ws"
	"github.com/Strob0t/CodeForge/internal/domain/agent"
	"github.com/Strob0t/CodeForge/internal/domain/orchestration"
	"github.com/Strob0t/CodeForge/internal/domain/trust"
	"github.com/Strob0t/CodeForge/internal/port/broadcast"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// HandoffService handles agent-to-agent handoff messaging (Phase 23B).
type HandoffService struct {
	db         database.Store
	queue      messagequeue.Queue
	hub        broadcast.Broadcaster
	quarantine *QuarantineService
}

// SetQuarantineService injects the quarantine evaluator (circular-dep breaker).
func (s *HandoffService) SetQuarantineService(qs *QuarantineService) { s.quarantine = qs }

// NewHandoffService creates a HandoffService. The optional hub parameter enables
// WS broadcasting for the War Room (Phase 23D).
func NewHandoffService(db database.Store, queue messagequeue.Queue, hub ...broadcast.Broadcaster) *HandoffService {
	svc := &HandoffService{db: db, queue: queue}
	if len(hub) > 0 {
		svc.hub = hub[0]
	}
	return svc
}

// CreateHandoff dispatches a handoff message from one agent to another.
func (s *HandoffService) CreateHandoff(ctx context.Context, msg *orchestration.HandoffMessage) error {
	if err := msg.Validate(); err != nil {
		return err
	}

	// Auto-stamp trust annotation if not provided.
	if msg.Trust == nil {
		msg.Trust = trust.Internal(msg.SourceAgentID)
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal handoff: %w", err)
	}

	// Quarantine evaluation (Phase 23B).
	if s.quarantine != nil {
		blocked, qErr := s.quarantine.Evaluate(ctx, msg.Trust, "handoff.request", data, "")
		if qErr != nil {
			slog.Warn("quarantine evaluation failed, allowing handoff", "error", qErr)
		}
		if blocked {
			slog.Info("handoff quarantined", "source", msg.SourceAgentID, "target", msg.TargetAgentID)
			return nil
		}
	}

	if err := s.queue.Publish(ctx, "handoff.request", data); err != nil {
		return fmt.Errorf("publish handoff: %w", err)
	}

	// Deliver inbox message to target agent.
	inboxMsg := &agent.InboxMessage{
		AgentID:   msg.TargetAgentID,
		FromAgent: msg.SourceAgentID,
		Content:   fmt.Sprintf("Handoff: %s", msg.Context),
		Priority:  1,
	}
	if err := s.db.SendAgentMessage(ctx, inboxMsg); err != nil {
		slog.Warn("failed to deliver handoff inbox message", "target", msg.TargetAgentID, "error", err)
	}

	// Broadcast to WS for War Room (Phase 23D).
	if s.hub != nil {
		s.hub.BroadcastEvent(ctx, ws.EventHandoffStatus, ws.HandoffStatusEvent{
			SourceAgentID: msg.SourceAgentID,
			TargetAgentID: msg.TargetAgentID,
			PlanID:        msg.PlanID,
			StepID:        msg.StepID,
			Status:        "initiated",
			Context:       msg.Context,
		})
	}

	slog.Info("handoff dispatched",
		"source", msg.SourceAgentID,
		"target", msg.TargetAgentID,
		"plan_id", msg.PlanID,
	)
	return nil
}
