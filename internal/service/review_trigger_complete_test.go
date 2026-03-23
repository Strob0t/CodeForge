package service

import (
	"context"
	"encoding/json"
	"testing"

	mq "github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

func TestHandleReviewTriggerComplete(t *testing.T) {
	t.Parallel()

	newSvc := func() *ReviewTriggerService {
		return NewReviewTriggerService(nil, nil, 0)
	}

	t.Run("dispatched_status_success", func(t *testing.T) {
		t.Parallel()
		svc := newSvc()
		payload := mq.ReviewTriggerCompletePayload{
			ProjectID: "proj-1",
			TenantID:  "t-1",
			CommitSHA: "abc123",
			Status:    "dispatched",
			RunID:     "run-1",
		}
		data, _ := json.Marshal(payload)

		err := svc.HandleReviewTriggerComplete(context.Background(), "", data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("malformed_json_returns_error", func(t *testing.T) {
		t.Parallel()
		svc := newSvc()

		err := svc.HandleReviewTriggerComplete(context.Background(), "", []byte(`{bad`))
		if err == nil {
			t.Fatal("expected error for malformed JSON")
		}
	})

	t.Run("missing_project_id_returns_error", func(t *testing.T) {
		t.Parallel()
		svc := newSvc()
		payload := mq.ReviewTriggerCompletePayload{
			TenantID:  "t-1",
			CommitSHA: "abc123",
			Status:    "dispatched",
			RunID:     "run-1",
		}
		data, _ := json.Marshal(payload)

		err := svc.HandleReviewTriggerComplete(context.Background(), "", data)
		if err == nil {
			t.Fatal("expected error for missing project_id")
		}
	})

	t.Run("error_status_is_logged", func(t *testing.T) {
		t.Parallel()
		svc := newSvc()
		payload := mq.ReviewTriggerCompletePayload{
			ProjectID: "proj-1",
			TenantID:  "t-1",
			CommitSHA: "abc123",
			Status:    "error",
			RunID:     "run-1",
		}
		data, _ := json.Marshal(payload)

		err := svc.HandleReviewTriggerComplete(context.Background(), "", data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}
