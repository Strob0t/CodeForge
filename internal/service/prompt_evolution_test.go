package service

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/prompt"
	mq "github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

func ptrTo[T any](v T) *T { return &v }

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

func (s *inMemoryVariantStore) InsertVariant(_ context.Context, v *prompt.PromptVariant) error {
	s.variants = append(s.variants, *v)
	return nil
}

func (s *inMemoryVariantStore) GetVariantByID(_ context.Context, id string) (prompt.PromptVariant, error) {
	for i := range s.variants {
		if s.variants[i].ID == id {
			return s.variants[i], nil
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
		svc := NewPromptEvolutionService(queue, nil, &cfg)

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
		svc := NewPromptEvolutionService(queue, nil, ptrTo(prompt.DefaultEvolutionConfig()))

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
		svc := NewPromptEvolutionService(&fakeQueue{}, store, ptrTo(prompt.DefaultEvolutionConfig()))

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
		svc := NewPromptEvolutionService(&fakeQueue{}, store, ptrTo(prompt.DefaultEvolutionConfig()))

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
		svc := NewPromptEvolutionService(&fakeQueue{}, store, ptrTo(prompt.DefaultEvolutionConfig()))

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
		svc := NewPromptEvolutionService(queue, store, ptrTo(prompt.DefaultEvolutionConfig()))

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
		svc := NewPromptEvolutionService(&fakeQueue{}, store, ptrTo(prompt.DefaultEvolutionConfig()))

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
		svc := NewPromptEvolutionService(queue, store, ptrTo(prompt.DefaultEvolutionConfig()))

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

func TestPromptEvolutionService_HandleReflectComplete(t *testing.T) {
	t.Parallel()

	t.Run("happy_path_with_fixes", func(t *testing.T) {
		t.Parallel()
		queue := &fakeQueue{}
		svc := NewPromptEvolutionService(queue, nil, ptrTo(prompt.DefaultEvolutionConfig()))

		payload := mq.PromptEvolutionReflectCompletePayload{
			TenantID:    "t1",
			ModeID:      "coder",
			ModelFamily: "openai",
			TacticalFixes: []mq.PromptEvolutionTacticalFix{
				{TaskID: "task-1", FailureDescription: "err unchecked", RootCause: "missing pattern", ProposedAddition: "check err", Confidence: 0.9},
			},
			StrategicPrinciples: []string{"always handle errors"},
		}
		data, _ := json.Marshal(payload)

		err := svc.HandleReflectComplete(context.Background(), "", data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("error_payload_is_logged_not_returned", func(t *testing.T) {
		t.Parallel()
		svc := NewPromptEvolutionService(&fakeQueue{}, nil, ptrTo(prompt.DefaultEvolutionConfig()))

		payload := mq.PromptEvolutionReflectCompletePayload{
			TenantID:    "t1",
			ModeID:      "coder",
			ModelFamily: "openai",
			Error:       "LLM call failed",
		}
		data, _ := json.Marshal(payload)

		err := svc.HandleReflectComplete(context.Background(), "", data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("malformed_json_returns_error", func(t *testing.T) {
		t.Parallel()
		svc := NewPromptEvolutionService(&fakeQueue{}, nil, ptrTo(prompt.DefaultEvolutionConfig()))

		err := svc.HandleReflectComplete(context.Background(), "", []byte(`{bad`))
		if err == nil {
			t.Fatal("expected error for malformed JSON")
		}
	})
}

func TestPromptEvolutionService_StartSubscribers(t *testing.T) {
	t.Parallel()

	t.Run("returns_cancels_for_both_subjects", func(t *testing.T) {
		t.Parallel()
		queue := &fakeQueue{}
		svc := NewPromptEvolutionService(queue, nil, ptrTo(prompt.DefaultEvolutionConfig()))

		cancels, err := svc.StartSubscribers(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(cancels) != 2 {
			t.Fatalf("expected 2 cancel functions (reflect + mutate), got %d", len(cancels))
		}
		for _, cancel := range cancels {
			cancel() // should not panic
		}
	})
}

func TestPromptEvolutionService_GetStatus(t *testing.T) {
	t.Parallel()

	t.Run("returns_config_status", func(t *testing.T) {
		t.Parallel()
		cfg := prompt.DefaultEvolutionConfig()
		svc := NewPromptEvolutionService(&fakeQueue{}, &inMemoryVariantStore{}, &cfg)

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

// --- Integration Tests: Full Evolution Loop ---

func TestEvolutionLoop_TriggerToStoreToPromote(t *testing.T) {
	t.Parallel()

	t.Run("full_loop_reflect_mutate_store_promote", func(t *testing.T) {
		t.Parallel()
		queue := &fakeQueue{}
		store := &inMemoryVariantStore{}
		cfg := prompt.DefaultEvolutionConfig()
		svc := NewPromptEvolutionService(queue, store, &cfg)

		// Step 1: TriggerReflection publishes a reflect request to NATS.
		failures := []map[string]json.RawMessage{
			{"task_id": json.RawMessage(`"t1"`), "error": json.RawMessage(`"unchecked error"`)},
		}
		err := svc.TriggerReflection(context.Background(), "tenant-1", "coder", "openai", "base prompt", failures)
		if err != nil {
			t.Fatalf("TriggerReflection: %v", err)
		}
		if len(queue.published) != 1 {
			t.Fatalf("expected 1 NATS publish after TriggerReflection, got %d", len(queue.published))
		}
		if queue.published[0].subject != mq.SubjectPromptEvolutionReflect {
			t.Errorf("expected reflect subject, got %s", queue.published[0].subject)
		}

		// Step 2: Simulate Python worker returning reflect complete (logged, not stored).
		reflectPayload := mq.PromptEvolutionReflectCompletePayload{
			TenantID:    "tenant-1",
			ModeID:      "coder",
			ModelFamily: "openai",
			TacticalFixes: []mq.PromptEvolutionTacticalFix{
				{TaskID: "t1", FailureDescription: "unchecked error", RootCause: "missing check", ProposedAddition: "add error handling", Confidence: 0.95},
			},
			StrategicPrinciples: []string{"always check errors"},
		}
		reflectData, _ := json.Marshal(reflectPayload)
		err = svc.HandleReflectComplete(context.Background(), "", reflectData)
		if err != nil {
			t.Fatalf("HandleReflectComplete: %v", err)
		}

		// Step 3: Simulate Python worker returning mutate complete (stores variant).
		mutatePayload := mq.PromptEvolutionMutateCompletePayload{
			TenantID:         "tenant-1",
			ModeID:           "coder",
			ModelFamily:      "openai",
			VariantContent:   "improved prompt with error handling",
			Version:          2,
			MutationSource:   "tactical",
			ValidationPassed: true,
		}
		mutateData, _ := json.Marshal(mutatePayload)
		err = svc.HandleMutateComplete(context.Background(), mutateData)
		if err != nil {
			t.Fatalf("HandleMutateComplete: %v", err)
		}

		// Step 4: Verify variant was stored as candidate.
		variants, _ := store.GetVariantsByModeAndModel(context.Background(), "coder", "openai")
		if len(variants) != 1 {
			t.Fatalf("expected 1 stored variant, got %d", len(variants))
		}
		if variants[0].PromotionStatus != prompt.PromotionCandidate {
			t.Errorf("variant should be candidate, got %s", variants[0].PromotionStatus)
		}
		if variants[0].Content != "improved prompt with error handling" {
			t.Errorf("variant content mismatch: %q", variants[0].Content)
		}
		if variants[0].Version != 2 {
			t.Errorf("variant version = %d, want 2", variants[0].Version)
		}

		// Step 5: Assign an ID to the stored variant (in real DB this would be auto-generated).
		store.variants[0].ID = "variant-1"

		// Step 6: Promote the variant.
		queue.published = nil // reset published messages
		err = svc.PromoteVariant(context.Background(), "tenant-1", "variant-1")
		if err != nil {
			t.Fatalf("PromoteVariant: %v", err)
		}

		// Step 7: Verify the variant is now promoted.
		v, err := store.GetVariantByID(context.Background(), "variant-1")
		if err != nil {
			t.Fatalf("GetVariantByID: %v", err)
		}
		if v.PromotionStatus != prompt.PromotionPromoted {
			t.Errorf("variant should be promoted, got %s", v.PromotionStatus)
		}

		// Step 8: Verify promoted event was published to NATS.
		if len(queue.published) != 1 {
			t.Fatalf("expected 1 promoted event, got %d", len(queue.published))
		}
		if queue.published[0].subject != mq.SubjectPromptEvolutionPromoted {
			t.Errorf("expected promoted subject, got %s", queue.published[0].subject)
		}
	})

	t.Run("full_loop_with_score_collector", func(t *testing.T) {
		t.Parallel()
		scoreStore := &inMemoryScoreStore{}
		collector := NewPromptScoreCollector(scoreStore)
		ctx := context.Background()

		fingerprint := "fp-evolved-prompt"

		// Record multiple score signals.
		err := collector.RecordBenchmarkScore(ctx, "t1", fingerprint, "coder", "openai", "r1", 0.88)
		if err != nil {
			t.Fatalf("RecordBenchmarkScore: %v", err)
		}
		err = collector.RecordSuccessScore(ctx, "t1", fingerprint, "coder", "openai", "r1", true)
		if err != nil {
			t.Fatalf("RecordSuccessScore: %v", err)
		}
		err = collector.RecordCostScore(ctx, "t1", fingerprint, "coder", "openai", "r1", 0.72)
		if err != nil {
			t.Fatalf("RecordCostScore: %v", err)
		}
		err = collector.RecordUserFeedback(ctx, "t1", fingerprint, "coder", "openai", "r1", true)
		if err != nil {
			t.Fatalf("RecordUserFeedback: %v", err)
		}
		err = collector.RecordEfficiencyScore(ctx, "t1", fingerprint, "coder", "openai", "r1", 0.65)
		if err != nil {
			t.Fatalf("RecordEfficiencyScore: %v", err)
		}

		// Verify composite score is computed.
		composite, ok := collector.CompositeScoreForFingerprint(fingerprint)
		if !ok {
			t.Fatal("expected composite score to exist")
		}
		// All 5 signals present:
		// benchmark=0.88*0.35 + success=1.0*0.25 + cost=0.72*0.15 + user=1.0*0.15 + efficiency=0.65*0.10
		// = 0.308 + 0.25 + 0.108 + 0.15 + 0.065 = 0.881
		// Divided by total weight: 0.35+0.25+0.15+0.15+0.10 = 1.0
		// = 0.881
		if composite < 0.87 || composite > 0.89 {
			t.Errorf("composite score = %f, expected ~0.881", composite)
		}

		// Verify score count.
		count := collector.ScoreCountForFingerprint(fingerprint)
		if count != 5 {
			t.Errorf("score count = %d, want 5", count)
		}

		// Verify store persisted all scores.
		scoreStore.mu.Lock()
		storedCount := len(scoreStore.scores)
		scoreStore.mu.Unlock()
		if storedCount != 5 {
			t.Errorf("stored scores = %d, want 5", storedCount)
		}
	})

	t.Run("mutate_then_promote_retires_previous", func(t *testing.T) {
		t.Parallel()
		queue := &fakeQueue{}
		store := &inMemoryVariantStore{}
		cfg := prompt.DefaultEvolutionConfig()
		svc := NewPromptEvolutionService(queue, store, &cfg)

		// Store two mutations (simulating two evolution rounds).
		for i, content := range []string{"variant-v2", "variant-v3"} {
			payload := mq.PromptEvolutionMutateCompletePayload{
				TenantID:         "t1",
				ModeID:           "coder",
				ModelFamily:      "openai",
				VariantContent:   content,
				Version:          i + 2,
				MutationSource:   "tactical",
				ValidationPassed: true,
			}
			data, _ := json.Marshal(payload)
			if err := svc.HandleMutateComplete(context.Background(), data); err != nil {
				t.Fatalf("HandleMutateComplete round %d: %v", i, err)
			}
		}

		// Assign IDs (simulating DB auto-generation).
		store.variants[0].ID = "v2"
		store.variants[1].ID = "v3"

		// Promote v2.
		if err := svc.PromoteVariant(context.Background(), "t1", "v2"); err != nil {
			t.Fatalf("PromoteVariant v2: %v", err)
		}

		// Verify v2 is promoted.
		v2, _ := store.GetVariantByID(context.Background(), "v2")
		if v2.PromotionStatus != prompt.PromotionPromoted {
			t.Errorf("v2 should be promoted, got %s", v2.PromotionStatus)
		}

		// Now promote v3 (should retire v2).
		if err := svc.PromoteVariant(context.Background(), "t1", "v3"); err != nil {
			t.Fatalf("PromoteVariant v3: %v", err)
		}

		// Verify v2 is now retired, v3 is promoted.
		v2, _ = store.GetVariantByID(context.Background(), "v2")
		if v2.PromotionStatus != prompt.PromotionRetired {
			t.Errorf("v2 should be retired after promoting v3, got %s", v2.PromotionStatus)
		}
		v3, _ := store.GetVariantByID(context.Background(), "v3")
		if v3.PromotionStatus != prompt.PromotionPromoted {
			t.Errorf("v3 should be promoted, got %s", v3.PromotionStatus)
		}
	})

	t.Run("selector_picks_promoted_variant", func(t *testing.T) {
		t.Parallel()
		store := &inMemoryVariantStore{
			variants: []prompt.PromptVariant{
				{
					ID:              "base",
					ModeID:          "coder",
					ModelFamily:     "openai",
					Content:         "base prompt",
					PromotionStatus: prompt.PromotionRetired,
					Enabled:         true,
					Version:         1,
				},
				{
					ID:              "evolved",
					ModeID:          "coder",
					ModelFamily:     "openai",
					Content:         "evolved prompt with improvements",
					PromotionStatus: prompt.PromotionPromoted,
					Enabled:         true,
					Version:         2,
					AvgScore:        0.9,
					TrialCount:      30,
				},
			},
		}
		cfg := prompt.DefaultEvolutionConfig()
		cfg.PromotionStrategy = prompt.StrategyManual

		selector := NewPromptSelector(store, &cfg)
		content, ok := selector.SelectVariant("coder", "openai")
		if !ok {
			t.Fatal("expected selector to return a variant")
		}
		if content != "evolved prompt with improvements" {
			t.Errorf("selector returned %q, expected evolved prompt", content)
		}
	})
}
