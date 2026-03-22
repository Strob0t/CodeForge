package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/Strob0t/CodeForge/internal/adapter/ws"
	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/quarantine"
	"github.com/Strob0t/CodeForge/internal/domain/trust"
	"github.com/Strob0t/CodeForge/internal/port/broadcast"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// QuarantineService manages message quarantine, risk scoring, and admin review.
type QuarantineService struct {
	db    database.Store
	queue messagequeue.Queue
	hub   broadcast.Broadcaster
	cfg   config.Quarantine
}

// NewQuarantineService creates a new QuarantineService.
func NewQuarantineService(db database.Store, queue messagequeue.Queue, hub broadcast.Broadcaster, cfg config.Quarantine) *QuarantineService {
	return &QuarantineService{db: db, queue: queue, hub: hub, cfg: cfg}
}

// Evaluate checks a message against quarantine thresholds. Returns true if the
// message was blocked (quarantined or rejected). Follows a fail-closed policy:
// if evaluation or persistence errors, the message is blocked.
func (s *QuarantineService) Evaluate(ctx context.Context, ann *trust.Annotation, subject string, payload []byte, projectID string) (bool, error) {
	if !s.cfg.Enabled {
		return false, nil
	}

	// Verify project belongs to caller's tenant (fail-closed).
	if projectID != "" {
		if _, err := s.db.GetProject(ctx, projectID); err != nil {
			slog.Warn("quarantine: project access check failed, blocking message",
				"project_id", projectID, "error", err)
			return true, fmt.Errorf("quarantine project access check: %w", err)
		}
	}

	// Messages from sufficiently trusted sources bypass quarantine.
	if ann != nil && ann.MeetsMinimum(trust.Level(s.cfg.MinTrustBypass)) {
		return false, nil
	}

	score, factors := quarantine.ScoreMessage(ann, payload)

	// Below quarantine threshold — allow through.
	if score < s.cfg.QuarantineThreshold {
		return false, nil
	}

	trustOrigin := ""
	trustLevel := ""
	if ann != nil {
		trustOrigin = ann.Origin
		trustLevel = string(ann.TrustLevel)
	}

	now := time.Now().UTC()
	msg := &quarantine.Message{
		ProjectID:   projectID,
		Subject:     subject,
		Payload:     payload,
		TrustOrigin: trustOrigin,
		TrustLevel:  trustLevel,
		RiskScore:   score,
		RiskFactors: factors,
		CreatedAt:   now,
		ExpiresAt:   now.Add(time.Duration(s.cfg.ExpiryHours) * time.Hour),
	}

	// Above block threshold — reject immediately.
	if score >= s.cfg.BlockThreshold {
		msg.Status = quarantine.StatusRejected
		msg.ReviewNote = "auto-blocked: risk score exceeds block threshold"
		if err := s.db.QuarantineMessage(ctx, msg); err != nil {
			slog.Error("failed to store auto-blocked message, blocking anyway (fail-closed)", "error", err)
			// Fail-closed: block the message even if DB persistence fails.
			return true, fmt.Errorf("quarantine db error: %w", err)
		}
		slog.Warn("message auto-blocked",
			"subject", subject, "score", score, "factors", factors)
		return true, nil
	}

	// Between quarantine and block thresholds — hold for review.
	msg.Status = quarantine.StatusPending
	if err := s.db.QuarantineMessage(ctx, msg); err != nil {
		slog.Error("failed to quarantine message, blocking anyway (fail-closed)", "error", err)
		// Fail-closed: block the message even if DB persistence fails.
		return true, fmt.Errorf("quarantine db error: %w", err)
	}

	// Broadcast alert to admin UI.
	s.hub.BroadcastEvent(ctx, ws.EventQuarantineAlert, map[string]any{
		"id":         msg.ID,
		"project_id": projectID,
		"subject":    subject,
		"risk_score": score,
		"factors":    factors,
	})

	slog.Info("message quarantined",
		"id", msg.ID, "subject", subject, "score", score, "factors", factors)
	return true, nil
}

// Approve releases a quarantined message, replaying the original payload to NATS.
func (s *QuarantineService) Approve(ctx context.Context, id, reviewedBy, note string) error {
	msg, err := s.db.GetQuarantinedMessage(ctx, id)
	if err != nil {
		return fmt.Errorf("get quarantined message: %w", err)
	}
	if msg.Status != quarantine.StatusPending {
		return fmt.Errorf("message %s is not pending (status: %s)", id, msg.Status)
	}

	if err := s.db.UpdateQuarantineStatus(ctx, id, quarantine.StatusApproved, reviewedBy, note); err != nil {
		return fmt.Errorf("update quarantine status: %w", err)
	}

	// Replay original payload byte-for-byte to the original NATS subject.
	if err := s.queue.Publish(ctx, msg.Subject, msg.Payload); err != nil {
		return fmt.Errorf("replay quarantined message: %w", err)
	}

	s.hub.BroadcastEvent(ctx, ws.EventQuarantineResolved, map[string]any{
		"id":          id,
		"project_id":  msg.ProjectID,
		"action":      "approved",
		"reviewed_by": reviewedBy,
	})

	slog.Info("quarantined message approved and replayed",
		"id", id, "subject", msg.Subject, "reviewed_by", reviewedBy)
	return nil
}

// Reject permanently blocks a quarantined message.
func (s *QuarantineService) Reject(ctx context.Context, id, reviewedBy, note string) error {
	msg, err := s.db.GetQuarantinedMessage(ctx, id)
	if err != nil {
		return fmt.Errorf("get quarantined message: %w", err)
	}
	if msg.Status != quarantine.StatusPending {
		return fmt.Errorf("message %s is not pending (status: %s)", id, msg.Status)
	}

	if err := s.db.UpdateQuarantineStatus(ctx, id, quarantine.StatusRejected, reviewedBy, note); err != nil {
		return fmt.Errorf("update quarantine status: %w", err)
	}

	s.hub.BroadcastEvent(ctx, ws.EventQuarantineResolved, map[string]any{
		"id":          id,
		"project_id":  msg.ProjectID,
		"action":      "rejected",
		"reviewed_by": reviewedBy,
	})

	slog.Info("quarantined message rejected",
		"id", id, "subject", msg.Subject, "reviewed_by", reviewedBy)
	return nil
}

// List returns quarantined messages for a project, filtered by status.
func (s *QuarantineService) List(ctx context.Context, projectID string, status quarantine.Status, limit, offset int) ([]*quarantine.Message, error) {
	return s.db.ListQuarantinedMessages(ctx, projectID, status, limit, offset)
}

// Get returns a single quarantined message by ID.
func (s *QuarantineService) Get(ctx context.Context, id string) (*quarantine.Message, error) {
	return s.db.GetQuarantinedMessage(ctx, id)
}
