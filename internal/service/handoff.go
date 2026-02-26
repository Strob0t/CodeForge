package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/Strob0t/CodeForge/internal/domain/orchestration"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// HandoffService manages agent-to-agent handoffs with context propagation.
type HandoffService struct {
	db    database.Store
	queue messagequeue.Queue
}

// NewHandoffService creates a new HandoffService.
func NewHandoffService(db database.Store, queue messagequeue.Queue) *HandoffService {
	return &HandoffService{db: db, queue: queue}
}

// CreateHandoff initiates a handoff from one agent to another,
// publishing a handoff request to the Python worker.
func (s *HandoffService) CreateHandoff(ctx context.Context, msg orchestration.HandoffMessage) error {
	if err := msg.Validate(); err != nil {
		return err
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal handoff: %w", err)
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
