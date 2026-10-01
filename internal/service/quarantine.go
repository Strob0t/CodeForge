package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain"
	a2adomain "github.com/Strob0t/CodeForge/internal/domain/a2a"
	"github.com/Strob0t/CodeForge/internal/domain/event"
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
	verdict, err := s.Screen(ctx, ann, subject, payload, projectID)
	return verdict != quarantine.VerdictPass, err
}

// Screen checks a message against the quarantine thresholds and tells
// whether it passes, is held for review (stored pending; Approve publishes
// it to subject) or is rejected (stored rejected). It fails closed: a
// message that cannot be checked or stored is rejected, with the error.
func (s *QuarantineService) Screen(ctx context.Context, ann *trust.Annotation, subject string, payload []byte, projectID string) (quarantine.Verdict, error) {
	verdict, _, err := s.ScreenMessage(ctx, ann, subject, payload, projectID)
	return verdict, err
}

// ScreenMessage is Screen that also returns the ID of the message it
// stored (held or rejected; "" when it passed or could not be stored).
func (s *QuarantineService) ScreenMessage(ctx context.Context, ann *trust.Annotation, subject string, payload []byte, projectID string) (quarantine.Verdict, string, error) {
	if !s.cfg.Enabled {
		return quarantine.VerdictPass, "", nil
	}

	// Verify project belongs to caller's tenant (fail-closed).
	if projectID != "" {
		if _, err := s.db.GetProject(ctx, projectID); err != nil {
			slog.Warn("quarantine: project access check failed, blocking message",
				"project_id", projectID, "error", err)
			return quarantine.VerdictRejected, "", fmt.Errorf("quarantine project access check: %w", err)
		}
	}

	// Messages from sufficiently trusted sources bypass quarantine.
	if ann != nil && ann.MeetsMinimum(trust.Level(s.cfg.MinTrustBypass)) {
		return quarantine.VerdictPass, "", nil
	}

	score, factors := quarantine.ScoreMessage(ann, payload)

	// Below quarantine threshold — allow through.
	if score < s.cfg.QuarantineThreshold {
		return quarantine.VerdictPass, "", nil
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
			return quarantine.VerdictRejected, "", fmt.Errorf("quarantine db error: %w", err)
		}
		slog.Warn("message auto-blocked",
			"subject", subject, "score", score, "factors", factors)
		return quarantine.VerdictRejected, msg.ID, nil
	}

	// Between quarantine and block thresholds — hold for review.
	msg.Status = quarantine.StatusPending
	if err := s.db.QuarantineMessage(ctx, msg); err != nil {
		slog.Error("failed to quarantine message, blocking anyway (fail-closed)", "error", err)
		// Fail-closed: block the message even if DB persistence fails.
		return quarantine.VerdictRejected, "", fmt.Errorf("quarantine db error: %w", err)
	}

	// Broadcast alert to admin UI.
	s.hub.BroadcastEvent(ctx, event.EventQuarantineAlert, event.QuarantineAlertEvent{
		ID:        msg.ID,
		ProjectID: projectID,
		Subject:   subject,
		RiskScore: score,
		Factors:   factors,
	})

	slog.Info("message quarantined",
		"id", msg.ID, "subject", subject, "score", score, "factors", factors)
	return quarantine.VerdictHeld, msg.ID, nil
}

// Approve releases a quarantined message, replaying the original payload to
// NATS. The held prompt of an inbound A2A task is replayed only while its
// task waits for it (S2-G fix, 9): a task its caller cancelled (or that is
// gone) gets nothing, and the message is rejected instead (ErrConflict).
// An approved task is working.
func (s *QuarantineService) Approve(ctx context.Context, id, reviewedBy, note string) error {
	msg, err := s.db.GetQuarantinedMessage(ctx, id)
	if err != nil {
		return fmt.Errorf("get quarantined message: %w", err)
	}
	if msg.Status != quarantine.StatusPending {
		return fmt.Errorf("message %s is not pending (status: %s)", id, msg.Status)
	}
	task, waits, err := s.heldA2ATask(ctx, msg)
	if err != nil {
		return err
	}
	if !waits {
		reason := fmt.Sprintf("its A2A task %s no longer waits for it", a2aTaskIDOf(msg))
		if task != nil && task.State != a2adomain.TaskStateSubmitted {
			reason = fmt.Sprintf("its A2A task %s is %s", task.ID, task.State)
		}
		if err := s.db.UpdateQuarantineStatus(ctx, id, quarantine.StatusRejected, reviewedBy, reason); err != nil {
			return fmt.Errorf("update quarantine status: %w", err)
		}
		s.broadcastResolved(ctx, msg, "rejected", reviewedBy)
		return fmt.Errorf("message %s not replayed: %s: %w", id, reason, domain.ErrConflict)
	}

	if err := s.db.UpdateQuarantineStatus(ctx, id, quarantine.StatusApproved, reviewedBy, note); err != nil {
		return fmt.Errorf("update quarantine status: %w", err)
	}

	// Replay original payload byte-for-byte to the original NATS subject.
	if err := s.queue.Publish(ctx, msg.Subject, msg.Payload); err != nil {
		return fmt.Errorf("replay quarantined message: %w", err)
	}
	if task != nil {
		s.resolveHeldA2ATask(ctx, task, a2adomain.TaskStateWorking)
	}

	s.broadcastResolved(ctx, msg, "approved", reviewedBy)

	slog.Info("quarantined message approved and replayed",
		"id", id, "subject", msg.Subject, "reviewed_by", reviewedBy)
	return nil
}

// Reject permanently blocks a quarantined message. The inbound A2A task
// whose prompt it held is rejected with it (S2-G fix, 9).
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
	if task, waits, err := s.heldA2ATask(ctx, msg); err != nil {
		logBestEffort(ctx, err, "reject the held A2A task", slog.String("quarantine_id", id))
	} else if waits && task != nil {
		s.resolveHeldA2ATask(ctx, task, a2adomain.TaskStateRejected)
	}

	s.broadcastResolved(ctx, msg, "rejected", reviewedBy)

	slog.Info("quarantined message rejected",
		"id", id, "subject", msg.Subject, "reviewed_by", reviewedBy)
	return nil
}

// Withdraw rejects a pending message its sender took back (an A2A caller
// cancelled its held task), so it is never replayed: domain.ErrConflict
// when it is no longer pending (an admin resolved it first).
func (s *QuarantineService) Withdraw(ctx context.Context, id, reason string) error {
	if err := s.db.UpdateQuarantineStatus(ctx, id, quarantine.StatusRejected, "sender", reason); err != nil {
		return fmt.Errorf("withdraw quarantined message %s: %w", id, err)
	}
	s.hub.BroadcastEvent(ctx, event.EventQuarantineResolved, event.QuarantineResolvedEvent{
		ID: id, Action: "withdrawn", ReviewedBy: "sender",
	})
	slog.InfoContext(ctx, "quarantined message withdrawn by its sender", "id", id, "reason", reason)
	return nil
}

// broadcastResolved announces how a quarantined message was resolved.
func (s *QuarantineService) broadcastResolved(ctx context.Context, msg *quarantine.Message, action, reviewedBy string) {
	s.hub.BroadcastEvent(ctx, event.EventQuarantineResolved, event.QuarantineResolvedEvent{
		ID:         msg.ID,
		ProjectID:  msg.ProjectID,
		Action:     action,
		ReviewedBy: reviewedBy,
	})
}

// a2aTaskIDOf returns the inbound A2A task whose prompt msg holds ("" for
// a message of another subject, or one that cannot be read).
func a2aTaskIDOf(msg *quarantine.Message) string {
	if msg.Subject != messagequeue.SubjectA2ATaskCreated {
		return ""
	}
	var p messagequeue.A2ATaskCreatedPayload
	if json.Unmarshal(msg.Payload, &p) != nil {
		return ""
	}
	return p.TaskID
}

// heldA2ATask returns the inbound A2A task whose prompt msg holds, and
// whether it still waits for it: submitted, and naming msg as its held
// prompt (a task that could not record it was failed and its message
// withdrawn; S2-G fix 2, 6). A message of another subject has no task and
// "waits" (it is replayed as before); a held prompt whose task is gone does
// not wait.
func (s *QuarantineService) heldA2ATask(ctx context.Context, msg *quarantine.Message) (*a2adomain.A2ATask, bool, error) {
	if msg.Subject != messagequeue.SubjectA2ATaskCreated {
		return nil, true, nil
	}
	taskID := a2aTaskIDOf(msg)
	if taskID == "" {
		return nil, false, nil
	}
	task, err := s.db.GetA2ATask(ctx, taskID)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("get the held a2a task %s: %w", taskID, err)
	}
	waits := task.State == a2adomain.TaskStateSubmitted && task.Metadata[a2adomain.MetadataQuarantineMessageID] == msg.ID
	return task, waits, nil
}

// resolveHeldA2ATask moves a held A2A task to state and announces it; best
// effort (a concurrent change of the task wins).
func (s *QuarantineService) resolveHeldA2ATask(ctx context.Context, task *a2adomain.A2ATask, state a2adomain.TaskState) {
	task.State = state
	if err := s.db.UpdateA2ATask(ctx, task); err != nil {
		logBestEffort(ctx, err, "UpdateA2ATask", slog.String("task_id", task.ID), slog.String("state", string(state)))
		return
	}
	s.hub.BroadcastEvent(ctx, event.EventA2ATaskStatus, map[string]string{
		"task_id":   task.ID,
		"state":     string(state),
		"direction": string(task.Direction),
	})
}

// List returns quarantined messages for a project ("" = the messages
// without project, such as inbound A2A prompts), filtered by status.
func (s *QuarantineService) List(ctx context.Context, projectID string, status quarantine.Status, limit, offset int) ([]*quarantine.Message, error) {
	return s.db.ListQuarantinedMessages(ctx, projectID, status, limit, offset)
}

// Get returns a single quarantined message by ID.
func (s *QuarantineService) Get(ctx context.Context, id string) (*quarantine.Message, error) {
	return s.db.GetQuarantinedMessage(ctx, id)
}
