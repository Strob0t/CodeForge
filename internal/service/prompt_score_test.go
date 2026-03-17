package service

import (
	"context"
	"sync"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/prompt"
)

// inMemoryScoreStore implements PromptScoreStore for testing.
type inMemoryScoreStore struct {
	mu     sync.Mutex
	scores []prompt.PromptScore
}

func (s *inMemoryScoreStore) InsertPromptScore(_ context.Context, score prompt.PromptScore) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scores = append(s.scores, score)
	return nil
}

func (s *inMemoryScoreStore) GetScoresByFingerprint(_ context.Context, fingerprint string) ([]prompt.PromptScore, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []prompt.PromptScore
	for _, sc := range s.scores {
		if sc.PromptFingerprint == fingerprint {
			result = append(result, sc)
		}
	}
	return result, nil
}

func (s *inMemoryScoreStore) GetAggregatedScores(_ context.Context, tenantID, modeID, modelFamily string) (map[string]map[prompt.SignalType]float64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make(map[string]map[prompt.SignalType]float64)
	for _, sc := range s.scores {
		if sc.TenantID == tenantID && sc.ModeID == modeID && sc.ModelFamily == modelFamily {
			if _, ok := result[sc.PromptFingerprint]; !ok {
				result[sc.PromptFingerprint] = make(map[prompt.SignalType]float64)
			}
			// Simple average: this mock just uses the last score.
			result[sc.PromptFingerprint][sc.SignalType] = sc.Score
		}
	}
	return result, nil
}

func newTestScoreCollector() (*PromptScoreCollector, *inMemoryScoreStore) {
	store := &inMemoryScoreStore{}
	return NewPromptScoreCollector(store), store
}

func TestPromptScoreCollector_RecordScore(t *testing.T) {
	t.Parallel()

	t.Run("records valid score", func(t *testing.T) {
		t.Parallel()
		collector, store := newTestScoreCollector()
		ctx := context.Background()

		err := collector.RecordScore(ctx, prompt.PromptScore{
			TenantID:          "t1",
			PromptFingerprint: "fp1",
			ModeID:            "coder",
			ModelFamily:       "openai",
			SignalType:        prompt.SignalBenchmark,
			Score:             0.85,
			RunID:             "run-1",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		store.mu.Lock()
		defer store.mu.Unlock()
		if len(store.scores) != 1 {
			t.Fatalf("expected 1 stored score, got %d", len(store.scores))
		}
		if store.scores[0].Score != 0.85 {
			t.Errorf("stored score = %f, want 0.85", store.scores[0].Score)
		}
	})

	t.Run("skips empty fingerprint", func(t *testing.T) {
		t.Parallel()
		collector, store := newTestScoreCollector()
		ctx := context.Background()

		err := collector.RecordScore(ctx, prompt.PromptScore{
			PromptFingerprint: "",
			SignalType:        prompt.SignalBenchmark,
			Score:             0.5,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		store.mu.Lock()
		defer store.mu.Unlock()
		if len(store.scores) != 0 {
			t.Error("should not store score with empty fingerprint")
		}
	})

	t.Run("skips invalid signal type", func(t *testing.T) {
		t.Parallel()
		collector, store := newTestScoreCollector()
		ctx := context.Background()

		err := collector.RecordScore(ctx, prompt.PromptScore{
			PromptFingerprint: "fp1",
			SignalType:        prompt.SignalType("invalid"),
			Score:             0.5,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		store.mu.Lock()
		defer store.mu.Unlock()
		if len(store.scores) != 0 {
			t.Error("should not store score with invalid signal type")
		}
	})

	t.Run("sets created_at when zero", func(t *testing.T) {
		t.Parallel()
		collector, store := newTestScoreCollector()
		ctx := context.Background()

		err := collector.RecordScore(ctx, prompt.PromptScore{
			PromptFingerprint: "fp1",
			SignalType:        prompt.SignalSuccess,
			Score:             1.0,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		store.mu.Lock()
		defer store.mu.Unlock()
		if store.scores[0].CreatedAt.IsZero() {
			t.Error("created_at should be set when originally zero")
		}
	})
}

func TestPromptScoreCollector_ConvenienceMethods(t *testing.T) {
	t.Parallel()

	t.Run("RecordBenchmarkScore", func(t *testing.T) {
		t.Parallel()
		collector, store := newTestScoreCollector()
		ctx := context.Background()

		err := collector.RecordBenchmarkScore(ctx, "t1", "fp1", "coder", "openai", "run1", 0.92)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		store.mu.Lock()
		defer store.mu.Unlock()
		if len(store.scores) != 1 {
			t.Fatal("expected 1 score")
		}
		if store.scores[0].SignalType != prompt.SignalBenchmark {
			t.Errorf("signal = %q, want benchmark", store.scores[0].SignalType)
		}
		if store.scores[0].Score != 0.92 {
			t.Errorf("score = %f, want 0.92", store.scores[0].Score)
		}
	})

	t.Run("RecordSuccessScore true", func(t *testing.T) {
		t.Parallel()
		collector, store := newTestScoreCollector()
		ctx := context.Background()

		err := collector.RecordSuccessScore(ctx, "t1", "fp1", "coder", "openai", "run1", true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		store.mu.Lock()
		defer store.mu.Unlock()
		if store.scores[0].Score != 1.0 {
			t.Errorf("success=true should score 1.0, got %f", store.scores[0].Score)
		}
	})

	t.Run("RecordSuccessScore false", func(t *testing.T) {
		t.Parallel()
		collector, store := newTestScoreCollector()
		ctx := context.Background()

		err := collector.RecordSuccessScore(ctx, "t1", "fp1", "coder", "openai", "run1", false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		store.mu.Lock()
		defer store.mu.Unlock()
		if store.scores[0].Score != 0.0 {
			t.Errorf("success=false should score 0.0, got %f", store.scores[0].Score)
		}
	})

	t.Run("RecordUserFeedback positive", func(t *testing.T) {
		t.Parallel()
		collector, store := newTestScoreCollector()
		ctx := context.Background()

		err := collector.RecordUserFeedback(ctx, "t1", "fp1", "coder", "openai", "run1", true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		store.mu.Lock()
		defer store.mu.Unlock()
		if store.scores[0].SignalType != prompt.SignalUser {
			t.Errorf("signal = %q, want user", store.scores[0].SignalType)
		}
		if store.scores[0].Score != 1.0 {
			t.Errorf("positive feedback should score 1.0, got %f", store.scores[0].Score)
		}
	})

	t.Run("RecordCostScore clamps to 0-1", func(t *testing.T) {
		t.Parallel()
		collector, store := newTestScoreCollector()
		ctx := context.Background()

		// Over 1.0 should be clamped to 1.0.
		err := collector.RecordCostScore(ctx, "t1", "fp1", "coder", "openai", "run1", 5.0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		store.mu.Lock()
		if store.scores[0].Score != 1.0 {
			t.Errorf("cost score > 1.0 should clamp to 1.0, got %f", store.scores[0].Score)
		}
		store.mu.Unlock()

		// Negative should be clamped to 0.
		err = collector.RecordCostScore(ctx, "t1", "fp2", "coder", "openai", "run2", -0.5)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		store.mu.Lock()
		defer store.mu.Unlock()
		if store.scores[1].Score != 0.0 {
			t.Errorf("negative cost score should clamp to 0.0, got %f", store.scores[1].Score)
		}
	})
}

func TestPromptScoreCollector_CompositeScore(t *testing.T) {
	t.Parallel()

	t.Run("returns false for unknown fingerprint", func(t *testing.T) {
		t.Parallel()
		collector, _ := newTestScoreCollector()

		_, ok := collector.CompositeScoreForFingerprint("unknown")
		if ok {
			t.Error("should return false for unknown fingerprint")
		}
	})

	t.Run("computes weighted average from multiple signals", func(t *testing.T) {
		t.Parallel()
		collector, _ := newTestScoreCollector()
		ctx := context.Background()

		fp := "fp-composite"
		_ = collector.RecordBenchmarkScore(ctx, "t1", fp, "coder", "openai", "r1", 0.9)
		_ = collector.RecordSuccessScore(ctx, "t1", fp, "coder", "openai", "r1", true) // 1.0
		_ = collector.RecordCostScore(ctx, "t1", fp, "coder", "openai", "r1", 0.8)

		score, ok := collector.CompositeScoreForFingerprint(fp)
		if !ok {
			t.Fatal("should return true for known fingerprint")
		}
		// Expected: (0.9*0.35 + 1.0*0.25 + 0.8*0.15) / (0.35+0.25+0.15)
		// = (0.315 + 0.25 + 0.12) / 0.75 = 0.685 / 0.75 = 0.9133
		if score < 0.91 || score > 0.92 {
			t.Errorf("composite score = %f, expected ~0.9133", score)
		}
	})

	t.Run("averages multiple scores per signal type", func(t *testing.T) {
		t.Parallel()
		collector, _ := newTestScoreCollector()
		ctx := context.Background()

		fp := "fp-avg"
		_ = collector.RecordBenchmarkScore(ctx, "t1", fp, "coder", "openai", "r1", 0.8)
		_ = collector.RecordBenchmarkScore(ctx, "t1", fp, "coder", "openai", "r2", 1.0)

		score, ok := collector.CompositeScoreForFingerprint(fp)
		if !ok {
			t.Fatal("should return true")
		}
		// Average benchmark = (0.8+1.0)/2 = 0.9. Only one signal type, so composite = 0.9.
		if score < 0.89 || score > 0.91 {
			t.Errorf("composite score = %f, expected ~0.9", score)
		}
	})
}

func TestPromptScoreCollector_ScoreCount(t *testing.T) {
	t.Parallel()

	t.Run("returns 0 for unknown fingerprint", func(t *testing.T) {
		t.Parallel()
		collector, _ := newTestScoreCollector()
		if n := collector.ScoreCountForFingerprint("unknown"); n != 0 {
			t.Errorf("count = %d, want 0", n)
		}
	})

	t.Run("counts all signals for fingerprint", func(t *testing.T) {
		t.Parallel()
		collector, _ := newTestScoreCollector()
		ctx := context.Background()

		fp := "fp-count"
		_ = collector.RecordBenchmarkScore(ctx, "t1", fp, "coder", "openai", "r1", 0.9)
		_ = collector.RecordSuccessScore(ctx, "t1", fp, "coder", "openai", "r1", true)
		_ = collector.RecordUserFeedback(ctx, "t1", fp, "coder", "openai", "r1", true)

		if n := collector.ScoreCountForFingerprint(fp); n != 3 {
			t.Errorf("count = %d, want 3", n)
		}
	})
}
