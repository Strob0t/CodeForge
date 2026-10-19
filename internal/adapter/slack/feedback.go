// Package slack provides a Slack-based human feedback provider: it posts
// approval requests to the operator's Slack channel (incoming webhook).
package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	fb "github.com/Strob0t/CodeForge/internal/domain/feedback"
)

// FeedbackProvider posts approval requests to a Slack channel with a link to
// the web UI's approval page, where a signed-in user decides (KI-84). The
// message decides nothing: CodeForge has no Slack interaction endpoint, so
// it carries no Approve/Deny buttons. Only the requests of the configured
// tenants are posted - the channel is the operator's.
type FeedbackProvider struct {
	webhookURL string
	webUIURL   string   // base URL of the web UI, e.g. "https://codeforge.example.com"
	tenants    []string // tenants whose requests are posted (notification.approval_tenants)
	httpClient *http.Client
}

// NewFeedbackProvider creates a Slack feedback provider that posts the
// approval requests of tenants to the incoming webhook webhookURL, linking
// to the approval page of the web UI at webUIURL.
func NewFeedbackProvider(webhookURL, webUIURL string, tenants []string) *FeedbackProvider {
	return &FeedbackProvider{
		webhookURL: webhookURL,
		webUIURL:   strings.TrimRight(webUIURL, "/"),
		tenants:    tenants,
		httpClient: httpClient,
	}
}

// Name returns the provider identifier.
func (p *FeedbackProvider) Name() string {
	return "slack"
}

// approvalMessage is an incoming-webhook message. Slack does not unfurl the
// links in it: no preview of anything the agent named is fetched or shown.
type approvalMessage struct {
	Text        string       `json:"text"` // notification fallback
	Blocks      []slackBlock `json:"blocks"`
	UnfurlLinks bool         `json:"unfurl_links"`
	UnfurlMedia bool         `json:"unfurl_media"`
}

// RequestFeedback posts the request with a link to its approval page. It
// returns no decision: the decision arrives through the web UI.
//
//nolint:gocritic // hugeParam: req must be passed by value to match feedback.Provider interface
func (p *FeedbackProvider) RequestFeedback(ctx context.Context, req fb.FeedbackRequest) (fb.FeedbackResult, error) {
	result := fb.FeedbackResult{Provider: fb.ProviderSlack}
	if !fb.SendsTo(p.tenants, req.TenantID) {
		slog.DebugContext(ctx, "slack approval message skipped: the tenant is not in notification.approval_tenants",
			"tenant_id", req.TenantID, "run_id", req.RunID, "call_id", req.CallID)
		return result, nil
	}

	body, err := json.Marshal(approvalMessage{Text: "CodeForge: tool approval required", Blocks: p.approvalBlocks(&req)})
	if err != nil {
		return result, fmt.Errorf("marshal slack message: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.webhookURL, bytes.NewReader(body))
	if err != nil {
		return result, fmt.Errorf("create slack request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	//nolint:gosec // G704: webhook URL is from config, not user-controlled
	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return result, fmt.Errorf("send slack message: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return result, fmt.Errorf("slack webhook returned %d", resp.StatusCode)
	}
	return result, nil
}

// approvalBlocks renders what the web approval page shows: run, tool,
// command, path, deciding profile and arguments preview, one section each
// (Slack limits a section to 3000 characters), and the link to the page.
func (p *FeedbackProvider) approvalBlocks(req *fb.FeedbackRequest) []slackBlock {
	blocks := []slackBlock{mrkdwnSection("*Tool approval required*")}
	for _, f := range []struct{ label, value string }{
		{"Run", req.RunID},
		{"Tool", req.Tool},
		{"Command", req.Command},
		{"Path", req.Path},
		{"Profile", req.Profile},
		{"Arguments", req.ArgumentsPreview},
	} {
		if f.value != "" {
			blocks = append(blocks, mrkdwnSection("*"+f.label+":* `"+mrkdwnCode(f.value)+"`"))
		}
	}
	link := p.webUIURL + "/approvals/" + url.PathEscape(req.RunID) + "/" + url.PathEscape(req.CallID)
	return append(blocks, mrkdwnSection("<"+mrkdwnLinkEscaper.Replace(link)+"|Review the request in CodeForge>"+
		" (sign-in required) to approve or deny it before it times out."))
}

func mrkdwnSection(text string) slackBlock {
	return slackBlock{Type: "section", Text: &slackText{Type: "mrkdwn", Text: text}}
}

// maxValueRunes bounds each value shown: escaped (at most five characters
// per rune) it stays within a section's 3000 characters. The approval page
// shows the full value.
const maxValueRunes = 500

// wordJoiner (U+2060) is invisible; inserted into "@channel" or "https://"
// it keeps Slack from reading a mention or a link.
const wordJoiner = "\u2060"

// mrkdwnCodeEscaper makes agent-controlled text inert inside a mrkdwn code
// span: &, < and > are escaped as Slack requires (so <!channel>, <@U123> and
// <url|label> are text), a backtick would end the span, "@" could name
// @channel or @here, and a URL would become a link.
var mrkdwnCodeEscaper = strings.NewReplacer(
	"&", "&amp;", "<", "&lt;", ">", "&gt;",
	"`", "'",
	"@", "@"+wordJoiner,
	"://", ":"+wordJoiner+"//",
)

// wwwPattern matches the "www." Slack links without a scheme.
var wwwPattern = regexp.MustCompile(`(?i)\bwww\.`)

// mrkdwnLinkEscaper escapes the approval link for a Slack <url|label>: the
// base URL comes from validated config and the IDs are path-escaped, but
// "&" must be written as "&amp;" and "|" would end the URL.
var mrkdwnLinkEscaper = strings.NewReplacer("&", "&amp;", "<", "%3C", ">", "%3E", "|", "%7C")

// mrkdwnCode shortens agent-controlled text, puts it on one line (a code
// span ends at a line break) and makes it inert for mrkdwn.
func mrkdwnCode(s string) string {
	if runes := []rune(s); len(runes) > maxValueRunes {
		s = string(runes[:maxValueRunes]) + "..."
	}
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, s)
	s = mrkdwnCodeEscaper.Replace(s)
	return wwwPattern.ReplaceAllStringFunc(s, func(m string) string { return m[:3] + wordJoiner + "." })
}
