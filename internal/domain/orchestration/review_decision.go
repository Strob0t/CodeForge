// Package orchestration defines domain models for intelligent agent orchestration.
package orchestration

// ReviewDecision represents the outcome of the confidence-based review router
// evaluating whether a plan step needs moderated review before execution.
type ReviewDecision struct {
	NeedsReview        bool     `json:"needs_review"`
	Confidence         float64  `json:"confidence"`
	Reason             string   `json:"reason"`
	SuggestedReviewers []string `json:"suggested_reviewers,omitempty"`
}
