package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/Strob0t/CodeForge/internal/adapter/litellm"
	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/orchestration"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
)

// reviewRouterData provides data for the review router prompt template.
type reviewRouterData struct {
	TaskID      string
	AgentID     string
	ModeID      string
	Description string
}

// ReviewRouterService uses an LLM to evaluate whether a plan step needs
// moderated review before execution, based on confidence scoring.
type ReviewRouterService struct {
	llm     *litellm.Client
	orchCfg *config.Orchestrator
}

// NewReviewRouterService creates a ReviewRouterService.
func NewReviewRouterService(
	llm *litellm.Client,
	orchCfg *config.Orchestrator,
) *ReviewRouterService {
	return &ReviewRouterService{
		llm:     llm,
		orchCfg: orchCfg,
	}
}

// Evaluate assesses whether a plan step needs moderated review.
// It calls an LLM with the step context and returns a ReviewDecision.
// If the review router is disabled, it returns a no-review decision.
func (s *ReviewRouterService) Evaluate(ctx context.Context, step *plan.Step, taskDescription string) (*orchestration.ReviewDecision, error) {
	if !s.orchCfg.ReviewRouterEnabled {
		return &orchestration.ReviewDecision{
			NeedsReview: false,
			Confidence:  1.0,
			Reason:      "review router disabled",
		}, nil
	}

	data := reviewRouterData{
		TaskID:      step.TaskID,
		AgentID:     step.AgentID,
		ModeID:      step.ModeID,
		Description: sanitizePromptInput(taskDescription),
	}

	var buf bytes.Buffer
	if err := decomposeTemplates.ExecuteTemplate(&buf, "review_router.tmpl", data); err != nil {
		return nil, fmt.Errorf("execute review router template: %w", err)
	}
	prompt := buf.String()

	model := s.orchCfg.ReviewRouterModel
	if model == "" {
		model = s.orchCfg.DecomposeModel
	}

	resp, err := s.llm.ChatCompletion(ctx, litellm.ChatCompletionRequest{
		Model: model,
		Messages: []litellm.ChatMessage{
			{Role: "user", Content: prompt},
		},
		Temperature: 0.1,
		MaxTokens:   512,
	})
	if err != nil {
		return nil, fmt.Errorf("review router LLM call: %w", err)
	}

	decision, err := parseReviewDecision(resp.Content)
	if err != nil {
		slog.Warn("review router: failed to parse LLM response, defaulting to no-review",
			"error", err,
			"step_id", step.ID,
			"content", truncate(resp.Content, 200),
		)
		return &orchestration.ReviewDecision{
			NeedsReview: false,
			Confidence:  0.5,
			Reason:      "failed to parse LLM evaluation, defaulting to auto-proceed",
		}, nil
	}

	slog.Info("review router evaluated step",
		"step_id", step.ID,
		"needs_review", decision.NeedsReview,
		"confidence", decision.Confidence,
		"reason", decision.Reason,
	)

	return decision, nil
}

// ShouldRoute returns true if the decision indicates the step should be
// routed through review, based on the confidence threshold.
func (s *ReviewRouterService) ShouldRoute(decision *orchestration.ReviewDecision) bool {
	if !decision.NeedsReview {
		return false
	}
	return decision.Confidence < s.orchCfg.ReviewConfidenceThreshold
}

// parseReviewDecision extracts and validates the ReviewDecision from LLM output.
func parseReviewDecision(content string) (*orchestration.ReviewDecision, error) {
	jsonStr := extractJSON(content)
	var decision orchestration.ReviewDecision
	if err := json.Unmarshal([]byte(jsonStr), &decision); err != nil {
		return nil, fmt.Errorf("unmarshal review decision: %w", err)
	}

	// Clamp confidence to [0, 1]
	if decision.Confidence < 0 {
		decision.Confidence = 0
	}
	if decision.Confidence > 1 {
		decision.Confidence = 1
	}

	return &decision, nil
}
