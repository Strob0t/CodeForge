package service

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/prompt"
	mq "github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// fakeQueue records published messages.
type fakeQueue struct {
	published []fakeMsg
}

type fakeMsg struct {
	subject string
	data    []byte
	msgID   string
}

func (q *fakeQueue) Publish(_ context.Context, subject string, data []byte) error {
	q.published = append(q.published, fakeMsg{subject: subject, data: data})
	return nil
}

func (q *fakeQueue) PublishWithDedup(_ context.Context, subject string, data []byte, msgID string) error {
	q.published = append(q.published, fakeMsg{subject: subject, data: data, msgID: msgID})
	return nil
}

func (q *fakeQueue) Subscribe(_ context.Context, _ string, _ mq.Handler) (func(), error) {
	return func() {}, nil
}

func (q *fakeQueue) Drain() error      { return nil }
func (q *fakeQueue) Close() error      { return nil }
func (q *fakeQueue) IsConnected() bool { return true }

// Extend inMemoryVariantStore (from prompt_selector_test.go) with PromptEvolutionStore methods.

func (s *inMemoryVariantStore) InsertVariant(_ context.Context, v prompt.PromptVariant) error {
	s.variants = append(s.variants, v)
	return nil
}

func (s *inMemoryVariantStore) GetVariantByID(_ context.Context, id string) (prompt.PromptVariant, error) {
	for _, v := range s.variants {
		if v.ID == id {
			return v, nil
		}
	}
	return prompt.PromptVariant{}, fmt.Errorf("variant %s not found", id)
}

func (s *inMemoryVariantStore) UpdatePromotionStatus(_ context.Context, id string, status prompt.PromotionStatus) error {
	for i := range s.variants {
		if s.variants[i].ID == id {
			s.variants[i].PromotionStatus = status
			return nil
		}
	}
	return fmt.Errorf("variant %s not found", id)
}

func TestPromptEvolutionService_TriggerReflection(t *testing.T) {
	t.Parallel()

	t.Run("publishes_reflect_request_to_NATS", func(t *testing.T) {
		t.Parallel()
		queue := &fakeQueue{}
		cfg := prompt.DefaultEvolutionConfig()
		svc := NewPromptEvolutionService(queue, nil, cfg)

		failures := []map[string]json.RawMessage{
			{"task_id": json.RawMessage(`"t1"`), "error": json.RawMessage(`"some error"`)},
		}

		err := svc.TriggerReflection(context.Background(), "tenant-1", "coder", "openai", "current prompt text", failures)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(queue.published) != 1 {
			t.Fatalf("expected 1 published message, got %d", len(queue.published))
		}

		msg := queue.published[0]
		if msg.subject != mq.SubjectPromptEvolutionReflect {
			t.Errorf("expected subject %s, got %s", mq.SubjectPromptEvolutionReflect, msg.subject)
		}

		var payload mq.PromptEvolutionReflectPayload
		if err := json.Unmarshal(msg.data, &payload); err != nil {
			t.Fatalf("failed to unmarshal payload: %v", err)
		}
		if payload.TenantID != "tenant-1" {
			t.Errorf("tenant_id = %q, want %q", payload.TenantID, "tenant-1")
		}
		if payload.ModeID != "coder" {
			t.Errorf("mode_id = %q, want %q", payload.ModeID, "coder")
		}
		if payload.ModelFamily != "openai" {
			t.Errorf("model_family = %q, want %q", payload.ModelFamily, "openai")
		}
		if payload.CurrentPrompt != "current prompt text" {
			t.Error("current_prompt mismatch")
		}
		if len(payload.Failures) != 1 {
			t.Errorf("expected 1 failure, got %d", len(payload.Failures))
		}
	})

	t.Run("empty_failures_is_allowed", func(t *testing.T) {
		t.Parallel()
		queue := &fakeQueue{}
		svc := NewPromptEvolutionService(queue, nil, prompt.DefaultEvolutionConfig())

		err := svc.TriggerReflection(context.Background(), "t1", "coder", "openai", "prompt", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(queue.published) != 1 {
			t.Fatalf("expected 1 published message, got %d", len(queue.published))
		}
	})
}

func TestPromptEvolutionService_HandleMutateComplete(t *testing.T) {
	t.Parallel()

	t.Run("stores_variant_in_store", func(t *testing.T) {
		t.Parallel()
		store := &inMemoryVariantStore{}
		svc := NewPromptEvolutionService(&fakeQueue{}, store, prompt.DefaultEvolutionConfig())

		payload := mq.PromptEvolutionMutateCompletePayload{
			TenantID:         "t1",
			ModeID:           "coder",
			ModelFamily:      "openai",
			VariantContent:   "improved prompt",
			Version:          2,
			MutationSource:   "tactical",
			ValidationPassed: true,
		}
		data, _ := json.Marshal(payload)

		err := svc.HandleMutateComplete(context.Background(), data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		variants, _ := store.GetVariantsByModeAndModel(context.Background(), "coder", "openai")
		if len(variants) != 1 {
			t.Fatalf("expected 1 stored variant, got %d", len(variants))
		}

		v := variants[0]
		if v.Content != "improved prompt" {
			t.Errorf("content = %q, want %q", v.Content, "improved prompt")
		}
		if v.PromotionStatus != prompt.PromotionCandidate {
			t.Errorf("promotion_status = %q, want %q", v.PromotionStatus, prompt.PromotionCandidate)
		}
		if v.MutationSource != "tactical" {
			t.Errorf("mutation_source = %q, want %q", v.MutationSource, "tactical")
		}
	})

	t.Run("skips_variant_that_failed_validation", func(t *testing.T) {
		t.Parallel()
		store := &inMemoryVariantStore{}
		svc := NewPromptEvolutionService(&fakeQueue{}, store, prompt.DefaultEvolutionConfig())

		payload := mq.PromptEvolutionMutateCompletePayload{
			TenantID:         "t1",
			ModeID:           "coder",
			ModelFamily:      "openai",
			VariantContent:   "bad",
			ValidationPassed: false,
		}
		data, _ := json.Marshal(payload)

		err := svc.HandleMutateComplete(context.Background(), data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		variants, _ := store.GetVariantsByModeAndModel(context.Background(), "coder", "openai")
		if len(variants) != 0 {
			t.Errorf("expected 0 stored variants (validation failed), got %d", len(variants))
		}
	})

	t.Run("error_payload_is_logged_not_stored", func(t *testing.T) {
		t.Parallel()
		store := &inMemoryVariantStore{}
		svc := NewPromptEvolutionService(&fakeQueue{}, store, prompt.DefaultEvolutionConfig())

		payload := mq.PromptEvolutionMutateCompletePayload{
			TenantID: "t1",
			ModeID:   "coder",
			Error:    "reflection failed",
		}
		data, _ := json.Marshal(payload)

		err := svc.HandleMutateComplete(context.Background(), data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		variants, _ := store.GetVariantsByModeAndModel(context.Background(), "coder", "openai")
		if len(variants) != 0 {
			t.Errorf("expected 0 stored variants on error, got %d", len(variants))
		}
	})
}

func TestPromptEvolutionService_PromoteVariant(t *testing.T) {
	t.Parallel()

	t.Run("promotes_candidate_and_retires_old_promoted", func(t *testing.T) {
		t.Parallel()
		queue := &fakeQueue{}
		store := &inMemoryVariantStore{
			variants: []prompt.PromptVariant{
				{
					ID:              "old-promoted",
					ModeID:          "coder",
					ModelFamily:     "openai",
					Content:         "old content",
					PromotionStatus: prompt.PromotionPromoted,
					Enabled:         true,
					Version:         1,
				},
				{
					ID:              "new-candidate",
					ModeID:          "coder",
					ModelFamily:     "openai",
					Content:         "new content",
					PromotionStatus: prompt.PromotionCandidate,
					Enabled:         true,
					Version:         2,
					AvgScore:        0.9,
					TrialCount:      50,
				},
			},
		}
		svc := NewPromptEvolutionService(queue, store, prompt.DefaultEvolutionConfig())

		err := svc.PromoteVariant(context.Background(), "t1", "new-candidate")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify the new candidate is now promoted.
		for _, v := range store.variants {
			if v.ID == "new-candidate" && v.PromotionStatus != prompt.PromotionPromoted {
				t.Errorf("new-candidate should be promoted, got %s", v.PromotionStatus)
			}
			if v.ID == "old-promoted" && v.PromotionStatus != prompt.PromotionRetired {
				t.Errorf("old-promoted should be retired, got %s", v.PromotionStatus)
			}
		}

		// Verify event published.
		if len(queue.published) != 1 {
			t.Fatalf("expected 1 published event, got %d", len(queue.published))
		}
		if queue.published[0].subject != mq.SubjectPromptEvolutionPromoted {
			t.Errorf("expected promoted event subject, got %s", queue.published[0].subject)
		}
	})

	t.Run("promote_nonexistent_returns_error", func(t *testing.T) {
		t.Parallel()
		store := &inMemoryVariantStore{}
		svc := NewPromptEvolutionService(&fakeQueue{}, store, prompt.DefaultEvolutionConfig())

		err := svc.PromoteVariant(context.Background(), "t1", "nonexistent")
		if err == nil {
			t.Error("expected error for nonexistent variant")
		}
	})
}

func TestPromptEvolutionService_RevertMode(t *testing.T) {
	t.Parallel()

	t.Run("retires_all_variants_for_mode", func(t *testing.T) {
		t.Parallel()
		queue := &fakeQueue{}
		store := &inMemoryVariantStore{
			variants: []prompt.PromptVariant{
				{
					ID:              "v1",
					ModeID:          "coder",
					ModelFamily:     "openai",
					PromotionStatus: prompt.PromotionPromoted,
					Enabled:         true,
				},
				{
					ID:              "v2",
					ModeID:          "coder",
					ModelFamily:     "openai",
					PromotionStatus: prompt.PromotionCandidate,
					Enabled:         true,
				},
				{
					ID:              "v3",
					ModeID:          "reviewer",
					ModelFamily:     "openai",
					PromotionStatus: prompt.PromotionPromoted,
					Enabled:         true,
				},
			},
		}
		svc := NewPromptEvolutionService(queue, store, prompt.DefaultEvolutionConfig())

		err := svc.RevertMode(context.Background(), "t1", "coder")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		for _, v := range store.variants {
			if v.ModeID == "coder" && v.PromotionStatus != prompt.PromotionRetired {
				t.Errorf("variant %s for coder should be retired, got %s", v.ID, v.PromotionStatus)
			}
			if v.ModeID == "reviewer" && v.PromotionStatus != prompt.PromotionPromoted {
				t.Errorf("variant %s for reviewer should be unaffected, got %s", v.ID, v.PromotionStatus)
			}
		}

		// Verify revert event published.
		if len(queue.published) != 1 {
			t.Fatalf("expected 1 published event, got %d", len(queue.published))
		}
		if queue.published[0].subject != mq.SubjectPromptEvolutionReverted {
			t.Errorf("expected reverted event subject, got %s", queue.published[0].subject)
		}
	})
}

func TestPromptEvolutionService_GetStatus(t *testing.T) {
	t.Parallel()

	t.Run("returns_config_status", func(t *testing.T) {
		t.Parallel()
		cfg := prompt.DefaultEvolutionConfig()
		svc := NewPromptEvolutionService(&fakeQueue{}, &inMemoryVariantStore{}, cfg)

		status := svc.GetStatus()
		if !status.Enabled {
			t.Error("expected enabled=true")
		}
		if status.Trigger != prompt.TriggerBenchmark {
			t.Errorf("trigger = %q, want %q", status.Trigger, prompt.TriggerBenchmark)
		}
		if status.Strategy != prompt.StrategyAuto {
			t.Errorf("strategy = %q, want %q", status.Strategy, prompt.StrategyAuto)
		}
	})
}
