package prompt

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
	"time"
)

// SignalType classifies the source of a prompt score signal.
type SignalType string

const (
	SignalBenchmark  SignalType = "benchmark"
	SignalSuccess    SignalType = "success"
	SignalCost       SignalType = "cost"
	SignalUser       SignalType = "user"
	SignalEfficiency SignalType = "efficiency"
)

// ValidSignalType returns true if the given signal type is known.
func ValidSignalType(s SignalType) bool {
	switch s {
	case SignalBenchmark, SignalSuccess, SignalCost, SignalUser, SignalEfficiency:
		return true
	}
	return false
}

// PromotionStatus tracks the lifecycle of a prompt variant.
type PromotionStatus string

const (
	PromotionCandidate PromotionStatus = "candidate"
	PromotionPromoted  PromotionStatus = "promoted"
	PromotionRetired   PromotionStatus = "retired"
)

// PromotionStrategy determines how variants are promoted.
type PromotionStrategy string

const (
	StrategyAuto   PromotionStrategy = "auto"
	StrategyShadow PromotionStrategy = "shadow"
	StrategyManual PromotionStrategy = "manual"
)

// EvolutionTrigger determines what initiates the evolution loop.
type EvolutionTrigger string

const (
	TriggerBenchmark  EvolutionTrigger = "benchmark"
	TriggerContinuous EvolutionTrigger = "continuous"
	TriggerCron       EvolutionTrigger = "cron"
)

// SignalWeight defines the default weighting for each signal type
// in composite score calculation.
var SignalWeight = map[SignalType]float64{
	SignalBenchmark:  0.35,
	SignalSuccess:    0.25,
	SignalCost:       0.15,
	SignalUser:       0.15,
	SignalEfficiency: 0.10,
}

// PromptScore records a single scoring signal for a prompt variant.
type PromptScore struct {
	ID                string     `json:"id"`
	TenantID          string     `json:"tenant_id"`
	PromptFingerprint string     `json:"prompt_fingerprint"`
	ModeID            string     `json:"mode_id"`
	ModelFamily       string     `json:"model_family"`
	SignalType        SignalType `json:"signal_type"`
	Score             float64    `json:"score"`
	RunID             string     `json:"run_id,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
}

// PromptVariant represents an evolved prompt section stored in prompt_sections.
type PromptVariant struct {
	ID              string          `json:"id"`
	TenantID        string          `json:"tenant_id"`
	Name            string          `json:"name"`
	Scope           string          `json:"scope"`
	Content         string          `json:"content"`
	Priority        int             `json:"priority"`
	Enabled         bool            `json:"enabled"`
	Version         int             `json:"version"`
	ParentID        string          `json:"parent_id,omitempty"`
	MutationSource  string          `json:"mutation_source,omitempty"`
	PromotionStatus PromotionStatus `json:"promotion_status"`
	TrialCount      int             `json:"trial_count"`
	AvgScore        float64         `json:"avg_score"`
	ModeID          string          `json:"mode_id"`
	ModelFamily     string          `json:"model_family"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// EvolutionConfig holds the configuration for the prompt evolution system.
type EvolutionConfig struct {
	Enabled              bool                             `yaml:"enabled"              json:"enabled"`
	Trigger              EvolutionTrigger                 `yaml:"trigger"              json:"trigger"`
	CronSchedule         string                           `yaml:"cron_schedule"        json:"cron_schedule,omitempty"`
	MinFailures          int                              `yaml:"min_failures"         json:"min_failures"`
	MinTrials            int                              `yaml:"min_trials"           json:"min_trials"`
	ImprovementThreshold float64                          `yaml:"improvement_threshold" json:"improvement_threshold"`
	MaxVariantsPerMode   int                              `yaml:"max_variants_per_mode" json:"max_variants_per_mode"`
	PromotionStrategy    PromotionStrategy                `yaml:"promotion_strategy"   json:"promotion_strategy"`
	ReflectionModel      string                           `yaml:"reflection_model"     json:"reflection_model"`
	ModeOverrides        map[string]ModeEvolutionOverride `yaml:"modes" json:"modes,omitempty"`
}

// ModeEvolutionOverride allows per-mode overrides for evolution config.
type ModeEvolutionOverride struct {
	PromotionStrategy PromotionStrategy `yaml:"promotion_strategy" json:"promotion_strategy"`
}

// DefaultEvolutionConfig returns the default evolution configuration.
func DefaultEvolutionConfig() EvolutionConfig {
	return EvolutionConfig{
		Enabled:              true,
		Trigger:              TriggerBenchmark,
		MinFailures:          5,
		MinTrials:            30,
		ImprovementThreshold: 0.05,
		MaxVariantsPerMode:   3,
		PromotionStrategy:    StrategyAuto,
		ReflectionModel:      "anthropic/claude-sonnet-4-6",
	}
}

// StrategyForMode returns the promotion strategy for a given mode,
// checking mode-level overrides before falling back to the global strategy.
func (c *EvolutionConfig) StrategyForMode(modeID string) PromotionStrategy {
	if override, ok := c.ModeOverrides[modeID]; ok && override.PromotionStrategy != "" {
		return override.PromotionStrategy
	}
	return c.PromotionStrategy
}

// CompositeScore computes a weighted average across score dimensions.
// Returns 0 if no scores are provided.
func CompositeScore(scores map[SignalType]float64) float64 {
	var total, weightSum float64
	for signal, weight := range SignalWeight {
		if val, ok := scores[signal]; ok {
			total += val * weight
			weightSum += weight
		}
	}
	if weightSum == 0 {
		return 0
	}
	return total / weightSum
}

// Fingerprint computes a SHA256 fingerprint from sorted entry ID:content pairs.
// This deterministically identifies a specific prompt assembly.
func Fingerprint(entries []PromptEntry) string {
	pairs := make([]string, 0, len(entries))
	for i := range entries {
		h := sha256.Sum256([]byte(entries[i].Content))
		pairs = append(pairs, fmt.Sprintf("%s:%x", entries[i].ID, h[:8]))
	}
	sort.Strings(pairs)
	combined := strings.Join(pairs, "|")
	fp := sha256.Sum256([]byte(combined))
	return fmt.Sprintf("%x", fp)
}

// EvolutionStatus summarizes the current state of prompt evolution for API responses.
type EvolutionStatus struct {
	Enabled    bool                  `json:"enabled"`
	Trigger    EvolutionTrigger      `json:"trigger"`
	Strategy   PromotionStrategy     `json:"strategy"`
	ModeStatus map[string]ModeStatus `json:"mode_status,omitempty"`
}

// ModeStatus describes the evolution state for a single mode.
type ModeStatus struct {
	ModeID         string            `json:"mode_id"`
	ActiveVariant  string            `json:"active_variant,omitempty"`
	CandidateCount int               `json:"candidate_count"`
	TotalTrials    int               `json:"total_trials"`
	AvgScore       float64           `json:"avg_score"`
	Strategy       PromotionStrategy `json:"strategy"`
}
