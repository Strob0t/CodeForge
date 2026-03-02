package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/Strob0t/CodeForge/internal/domain/orchestration"
	"github.com/Strob0t/CodeForge/internal/domain/trust"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// HandoffService manages agent-to-agent handoffs with context propagation.
type HandoffService struct {
	db         database.Store
	queue      messagequeue.Queue
	quarantine *QuarantineService
}

// SetQuarantineService injects the quarantine service for message interception.
func (s *HandoffService) SetQuarantineService(qs *QuarantineService) { s.quarantine = qs }

// NewHandoffService creates a new HandoffService.
func NewHandoffService(db database.Store, queue messagequeue.Queue) *HandoffService {
	return &HandoffService{db: db, queue: queue}
}

// CreateHandoff initiates a handoff from one agent to another,
// publishing a handoff request to the Python worker.
func (s *HandoffService) CreateHandoff(ctx context.Context, msg *orchestration.HandoffMessage) error {
	if err := msg.Validate(); err != nil {
		return err
	}

	// Stamp internal trust if not already annotated.
	if msg.Trust == nil {
		msg.Trust = trust.Internal(msg.SourceAgentID)
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal handoff: %w", err)
	}

	// Quarantine gate: check message before publishing.
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

	slog.Info("handoff dispatched",
		"source", msg.SourceAgentID,
		"target", msg.TargetAgentID,
		"plan_id", msg.PlanID,
	)
	return nil
}
