package service

import (
	"context"
	"math/rand/v2"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/prompt"
)

// inMemoryVariantStore implements PromptVariantStore for testing.
type inMemoryVariantStore struct {
	variants []prompt.PromptVariant
}

func (s *inMemoryVariantStore) GetVariantsByModeAndModel(_ context.Context, modeID, modelFamily string) ([]prompt.PromptVariant, error) {
	var result []prompt.PromptVariant
	for i := range s.variants {
		if s.variants[i].ModeID == modeID && s.variants[i].ModelFamily == modelFamily {
			result = append(result, s.variants[i])
		}
	}
	return result, nil
}

func (s *inMemoryVariantStore) ListVariants(_ context.Context, modeID, status string) ([]prompt.PromptVariant, error) {
	var result []prompt.PromptVariant
	for i := range s.variants {
		if modeID != "" && s.variants[i].ModeID != modeID {
			continue
		}
		if status != "" && string(s.variants[i].PromotionStatus) != status {
			continue
		}
		if s.variants[i].ModeID != "" {
			result = append(result, s.variants[i])
		}
	}
	return result, nil
}

func (s *inMemoryVariantStore) UpdateVariantStats(_ context.Context, id string, trialCount int, avgScore float64) error {
	for i := range s.variants {
		if s.variants[i].ID == id {
			s.variants[i].TrialCount = trialCount
			s.variants[i].AvgScore = avgScore
		}
	}
	return nil
}

func TestPromptSelector_SelectVariant(t *testing.T) {
	t.Parallel()

	t.Run("nil_store_returns_not_ok", func(t *testing.T) {
		t.Parallel()
		sel := NewPromptSelector(nil, ptrTo(prompt.DefaultEvolutionConfig()))
		_, ok := sel.SelectVariant("some-entry", "openai")
		if ok {
			t.Error("expected ok=false with nil store")
		}
	})

	t.Run("no_variants_returns_not_ok", func(t *testing.T) {
		t.Parallel()
		store := &inMemoryVariantStore{}
		sel := NewPromptSelector(store, ptrTo(prompt.DefaultEvolutionConfig()))
		_, ok := sel.SelectVariant("coder", "openai")
		if ok {
			t.Error("expected ok=false with no variants")
		}
	})

	t.Run("returns_promoted_variant", func(t *testing.T) {
		t.Parallel()
		store := &inMemoryVariantStore{
			variants: []prompt.PromptVariant{
				{
					ID:              "v1",
					ModeID:          "coder",
					ModelFamily:     "openai",
					Content:         "promoted content",
					PromotionStatus: prompt.PromotionPromoted,
					Enabled:         true,
				},
				{
					ID:              "v2",
					ModeID:          "coder",
					ModelFamily:     "openai",
					Content:         "candidate content",
					PromotionStatus: prompt.PromotionCandidate,
					Enabled:         true,
				},
			},
		}
		cfg := prompt.DefaultEvolutionConfig()
		cfg.PromotionStrategy = prompt.StrategyManual // only return explicitly promoted
		sel := NewPromptSelector(store, &cfg)
		content, ok := sel.SelectVariant("coder", "openai")
		if !ok {
			t.Fatal("expected ok=true")
		}
		if content != "promoted content" {
			t.Errorf("expected promoted content, got %q", content)
		}
	})

	t.Run("retired_variants_ignored", func(t *testing.T) {
		t.Parallel()
		store := &inMemoryVariantStore{
			variants: []prompt.PromptVariant{
				{
					ID:              "v1",
					ModeID:          "coder",
					ModelFamily:     "openai",
					Content:         "retired content",
					PromotionStatus: prompt.PromotionRetired,
					Enabled:         true,
				},
			},
		}
		sel := NewPromptSelector(store, ptrTo(prompt.DefaultEvolutionConfig()))
		_, ok := sel.SelectVariant("coder", "openai")
		if ok {
			t.Error("expected ok=false for retired-only variants")
		}
	})

	t.Run("disabled_variants_ignored", func(t *testing.T) {
		t.Parallel()
		store := &inMemoryVariantStore{
			variants: []prompt.PromptVariant{
				{
					ID:              "v1",
					ModeID:          "coder",
					ModelFamily:     "openai",
					Content:         "disabled promoted",
					PromotionStatus: prompt.PromotionPromoted,
					Enabled:         false,
				},
			},
		}
		sel := NewPromptSelector(store, ptrTo(prompt.DefaultEvolutionConfig()))
		_, ok := sel.SelectVariant("coder", "openai")
		if ok {
			t.Error("expected ok=false for disabled variants")
		}
	})

	t.Run("manual_strategy_only_returns_promoted", func(t *testing.T) {
		t.Parallel()
		cfg := prompt.DefaultEvolutionConfig()
		cfg.PromotionStrategy = prompt.StrategyManual
		store := &inMemoryVariantStore{
			variants: []prompt.PromptVariant{
				{
					ID:              "v1",
					ModeID:          "coder",
					ModelFamily:     "openai",
					Content:         "candidate only",
					PromotionStatus: prompt.PromotionCandidate,
					Enabled:         true,
				},
			},
		}
		sel := NewPromptSelector(store, &cfg)
		_, ok := sel.SelectVariant("coder", "openai")
		if ok {
			t.Error("manual strategy should not return candidates")
		}
	})

	t.Run("shadow_strategy_returns_promoted", func(t *testing.T) {
		t.Parallel()
		cfg := prompt.DefaultEvolutionConfig()
		cfg.PromotionStrategy = prompt.StrategyShadow
		store := &inMemoryVariantStore{
			variants: []prompt.PromptVariant{
				{
					ID:              "v1",
					ModeID:          "coder",
					ModelFamily:     "openai",
					Content:         "promoted shadow",
					PromotionStatus: prompt.PromotionPromoted,
					Enabled:         true,
				},
				{
					ID:              "v2",
					ModeID:          "coder",
					ModelFamily:     "openai",
					Content:         "candidate shadow",
					PromotionStatus: prompt.PromotionCandidate,
					Enabled:         true,
				},
			},
		}
		sel := NewPromptSelector(store, &cfg)
		content, ok := sel.SelectVariant("coder", "openai")
		if !ok {
			t.Fatal("expected ok=true for shadow with promoted variant")
		}
		if content != "promoted shadow" {
			t.Errorf("shadow strategy should return promoted, got %q", content)
		}
	})

	t.Run("auto_strategy_explores_candidates", func(t *testing.T) {
		t.Parallel()
		cfg := prompt.DefaultEvolutionConfig()
		cfg.PromotionStrategy = prompt.StrategyAuto
		store := &inMemoryVariantStore{
			variants: []prompt.PromptVariant{
				{
					ID:              "v1",
					ModeID:          "coder",
					ModelFamily:     "openai",
					Content:         "promoted auto",
					PromotionStatus: prompt.PromotionPromoted,
					Enabled:         true,
					TrialCount:      100,
					AvgScore:        0.8,
				},
				{
					ID:              "v2",
					ModeID:          "coder",
					ModelFamily:     "openai",
					Content:         "candidate auto",
					PromotionStatus: prompt.PromotionCandidate,
					Enabled:         true,
					TrialCount:      5,
					AvgScore:        0.7,
				},
			},
		}

		// Run many iterations to verify exploration happens sometimes.
		sel := NewPromptSelector(store, &cfg)
		sel.rng = rand.New(rand.NewPCG(42, 0)) // deterministic

		promotedCount := 0
		candidateCount := 0
		for range 100 {
			content, ok := sel.SelectVariant("coder", "openai")
			if !ok {
				t.Fatal("expected ok=true")
			}
			if content == "promoted auto" {
				promotedCount++
			} else {
				candidateCount++
			}
		}

		// With 80/20 split, we expect roughly 80 promoted and 20 candidate.
		if promotedCount < 50 || candidateCount < 5 {
			t.Errorf("exploration ratio unexpected: promoted=%d candidate=%d", promotedCount, candidateCount)
		}
	})

	t.Run("auto_strategy_no_promoted_explores_candidates", func(t *testing.T) {
		t.Parallel()
		cfg := prompt.DefaultEvolutionConfig()
		cfg.PromotionStrategy = prompt.StrategyAuto
		store := &inMemoryVariantStore{
			variants: []prompt.PromptVariant{
				{
					ID:              "v1",
					ModeID:          "coder",
					ModelFamily:     "openai",
					Content:         "candidate 1",
					PromotionStatus: prompt.PromotionCandidate,
					Enabled:         true,
					TrialCount:      2,
					AvgScore:        0.6,
				},
				{
					ID:              "v2",
					ModeID:          "coder",
					ModelFamily:     "openai",
					Content:         "candidate 2",
					PromotionStatus: prompt.PromotionCandidate,
					Enabled:         true,
					TrialCount:      10,
					AvgScore:        0.5,
				},
			},
		}

		sel := NewPromptSelector(store, &cfg)
		content, ok := sel.SelectVariant("coder", "openai")
		if !ok {
			t.Fatal("expected ok=true with candidates")
		}
		if content != "candidate 1" && content != "candidate 2" {
			t.Errorf("expected a candidate content, got %q", content)
		}
	})

	t.Run("mode_override_strategy", func(t *testing.T) {
		t.Parallel()
		cfg := prompt.DefaultEvolutionConfig()
		cfg.PromotionStrategy = prompt.StrategyAuto
		cfg.ModeOverrides = map[string]prompt.ModeEvolutionOverride{
			"reviewer": {PromotionStrategy: prompt.StrategyManual},
		}
		store := &inMemoryVariantStore{
			variants: []prompt.PromptVariant{
				{
					ID:              "v1",
					ModeID:          "reviewer",
					ModelFamily:     "openai",
					Content:         "candidate reviewer",
					PromotionStatus: prompt.PromotionCandidate,
					Enabled:         true,
				},
			},
		}
		sel := NewPromptSelector(store, &cfg)
		_, ok := sel.SelectVariant("reviewer", "openai")
		if ok {
			t.Error("reviewer mode has manual strategy, should not return candidates")
		}
	})

	t.Run("empty_entryID_returns_not_ok", func(t *testing.T) {
		t.Parallel()
		store := &inMemoryVariantStore{}
		sel := NewPromptSelector(store, ptrTo(prompt.DefaultEvolutionConfig()))
		_, ok := sel.SelectVariant("", "openai")
		if ok {
			t.Error("expected ok=false for empty entryID")
		}
	})

	t.Run("empty_modelFamily_returns_not_ok", func(t *testing.T) {
		t.Parallel()
		store := &inMemoryVariantStore{}
		sel := NewPromptSelector(store, ptrTo(prompt.DefaultEvolutionConfig()))
		_, ok := sel.SelectVariant("coder", "")
		if ok {
			t.Error("expected ok=false for empty modelFamily")
		}
	})
}

func TestPromptSelector_UCB1Selection(t *testing.T) {
	t.Parallel()

	t.Run("prefers_low_trial_count_for_exploration", func(t *testing.T) {
		t.Parallel()
		store := &inMemoryVariantStore{
			variants: []prompt.PromptVariant{
				{
					ID:              "v1",
					ModeID:          "coder",
					ModelFamily:     "openai",
					Content:         "well-explored",
					PromotionStatus: prompt.PromotionCandidate,
					Enabled:         true,
					TrialCount:      100,
					AvgScore:        0.7,
				},
				{
					ID:              "v2",
					ModeID:          "coder",
					ModelFamily:     "openai",
					Content:         "under-explored",
					PromotionStatus: prompt.PromotionCandidate,
					Enabled:         true,
					TrialCount:      1,
					AvgScore:        0.5,
				},
			},
		}
		cfg := prompt.DefaultEvolutionConfig()
		cfg.PromotionStrategy = prompt.StrategyAuto
		sel := NewPromptSelector(store, &cfg)

		// UCB1 should strongly prefer the under-explored variant.
		content, ok := sel.SelectVariant("coder", "openai")
		if !ok {
			t.Fatal("expected ok=true")
		}
		// The UCB1 bonus for v2 (trial=1) should dominate.
		if content != "under-explored" {
			t.Errorf("expected under-explored variant (UCB1 bonus), got %q", content)
		}
	})
}
