// Package slack provides a Slack-based human feedback provider
// using Block Kit interactive messages.
package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	fb "github.com/Strob0t/CodeForge/internal/domain/feedback"
)

// FeedbackProvider sends approval requests via Slack Block Kit messages.
type FeedbackProvider struct {
	webhookURL string
	httpClient *http.Client
}

// NewFeedbackProvider creates a new Slack feedback provider.
func NewFeedbackProvider(webhookURL string) *FeedbackProvider {
	return &FeedbackProvider{
		webhookURL: webhookURL,
		httpClient: &http.Client{},
	}
}

// Name returns the provider identifier.
func (p *FeedbackProvider) Name() string {
	return "slack"
}

// RequestFeedback sends a Block Kit message with Approve/Deny buttons.
// In a full implementation, this would listen for a callback via Slack's
// interaction endpoint. For now, it posts the notification and returns
// a pending result (the actual decision is received via the HTTP callback).
//
//nolint:gocritic // hugeParam: req must be passed by value to match feedback.Provider interface
func (p *FeedbackProvider) RequestFeedback(ctx context.Context, req fb.FeedbackRequest) (fb.FeedbackResult, error) {
	msg := map[string]any{
		"blocks": []map[string]any{
			{
				"type": "section",
				"text": map[string]string{
					"type": "mrkdwn",
					"text": fmt.Sprintf("*Tool Approval Required*\n\nRun: `%s`\nTool: `%s`\nCommand: `%s`\nPath: `%s`",
						req.RunID, req.Tool, req.Command, req.Path),
				},
			},
			{
				"type": "actions",
				"elements": []map[string]any{
					{
						"type":      "button",
						"text":      map[string]string{"type": "plain_text", "text": "Approve"},
						"style":     "primary",
						"action_id": "approve_" + req.CallID,
						"value":     req.CallID,
					},
					{
						"type":      "button",
						"text":      map[string]string{"type": "plain_text", "text": "Deny"},
						"style":     "danger",
						"action_id": "deny_" + req.CallID,
						"value":     req.CallID,
					},
				},
			},
		},
	}

	body, err := json.Marshal(msg)
	if err != nil {
		return fb.FeedbackResult{}, fmt.Errorf("marshal slack message: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.webhookURL, bytes.NewReader(body))
	if err != nil {
		return fb.FeedbackResult{}, fmt.Errorf("create slack request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	//nolint:gosec // G704: webhook URL is from config, not user-controlled
	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return fb.FeedbackResult{}, fmt.Errorf("send slack message: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fb.FeedbackResult{}, fmt.Errorf("slack webhook returned %d", resp.StatusCode)
	}

	// The actual decision comes via the Slack interaction callback (separate HTTP handler).
	// Return a pending/allow placeholder — the runtime service will use the first
	// response from any provider.
	return fb.FeedbackResult{
		Provider: fb.ProviderSlack,
	}, nil
}
