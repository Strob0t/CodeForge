// Package feedback defines the port interface for human feedback providers.
package feedback

import (
	"context"

	fb "github.com/Strob0t/CodeForge/internal/domain/feedback"
)

// Provider is the interface for channels that collect human feedback (HITL approval).
type Provider interface {
	// RequestFeedback sends a feedback request to the channel and waits for a response.
	RequestFeedback(ctx context.Context, req fb.FeedbackRequest) (fb.FeedbackResult, error)

	// Name returns the provider identifier (e.g., "web", "slack", "email").
	Name() string
}
