package email

import (
	"context"
	"fmt"
	"html"
	"net/url"
	"strings"

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
	approveURL := fmt.Sprintf("%s/%s/%s?decision=allow", p.callbackURL, url.PathEscape(req.RunID), url.PathEscape(req.CallID))
	denyURL := fmt.Sprintf("%s/%s/%s?decision=deny", p.callbackURL, url.PathEscape(req.RunID), url.PathEscape(req.CallID))
	subject, body := approvalEmail(req, approveURL, denyURL)

	for _, to := range p.recipients {
		if err := p.notifier.Send(ctx, to, subject, body); err != nil {
			return fb.FeedbackResult{}, fmt.Errorf("send email to %s: %w", to, err)
		}
	}

	return fb.FeedbackResult{
		Provider: fb.ProviderEmail,
	}, nil
}

// approvalEmail renders the subject and HTML body of an approval request:
// what the web approval card shows, with the deciding profile and the
// arguments preview. The values come from the agent, so the body escapes
// them and the subject (a mail header) gets no line breaks.
//
//nolint:gocritic // hugeParam: the request is passed by value like in RequestFeedback
func approvalEmail(req fb.FeedbackRequest, approveURL, denyURL string) (subject, body string) {
	esc := html.EscapeString
	body = fmt.Sprintf(`<h2>Tool Approval Required</h2>
<p><strong>Run:</strong> %s</p>
<p><strong>Tool:</strong> %s</p>
<p><strong>Command:</strong> %s</p>
<p><strong>Path:</strong> %s</p>
<p><strong>Profile:</strong> %s</p>
<p><strong>Arguments:</strong> <code>%s</code></p>
<p>
  <a href="%s" style="background:green;color:white;padding:8px 16px;text-decoration:none;border-radius:4px;">Approve</a>
  &nbsp;
  <a href="%s" style="background:red;color:white;padding:8px 16px;text-decoration:none;border-radius:4px;">Deny</a>
</p>`,
		esc(req.RunID), esc(req.Tool), esc(req.Command), esc(req.Path), esc(req.Profile), esc(req.ArgumentsPreview),
		esc(approveURL), esc(denyURL))
	subject = headerLineBreaks.Replace(fmt.Sprintf("[CodeForge] Approval required: %s on %s", req.Tool, req.Path))
	return subject, body
}

var headerLineBreaks = strings.NewReplacer("\r", " ", "\n", " ")
