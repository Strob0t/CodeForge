// Package routing defines domain types for the intelligent model routing system.
// The routing system combines three layers — complexity analysis (rule-based),
// multi-armed bandit (learning from benchmark data), and LLM-as-router (cold-start
// fallback) — to select the best model for each task automatically.
package routing

import "time"

// ComplexityTier classifies prompt complexity for routing decisions.
type ComplexityTier string

const (
	TierSimple    ComplexityTier = "simple"
	TierMedium    ComplexityTier = "medium"
	TierComplex   ComplexityTier = "complex"
	TierReasoning ComplexityTier = "reasoning"
)

// IsValid returns true if the tier is one of the known values.
func (t ComplexityTier) IsValid() bool {
	switch t {
	case TierSimple, TierMedium, TierComplex, TierReasoning:
		return true
	}
	return false
}

// TaskType classifies the kind of work a prompt requests.
type TaskType string

const (
	TaskCode     TaskType = "code"
	TaskReview   TaskType = "review"
	TaskPlan     TaskType = "plan"
	TaskQA       TaskType = "qa"
	TaskChat     TaskType = "chat"
	TaskDebug    TaskType = "debug"
	TaskRefactor TaskType = "refactor"
)

// IsValid returns true if the task type is one of the known values.
func (t TaskType) IsValid() bool {
	switch t {
	case TaskCode, TaskReview, TaskPlan, TaskQA, TaskChat, TaskDebug, TaskRefactor:
		return true
	}
	return false
}

// ModelPerformanceStats holds aggregated MAB state for a model/task_type/complexity_tier
// combination. This is the primary data structure the UCB1 algorithm uses to select models.
type ModelPerformanceStats struct {
	ID             string         `json:"id"`
	ModelName      string         `json:"model_name"`
	TaskType       TaskType       `json:"task_type"`
	ComplexityTier ComplexityTier `json:"complexity_tier"`

	// MAB statistics.
	TrialCount   int        `json:"trial_count"`
	TotalReward  float64    `json:"total_reward"`
	AvgReward    float64    `json:"avg_reward"`
	AvgCostUSD   float64    `json:"avg_cost_usd"`
	AvgLatencyMs int64      `json:"avg_latency_ms"`
	AvgQuality   float64    `json:"avg_quality"`
	LastSelected *time.Time `json:"last_selected,omitempty"`

	// Model capabilities (populated from LiteLLM metadata during refresh).
	SupportsTools  bool    `json:"supports_tools"`
	SupportsVision bool    `json:"supports_vision"`
	MaxContext     int     `json:"max_context"`
	InputCostPer   float64 `json:"input_cost_per"`
	OutputCostPer  float64 `json:"output_cost_per"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// RoutingOutcome records the result of a single model routing decision.
// These individual outcomes feed the MAB reward computation and are periodically
// aggregated into ModelPerformanceStats.
type RoutingOutcome struct {
	ID             string         `json:"id"`
	ModelName      string         `json:"model_name"`
	TaskType       TaskType       `json:"task_type"`
	ComplexityTier ComplexityTier `json:"complexity_tier"`

	// Outcome metrics.
	Success      bool    `json:"success"`
	QualityScore float64 `json:"quality_score"`
	CostUSD      float64 `json:"cost_usd"`
	LatencyMs    int64   `json:"latency_ms"`
	TokensIn     int64   `json:"tokens_in"`
	TokensOut    int64   `json:"tokens_out"`
	Reward       float64 `json:"reward"`

	// Context.
	RoutingLayer   string `json:"routing_layer"`
	RunID          string `json:"run_id,omitempty"`
	ConversationID string `json:"conversation_id,omitempty"`
	PromptHash     string `json:"prompt_hash,omitempty"`

	CreatedAt time.Time `json:"created_at"`
}
