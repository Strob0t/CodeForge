package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// Runs whose start was dead-lettered (KI-76). A worker moves a start it
// rejects (invalid payload) or cannot accept within its deliveries to
// "{subject}.dlq". No worker executes such a run and it sends no heartbeat,
// so neither its completion nor the stuck-work watchdog would end it. The Go
// Core reads the dead-lettered starts (besides the DLQ monitor, which only
// logs them) and ends these runs as failed through their completion paths.

// deadLetterSuffix is appended to a subject for its dead-letter copy
// (adapter/nats and workers/codeforge/nats_subjects.py).
const deadLetterSuffix = ".dlq"

// deadLetteredStartError is the error of a run whose start was dead-lettered.
const deadLetteredStartError = "the run's start could not be delivered to a worker (dead-lettered)"

// HandleDeadLetteredRunStart ends the conversation run whose start was
// dead-lettered, if it is still the conversation's active run, as failed
// through the completion path: the conversation takes its next message.
// A start that cannot be read, or of a run that ended, is ignored.
func (s *ConversationService) HandleDeadLetteredRunStart(ctx context.Context, _ string, data []byte) error {
	var start messagequeue.ConversationRunStartPayload
	if err := json.Unmarshal(data, &start); err != nil || start.ConversationID == "" {
		slog.Warn("dead-lettered conversation run start without a conversation, ignored", "error", err)
		return nil
	}
	if s.runTracker == nil || !s.runTracker.IsActiveConversationRun(start.ConversationID, start.TurnID) {
		slog.Info("dead-lettered start of a conversation run that is not active, ignored",
			"conversation_id", start.ConversationID, "turn_id", start.TurnID)
		return nil
	}
	slog.Warn("conversation run start dead-lettered, ending the run",
		"conversation_id", start.ConversationID, "turn_id", start.TurnID)
	completion, err := json.Marshal(messagequeue.ConversationRunCompletePayload{
		RunID:          start.ConversationID,
		ConversationID: start.ConversationID,
		TenantID:       start.TenantID,
		TurnID:         start.TurnID,
		Status:         "failed",
		Error:          deadLetteredStartError,
	})
	if err != nil {
		return fmt.Errorf("marshal completion of dead-lettered start: %w", err)
	}
	return s.HandleConversationRunComplete(ctx, messagequeue.SubjectConversationRunComplete, completion)
}

// StartDeadLetterSubscriber subscribes to dead-lettered conversation run starts.
func (s *ConversationService) StartDeadLetterSubscriber(ctx context.Context) (func(), error) {
	if s.queue == nil {
		return func() {}, nil
	}
	return s.queue.Subscribe(ctx, messagequeue.SubjectConversationRunStart+deadLetterSuffix, s.HandleDeadLetteredRunStart)
}

// HandleDeadLetteredRunStart fails a run whose start was dead-lettered while
// it waits for its worker, in the run's tenant, through the run completion
// path. A start that cannot be read, of an unknown run or of a run that
// ended, is ignored.
func (s *RuntimeService) HandleDeadLetteredRunStart(ctx context.Context, data []byte) error {
	var start messagequeue.RunStartPayload
	if err := json.Unmarshal(data, &start); err != nil {
		slog.Warn("unreadable dead-lettered run start, ignored", "error", err)
		return nil
	}
	if _, err := uuid.Parse(start.RunID); err != nil {
		// Every run start names a run of the store (handoff runs too, KI-15);
		// a start that does not was not made by the Go Core.
		slog.Info("dead-lettered start of a run the Go Core does not know, ignored", "run_id", start.RunID)
		return nil
	}
	ctx, r, err := s.loadRunScoped(ctx, start.RunID, start.TenantID)
	if err != nil {
		slog.Info("dead-lettered start of a run that is not stored, ignored", "run_id", start.RunID, "error", err)
		return nil
	}
	if r.Status != run.StatusRunning && r.Status != run.StatusPending {
		return nil
	}
	slog.WarnContext(ctx, "run start dead-lettered, failing the run", "run_id", r.ID)
	return s.HandleRunComplete(ctx, &messagequeue.RunCompletePayload{
		RunID:    r.ID,
		TaskID:   r.TaskID,
		TenantID: r.TenantID,
		Status:   string(run.StatusFailed),
		Error:    deadLetteredStartError,
	})
}

// handleDeadLetteredRunStart is the NATS handler of dead-lettered run starts.
func (s *RuntimeService) handleDeadLetteredRunStart(ctx context.Context, data []byte) error {
	return s.HandleDeadLetteredRunStart(ctx, data)
}
