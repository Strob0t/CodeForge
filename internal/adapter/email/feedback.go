package email

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"net/url"
	"strings"

	fb "github.com/Strob0t/CodeForge/internal/domain/feedback"
)

// sender delivers one email (Notifier).
type sender interface {
	Send(ctx context.Context, to, subject, body string) error
}

// FeedbackProvider emails approval requests to the configured recipients.
// The email links to the web UI's approval page, where a signed-in user
// sees the request and decides; the email itself decides nothing.
type FeedbackProvider struct {
	sender     sender
	recipients []string
	webUIURL   string // base URL of the web UI, e.g. "https://codeforge.example.com"
}

// NewFeedbackProvider creates an email feedback provider.
func NewFeedbackProvider(s sender, recipients []string, webUIURL string) *FeedbackProvider {
	return &FeedbackProvider{
		sender:     s,
		recipients: recipients,
		webUIURL:   strings.TrimRight(webUIURL, "/"),
	}
}

// Name returns the provider identifier.
func (p *FeedbackProvider) Name() string {
	return "email"
}

// approvalMail is the body: what the web approval card shows, with the
// deciding profile and the arguments preview. html/template escapes
// everything the agent asks (tool, command, path, arguments) - it is
// untrusted text.
var approvalMail = template.Must(template.New("approval").Parse(`<h2>Tool approval required</h2>
<p><strong>Run:</strong> {{.RunID}}</p>
<p><strong>Tool:</strong> {{.Tool}}</p>
{{if .Command}}<p><strong>Command:</strong> <code>{{.Command}}</code></p>
{{end}}{{if .Path}}<p><strong>Path:</strong> <code>{{.Path}}</code></p>
{{end}}{{if .Profile}}<p><strong>Profile:</strong> {{.Profile}}</p>
{{end}}{{if .ArgumentsPreview}}<p><strong>Arguments:</strong> <code>{{.ArgumentsPreview}}</code></p>
{{end}}<p><a href="{{.Link}}">Review the request in CodeForge</a> (sign-in required) to approve or deny it before it times out.</p>
`))

// RequestFeedback emails every recipient a link to the approval page. It
// returns no decision: the decision arrives through the web UI.
//
//nolint:gocritic // hugeParam: req must be passed by value to match feedback.Provider interface
func (p *FeedbackProvider) RequestFeedback(ctx context.Context, req fb.FeedbackRequest) (fb.FeedbackResult, error) {
	var body bytes.Buffer
	err := approvalMail.Execute(&body, struct {
		RunID, Tool, Command, Path, Profile, ArgumentsPreview string
		Link                                                  template.URL
	}{
		RunID:            req.RunID,
		Tool:             req.Tool,
		Command:          req.Command,
		Path:             req.Path,
		Profile:          req.Profile,
		ArgumentsPreview: req.ArgumentsPreview,
		// The base URL comes from validated config; the IDs are escaped.
		Link: template.URL(p.webUIURL + "/approvals/" + url.PathEscape(req.RunID) + "/" + url.PathEscape(req.CallID)), //nolint:gosec // G203: config base URL plus path-escaped IDs
	})
	if err != nil {
		return fb.FeedbackResult{}, fmt.Errorf("render approval email: %w", err)
	}
	subject := "[CodeForge] Approval required: " + subjectSafe(req.Tool)

	var errs []error
	for _, to := range p.recipients {
		if err := p.sender.Send(ctx, to, subject, body.String()); err != nil {
			errs = append(errs, fmt.Errorf("send approval email to %s: %w", to, err))
		}
	}
	return fb.FeedbackResult{Provider: fb.ProviderEmail}, errors.Join(errs...)
}

// subjectSafe keeps a short, printable tool name for the subject header.
func subjectSafe(tool string) string {
	clean := strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, tool)
	if runes := []rune(clean); len(runes) > 64 {
		clean = string(runes[:64])
	}
	return clean
}
