package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// EndConversationRunsWithLostWorker ends the conversation runs whose worker
// stopped sending heartbeats for their turn (KI-65): the worker is told to
// stop (it may only have lost its connection), and the run ends as failed
// through the conversation's completion path in the conversation's tenant,
// so the conversation takes its next message and waiters are woken. It
// returns how many lost runs it handled; see LostWorkerAfter.
func (s *ConversationService) EndConversationRunsWithLostWorker(ctx context.Context) (int, error) {
	after := LostWorkerAfter(s.runtimeCfg)
	if after <= 0 {
		return 0, nil
	}
	lost, err := s.db.ListConversationTurnsWithStaleHeartbeat(ctx, after, staleRunBatch)
	if err != nil {
		return 0, fmt.Errorf("list conversation runs with lost worker: %w", err)
	}
	handled := 0
	var errs []error
	for _, turn := range lost {
		convCtx := withEntityTenant(ctx, turn.TenantID)
		slog.WarnContext(convCtx, "conversation run worker heartbeat lost, ending the run",
			"conversation_id", turn.ConversationID, "turn_id", turn.TurnID, "after", after)
		if s.queue != nil {
			logBestEffort(convCtx, s.publishConversationCancel(convCtx, turn.ConversationID), "publishConversationCancel",
				slog.String("conversation_id", turn.ConversationID))
		}
		data, err := json.Marshal(messagequeue.ConversationRunCompletePayload{
			RunID:          turn.ConversationID,
			ConversationID: turn.ConversationID,
			TenantID:       turn.TenantID,
			TurnID:         turn.TurnID,
			Status:         "failed",
			Error:          lostWorkerReason(after),
		})
		if err != nil {
			errs = append(errs, fmt.Errorf("conversation %s: %w", turn.ConversationID, err))
			continue
		}
		if err := s.HandleConversationRunComplete(convCtx, messagequeue.SubjectConversationRunComplete, data); err != nil {
			errs = append(errs, fmt.Errorf("conversation %s: %w", turn.ConversationID, err))
			continue
		}
		handled++
	}
	return handled, errors.Join(errs...)
}
