package service

import (
	"context"
	"log/slog"
	"math"
	"math/rand/v2"
	"sync"

	"github.com/Strob0t/CodeForge/internal/domain/prompt"
)

// PromptVariantStore abstracts persistence for prompt variants.
type PromptVariantStore interface {
	GetVariantsByModeAndModel(ctx context.Context, modeID, modelFamily string) ([]prompt.PromptVariant, error)
	UpdateVariantStats(ctx context.Context, id string, trialCount int, avgScore float64) error
}

// explorationRate controls the fraction of requests routed to candidate
// exploration in auto strategy (80% exploit / 20% explore).
const explorationRate = 0.20

// PromptSelector selects the best prompt variant for a given mode + model
// using Pareto-aware selection with UCB1 exploration.
// Implements PromptVariantSelector interface for assembler integration.
type PromptSelector struct {
	store  PromptVariantStore
	config prompt.EvolutionConfig
	mu     sync.RWMutex
	rng    *rand.Rand
}

// NewPromptSelector creates a new selector with the given store and config.
func NewPromptSelector(store PromptVariantStore, config prompt.EvolutionConfig) *PromptSelector {
	return &PromptSelector{
		store:  store,
		config: config,
		rng:    rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64())),
	}
}

// SelectVariant implements PromptVariantSelector. It returns the content of
// the best variant for the given entry (mode) and model family, or ok=false
// if no variant should override the base YAML.
func (s *PromptSelector) SelectVariant(entryID, modelFamily string) (content string, ok bool) {
	if s.store == nil || entryID == "" || modelFamily == "" {
		return "", false
	}

	variants, err := s.store.GetVariantsByModeAndModel(context.Background(), entryID, modelFamily)
	if err != nil {
		slog.Error("failed to load variants", "mode_id", entryID, "error", err)
		return "", false
	}

	// Filter: only enabled, non-retired variants.
	active := filterActive(variants)
	if len(active) == 0 {
		return "", false
	}

	strategy := s.config.StrategyForMode(entryID)

	switch strategy {
	case prompt.StrategyManual, prompt.StrategyShadow:
		// Only return explicitly promoted variants.
		promoted := filterByStatus(active, prompt.PromotionPromoted)
		if len(promoted) == 0 {
			return "", false
		}
		return promoted[0].Content, true

	case prompt.StrategyAuto:
		return s.selectAuto(active)

	default:
		return "", false
	}
}

// selectAuto implements the auto strategy: 80% exploit promoted, 20% explore candidates via UCB1.
func (s *PromptSelector) selectAuto(variants []prompt.PromptVariant) (string, bool) {
	promoted := filterByStatus(variants, prompt.PromotionPromoted)
	candidates := filterByStatus(variants, prompt.PromotionCandidate)

	s.mu.Lock()
	explore := s.rng.Float64() < explorationRate
	s.mu.Unlock()

	// If we have both promoted and candidates, use the exploration rate.
	if len(promoted) > 0 && len(candidates) > 0 {
		if explore {
			return ucb1Select(candidates).Content, true
		}
		return promoted[0].Content, true
	}

	// Only promoted available.
	if len(promoted) > 0 {
		return promoted[0].Content, true
	}

	// Only candidates available — always explore via UCB1.
	if len(candidates) > 0 {
		return ucb1Select(candidates).Content, true
	}

	return "", false
}

// ucb1Select picks the candidate with the highest UCB1 score.
// UCB1 = avg_score + sqrt(2 * ln(total_trials) / trial_count)
func ucb1Select(candidates []prompt.PromptVariant) prompt.PromptVariant {
	if len(candidates) == 1 {
		return candidates[0]
	}

	totalTrials := 0
	for i := range candidates {
		totalTrials += candidates[i].TrialCount
	}
	if totalTrials == 0 {
		totalTrials = 1
	}

	logTotal := math.Log(float64(totalTrials))

	best := candidates[0]
	bestScore := ucb1Score(best, logTotal)

	for i := 1; i < len(candidates); i++ {
		score := ucb1Score(candidates[i], logTotal)
		if score > bestScore {
			bestScore = score
			best = candidates[i]
		}
	}

	return best
}

func ucb1Score(v prompt.PromptVariant, logTotal float64) float64 {
	if v.TrialCount == 0 {
		return math.Inf(1) // unexplored = infinite priority
	}
	return v.AvgScore + math.Sqrt(2*logTotal/float64(v.TrialCount))
}

func filterActive(variants []prompt.PromptVariant) []prompt.PromptVariant {
	result := make([]prompt.PromptVariant, 0, len(variants))
	for _, v := range variants {
		if v.Enabled && v.PromotionStatus != prompt.PromotionRetired {
			result = append(result, v)
		}
	}
	return result
}

func filterByStatus(variants []prompt.PromptVariant, status prompt.PromotionStatus) []prompt.PromptVariant {
	result := make([]prompt.PromptVariant, 0, len(variants))
	for _, v := range variants {
		if v.PromotionStatus == status {
			result = append(result, v)
		}
	}
	return result
}
