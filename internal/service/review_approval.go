package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/port/broadcast"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// ReviewApprovalService publishes review.approval.required events when a
// review run's diff impact requires human approval, and subscribes to
// the same subject to broadcast the notification over WebSocket.
type ReviewApprovalService struct {
	queue messagequeue.Queue
	hub   broadcast.Broadcaster
}

// NewReviewApprovalService creates a ReviewApprovalService.
func NewReviewApprovalService(queue messagequeue.Queue, hub broadcast.Broadcaster) *ReviewApprovalService {
	return &ReviewApprovalService{queue: queue, hub: hub}
}

// PublishApprovalRequired sends a review.approval.required event on NATS.
// Callers invoke this when a diff impact score triggers HITL approval.
func (s *ReviewApprovalService) PublishApprovalRequired(
	ctx context.Context,
	runID, projectID, tenantID string,
	impactLevel ImpactLevel,
	stats DiffStats,
) {
	if s.queue == nil {
		return
	}
	payload := messagequeue.ReviewApprovalRequiredPayload{
		RunID:     runID,
		ProjectID: projectID,
		TenantID:  tenantID,
		DiffStats: messagequeue.ReviewDiffStats{
			FilesChanged: stats.FilesChanged,
			LinesAdded:   stats.LinesAdded,
			LinesRemoved: stats.LinesRemoved,
			CrossLayer:   stats.CrossLayer,
			Structural:   stats.Structural,
		},
		ImpactLevel: string(impactLevel),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		slog.Error("marshal review approval required", "run_id", runID, "error", err)
		return
	}
	if pubErr := s.queue.Publish(ctx, messagequeue.SubjectReviewApprovalRequired, data); pubErr != nil {
		slog.Warn("failed to publish review approval required", "run_id", runID, "error", pubErr)
	}
}

// HandleApprovalRequired processes a review.approval.required NATS message
// and broadcasts it to WebSocket clients so the frontend can show the
// approval UI.
func (s *ReviewApprovalService) HandleApprovalRequired(ctx context.Context, _ string, data []byte) error {
	var payload messagequeue.ReviewApprovalRequiredPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("unmarshal review approval required: %w", err)
	}
	slog.Info("review approval required",
		"run_id", payload.RunID,
		"project_id", payload.ProjectID,
		"impact_level", payload.ImpactLevel,
	)
	s.hub.BroadcastEvent(ctx, event.EventReviewApprovalRequired, payload)
	return nil
}

// StartSubscriber subscribes to review.approval.required and broadcasts
// each message to WebSocket clients.
func (s *ReviewApprovalService) StartSubscriber(ctx context.Context) (func(), error) {
	if s.queue == nil {
		return func() {}, nil
	}
	return s.queue.Subscribe(ctx, messagequeue.SubjectReviewApprovalRequired, s.HandleApprovalRequired)
}
