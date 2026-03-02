package routing_test

import (
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/routing"
)

func TestComplexityTier_IsValid(t *testing.T) {
	tests := []struct {
		name string
		tier routing.ComplexityTier
		want bool
	}{
		{"simple", routing.TierSimple, true},
		{"medium", routing.TierMedium, true},
		{"complex", routing.TierComplex, true},
		{"reasoning", routing.TierReasoning, true},
		{"empty string", routing.ComplexityTier(""), false},
		{"unknown value", routing.ComplexityTier("ultra"), false},
		{"case sensitive", routing.ComplexityTier("Simple"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.tier.IsValid(); got != tt.want {
				t.Errorf("ComplexityTier(%q).IsValid() = %v, want %v", tt.tier, got, tt.want)
			}
		})
	}
}

func TestTaskType_IsValid(t *testing.T) {
	tests := []struct {
		name     string
		taskType routing.TaskType
		want     bool
	}{
		{"code", routing.TaskCode, true},
		{"review", routing.TaskReview, true},
		{"plan", routing.TaskPlan, true},
		{"qa", routing.TaskQA, true},
		{"chat", routing.TaskChat, true},
		{"debug", routing.TaskDebug, true},
		{"refactor", routing.TaskRefactor, true},
		{"empty string", routing.TaskType(""), false},
		{"unknown value", routing.TaskType("deploy"), false},
		{"case sensitive", routing.TaskType("Code"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.taskType.IsValid(); got != tt.want {
				t.Errorf("TaskType(%q).IsValid() = %v, want %v", tt.taskType, got, tt.want)
			}
		})
	}
}

func TestComplexityTier_StringValues(t *testing.T) {
	// Verify the string representations match database values.
	tests := []struct {
		tier routing.ComplexityTier
		want string
	}{
		{routing.TierSimple, "simple"},
		{routing.TierMedium, "medium"},
		{routing.TierComplex, "complex"},
		{routing.TierReasoning, "reasoning"},
	}
	for _, tt := range tests {
		if string(tt.tier) != tt.want {
			t.Errorf("ComplexityTier string = %q, want %q", tt.tier, tt.want)
		}
	}
}

func TestTaskType_StringValues(t *testing.T) {
	// Verify the string representations match database values.
	tests := []struct {
		taskType routing.TaskType
		want     string
	}{
		{routing.TaskCode, "code"},
		{routing.TaskReview, "review"},
		{routing.TaskPlan, "plan"},
		{routing.TaskQA, "qa"},
		{routing.TaskChat, "chat"},
		{routing.TaskDebug, "debug"},
		{routing.TaskRefactor, "refactor"},
	}
	for _, tt := range tests {
		if string(tt.taskType) != tt.want {
			t.Errorf("TaskType string = %q, want %q", tt.taskType, tt.want)
		}
	}
}

func TestModelPerformanceStats_Defaults(t *testing.T) {
	var s routing.ModelPerformanceStats

	if s.TrialCount != 0 {
		t.Errorf("TrialCount default = %d, want 0", s.TrialCount)
	}
	if s.AvgReward != 0.0 {
		t.Errorf("AvgReward default = %f, want 0.0", s.AvgReward)
	}
	if s.AvgCostUSD != 0.0 {
		t.Errorf("AvgCostUSD default = %f, want 0.0", s.AvgCostUSD)
	}
	if s.SupportsTools {
		t.Error("SupportsTools default should be false")
	}
	if s.SupportsVision {
		t.Error("SupportsVision default should be false")
	}
	if s.MaxContext != 0 {
		t.Errorf("MaxContext default = %d, want 0", s.MaxContext)
	}
	if s.LastSelected != nil {
		t.Error("LastSelected default should be nil")
	}
}

func TestRoutingOutcome_Defaults(t *testing.T) {
	var o routing.RoutingOutcome

	if o.Success {
		t.Error("Success default should be false")
	}
	if o.QualityScore != 0.0 {
		t.Errorf("QualityScore default = %f, want 0.0", o.QualityScore)
	}
	if o.Reward != 0.0 {
		t.Errorf("Reward default = %f, want 0.0", o.Reward)
	}
	if o.RoutingLayer != "" {
		t.Errorf("RoutingLayer default = %q, want empty", o.RoutingLayer)
	}
}

func TestModelPerformanceStats_Fields(t *testing.T) {
	now := time.Now()
	s := routing.ModelPerformanceStats{
		ID:             "test-id",
		ModelName:      "openai/gpt-4o",
		TaskType:       routing.TaskCode,
		ComplexityTier: routing.TierComplex,
		TrialCount:     42,
		TotalReward:    35.0,
		AvgReward:      0.833,
		AvgCostUSD:     0.05,
		AvgLatencyMs:   1500,
		AvgQuality:     0.9,
		LastSelected:   &now,
		SupportsTools:  true,
		SupportsVision: true,
		MaxContext:     128000,
		InputCostPer:   0.0025,
		OutputCostPer:  0.01,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if s.ModelName != "openai/gpt-4o" {
		t.Errorf("ModelName = %q, want %q", s.ModelName, "openai/gpt-4o")
	}
	if !s.TaskType.IsValid() {
		t.Errorf("TaskType %q should be valid", s.TaskType)
	}
	if !s.ComplexityTier.IsValid() {
		t.Errorf("ComplexityTier %q should be valid", s.ComplexityTier)
	}
	if s.TrialCount != 42 {
		t.Errorf("TrialCount = %d, want 42", s.TrialCount)
	}
	if !s.SupportsTools {
		t.Error("SupportsTools should be true")
	}
}

func TestRoutingOutcome_Fields(t *testing.T) {
	now := time.Now()
	o := routing.RoutingOutcome{
		ID:             "outcome-1",
		ModelName:      "anthropic/claude-sonnet-4",
		TaskType:       routing.TaskReview,
		ComplexityTier: routing.TierMedium,
		Success:        true,
		QualityScore:   0.85,
		CostUSD:        0.003,
		LatencyMs:      2500,
		TokensIn:       500,
		TokensOut:      200,
		Reward:         0.42,
		RoutingLayer:   "mab",
		RunID:          "run-123",
		ConversationID: "conv-456",
		PromptHash:     "abc123",
		CreatedAt:      now,
	}

	if !o.Success {
		t.Error("Success should be true")
	}
	if o.QualityScore != 0.85 {
		t.Errorf("QualityScore = %f, want 0.85", o.QualityScore)
	}
	if o.RoutingLayer != "mab" {
		t.Errorf("RoutingLayer = %q, want %q", o.RoutingLayer, "mab")
	}
	if o.TokensIn != 500 {
		t.Errorf("TokensIn = %d, want 500", o.TokensIn)
	}
}
