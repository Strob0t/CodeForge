package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/Strob0t/CodeForge/internal/domain/conversation"
	"github.com/Strob0t/CodeForge/internal/port/broadcast"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// ConversationMessageService handles message listing, search, compaction, and clearing.
type ConversationMessageService struct {
	store database.Store
	queue messagequeue.Queue
	hub   broadcast.Broadcaster
}

// NewConversationMessageService creates a message service.
func NewConversationMessageService(store database.Store, queue messagequeue.Queue, hub broadcast.Broadcaster) *ConversationMessageService {
	return &ConversationMessageService{store: store, queue: queue, hub: hub}
}

// ListMessages returns all messages in a conversation.
func (s *ConversationMessageService) ListMessages(ctx context.Context, conversationID string) ([]conversation.Message, error) {
	return s.store.ListMessages(ctx, conversationID)
}

// SearchMessages performs full-text search across conversation messages.
func (s *ConversationMessageService) SearchMessages(ctx context.Context, query string, projectIDs []string, limit int) ([]conversation.Message, error) {
	return s.store.SearchConversationMessages(ctx, query, projectIDs, limit)
}

// ClearConversation deletes all messages from a conversation.
func (s *ConversationMessageService) ClearConversation(ctx context.Context, conversationID string) error {
	_, err := s.store.GetConversation(ctx, conversationID)
	if err != nil {
		return err
	}
	return s.store.DeleteConversationMessages(ctx, conversationID)
}

// CompactConversation publishes a compaction request to the Python worker via NATS.
// The worker will summarise the conversation history to reduce token usage.
func (s *ConversationMessageService) CompactConversation(ctx context.Context, conversationID string) error {
	_, err := s.store.GetConversation(ctx, conversationID)
	if err != nil {
		return err
	}
	if s.queue == nil {
		return errors.New("message queue not configured")
	}
	payload := map[string]string{
		"conversation_id": conversationID,
		"tenant_id":       tenantctx.FromContext(ctx),
	}
	data, _ := json.Marshal(payload)
	return s.queue.Publish(ctx, messagequeue.SubjectConversationCompactRequest, data)
}

// HandleCompactComplete processes a conversation.compact.complete message from the Python worker.
// It logs the outcome and silently drops non-completed statuses.
func (s *ConversationMessageService) HandleCompactComplete(_ context.Context, _ string, data []byte) error {
	var p messagequeue.ConversationCompactCompletePayload
	if err := json.Unmarshal(data, &p); err != nil {
		return fmt.Errorf("unmarshal compact complete: %w", err)
	}
	if p.ConversationID == "" {
		return errMissingConversationID
	}
	if p.Status != "completed" {
		slog.Warn("compact not completed", "conversation_id", p.ConversationID, "status", p.Status)
		return nil
	}
	slog.Info("compact complete",
		"conversation_id", p.ConversationID,
		"original_count", p.OriginalCount,
		"summary_len", len(p.Summary),
	)
	return nil
}

// StartCompactSubscriber subscribes to conversation.compact.complete and returns a cancel function.
func (s *ConversationMessageService) StartCompactSubscriber(ctx context.Context) (func(), error) {
	if s.queue == nil {
		return func() {}, nil
	}
	return s.queue.Subscribe(ctx, messagequeue.SubjectConversationCompactComplete, s.HandleCompactComplete)
}
