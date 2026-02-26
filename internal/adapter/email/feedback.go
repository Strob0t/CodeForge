package email

import (
	"context"
	"fmt"

	fb "github.com/Strob0t/CodeForge/internal/domain/feedback"
)

// FeedbackProvider sends approval requests via email with callback links.
type FeedbackProvider struct {
	notifier    *Notifier
	recipients  []string
	callbackURL string // Base URL for approval callback (e.g. "https://codeforge.local/api/v1/feedback")
}

// NewFeedbackProvider creates a new email feedback provider.
func NewFeedbackProvider(notifier *Notifier, recipients []string, callbackURL string) *FeedbackProvider {
	return &FeedbackProvider{
		notifier:    notifier,
		recipients:  recipients,
		callbackURL: callbackURL,
	}
}

// Name returns the provider identifier.
func (p *FeedbackProvider) Name() string {
	return "email"
}

// RequestFeedback sends an email with approve/deny links.
//
//nolint:gocritic // hugeParam: req must be passed by value to match feedback.Provider interface
func (p *FeedbackProvider) RequestFeedback(ctx context.Context, req fb.FeedbackRequest) (fb.FeedbackResult, error) {
	approveURL := fmt.Sprintf("%s/%s/%s?decision=allow", p.callbackURL, req.RunID, req.CallID)
	denyURL := fmt.Sprintf("%s/%s/%s?decision=deny", p.callbackURL, req.RunID, req.CallID)

	body := fmt.Sprintf(`<h2>Tool Approval Required</h2>
<p><strong>Run:</strong> %s</p>
<p><strong>Tool:</strong> %s</p>
<p><strong>Command:</strong> %s</p>
<p><strong>Path:</strong> %s</p>
<p>
  <a href="%s" style="background:green;color:white;padding:8px 16px;text-decoration:none;border-radius:4px;">Approve</a>
  &nbsp;
  <a href="%s" style="background:red;color:white;padding:8px 16px;text-decoration:none;border-radius:4px;">Deny</a>
</p>`,
		req.RunID, req.Tool, req.Command, req.Path, approveURL, denyURL)

	subject := fmt.Sprintf("[CodeForge] Approval required: %s on %s", req.Tool, req.Path)

	for _, to := range p.recipients {
		if err := p.notifier.Send(ctx, to, subject, body); err != nil {
			return fb.FeedbackResult{}, fmt.Errorf("send email to %s: %w", to, err)
		}
	}

	return fb.FeedbackResult{
		Provider: fb.ProviderEmail,
	}, nil
}
