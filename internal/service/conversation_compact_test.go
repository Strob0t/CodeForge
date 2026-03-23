package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	mq "github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

func TestHandleCompactComplete(t *testing.T) {
	t.Parallel()

	newSvc := func() *ConversationService {
		return &ConversationService{
			queue:             &fakeQueue{},
			completionWaiters: make(map[string]chan CompletionResult),
		}
	}

	t.Run("completed_summary_logs_success", func(t *testing.T) {
		t.Parallel()
		svc := newSvc()
		payload := mq.ConversationCompactCompletePayload{
			ConversationID: "conv-1",
			TenantID:       "t-1",
			Summary:        "Summarised conversation about LRU cache design.",
			OriginalCount:  42,
			Status:         "completed",
		}
		data, _ := json.Marshal(payload)

		err := svc.HandleCompactComplete(context.Background(), "", data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("missing_conversation_id_returns_error", func(t *testing.T) {
		t.Parallel()
		svc := newSvc()
		payload := mq.ConversationCompactCompletePayload{
			TenantID: "t-1",
			Summary:  "summary",
			Status:   "completed",
		}
		data, _ := json.Marshal(payload)

		err := svc.HandleCompactComplete(context.Background(), "", data)
		if err == nil {
			t.Fatal("expected error for missing conversation_id")
		}
		if !errors.Is(err, errMissingConversationID) {
			t.Errorf("expected errMissingConversationID, got: %v", err)
		}
	})

	t.Run("malformed_json_returns_error", func(t *testing.T) {
		t.Parallel()
		svc := newSvc()

		err := svc.HandleCompactComplete(context.Background(), "", []byte(`{bad json}`))
		if err == nil {
			t.Fatal("expected error for malformed JSON")
		}
	})

	t.Run("non_completed_status_is_noop", func(t *testing.T) {
		t.Parallel()
		svc := newSvc()
		payload := mq.ConversationCompactCompletePayload{
			ConversationID: "conv-1",
			TenantID:       "t-1",
			Summary:        "partial",
			Status:         "error",
		}
		data, _ := json.Marshal(payload)

		err := svc.HandleCompactComplete(context.Background(), "", data)
		if err != nil {
			t.Fatalf("unexpected error for non-completed status: %v", err)
		}
	})

	t.Run("empty_summary_with_completed_status_succeeds", func(t *testing.T) {
		t.Parallel()
		svc := newSvc()
		payload := mq.ConversationCompactCompletePayload{
			ConversationID: "conv-1",
			TenantID:       "t-1",
			Summary:        "",
			OriginalCount:  0,
			Status:         "completed",
		}
		data, _ := json.Marshal(payload)

		err := svc.HandleCompactComplete(context.Background(), "", data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestStartCompactSubscriber(t *testing.T) {
	t.Parallel()

	t.Run("returns_noop_when_queue_is_nil", func(t *testing.T) {
		t.Parallel()
		svc := &ConversationService{
			completionWaiters: make(map[string]chan CompletionResult),
		}
		cancel, err := svc.StartCompactSubscriber(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		cancel() // should not panic
	})

	t.Run("returns_cancel_when_queue_is_set", func(t *testing.T) {
		t.Parallel()
		svc := &ConversationService{
			queue:             &fakeQueue{},
			completionWaiters: make(map[string]chan CompletionResult),
		}
		cancel, err := svc.StartCompactSubscriber(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		cancel() // should not panic
	})
}
