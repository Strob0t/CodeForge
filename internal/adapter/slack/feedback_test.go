package slack

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	fb "github.com/Strob0t/CodeForge/internal/domain/feedback"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

const otherTenant = "11111111-2222-4333-8444-555555555555"

// slackCapture is an incoming-webhook endpoint that records the posted
// messages.
type slackCapture struct {
	mu       sync.Mutex
	messages []map[string]json.RawMessage
	srv      *httptest.Server
}

func newSlackCapture(t *testing.T) *slackCapture {
	t.Helper()
	c := &slackCapture{}
	c.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		var msg map[string]json.RawMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			t.Errorf("posted message is not JSON: %v", err)
		}
		c.mu.Lock()
		c.messages = append(c.messages, msg)
		c.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(c.srv.Close)
	return c
}

func (c *slackCapture) posted() []map[string]json.RawMessage {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.messages
}

// texts returns the text of every block of a posted message.
func blockTexts(t *testing.T, msg map[string]json.RawMessage) string {
	t.Helper()
	var blocks []struct {
		Type string `json:"type"`
		Text struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"text"`
	}
	if err := json.Unmarshal(msg["blocks"], &blocks); err != nil {
		t.Fatalf("blocks: %v", err)
	}
	var b strings.Builder
	for _, bl := range blocks {
		if bl.Type != "section" || bl.Text.Type != "mrkdwn" {
			t.Fatalf("block %+v: want mrkdwn sections only (no buttons, no actions)", bl)
		}
		b.WriteString(bl.Text.Text)
		b.WriteString("\n")
	}
	return b.String()
}

func request(tenantID string) fb.FeedbackRequest {
	return fb.FeedbackRequest{
		TenantID: tenantID, RunID: "run-1", CallID: "call/1", Tool: "Bash", Command: "ls -la",
		Path: "src/main.go", Profile: "supervised-ask-all", ArgumentsPreview: `{"command":"ls -la"}`,
	}
}

// KI-84: Slack approval requests are sent only for the tenants the operator
// configured (notification.approval_tenants, default: the default tenant) -
// the channel is the operator's, other tenants' tools, commands and paths do
// not reach it. A request without a tenant is not sent either.
func TestFeedbackProvider_SendsOnlyTheConfiguredTenants(t *testing.T) {
	tests := []struct {
		name     string
		tenants  []string
		tenantID string
		wantPost bool
	}{
		{"default tenant, default config", []string{tenantctx.DefaultTenantID}, tenantctx.DefaultTenantID, true},
		{"other tenant, default config", []string{tenantctx.DefaultTenantID}, otherTenant, false},
		{"request without a tenant", []string{tenantctx.DefaultTenantID}, "", false},
		{"other tenant configured", []string{tenantctx.DefaultTenantID, otherTenant}, otherTenant, true},
		{"no tenant configured", nil, tenantctx.DefaultTenantID, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			capture := newSlackCapture(t)
			p := NewFeedbackProvider(capture.srv.URL, "https://codeforge.example.com", tc.tenants)
			res, err := p.RequestFeedback(context.Background(), request(tc.tenantID))
			if err != nil {
				t.Fatalf("RequestFeedback: %v", err)
			}
			if res.Decision != "" || res.Provider != fb.ProviderSlack {
				t.Fatalf("result = %+v, want no decision from provider slack", res)
			}
			if got := len(capture.posted()) == 1; got != tc.wantPost {
				t.Fatalf("posted %d messages, want post = %v", len(capture.posted()), tc.wantPost)
			}
		})
	}
}

// KI-84: the message has no Approve/Deny buttons (CodeForge has no Slack
// interaction endpoint; a click did nothing) but a link to the web UI's
// approval page, where a signed-in user decides (KI-57). It shows what the
// approval page shows, and Slack does not unfurl links in it.
func TestFeedbackProvider_LinksToTheApprovalPage(t *testing.T) {
	capture := newSlackCapture(t)
	p := NewFeedbackProvider(capture.srv.URL, "https://codeforge.example.com/", []string{tenantctx.DefaultTenantID})
	if _, err := p.RequestFeedback(context.Background(), request(tenantctx.DefaultTenantID)); err != nil {
		t.Fatalf("RequestFeedback: %v", err)
	}
	msgs := capture.posted()
	if len(msgs) != 1 {
		t.Fatalf("posted %d messages, want 1", len(msgs))
	}
	text := blockTexts(t, msgs[0])
	const link = "<https://codeforge.example.com/approvals/run-1/call%2F1|Review the request in CodeForge>"
	if !strings.Contains(text, link) {
		t.Fatalf("message lacks the approval link %s:\n%s", link, text)
	}
	for _, want := range []string{"`run-1`", "`Bash`", "`ls -la`", "`src/main.go`", "`supervised-ask-all`", "`{\"command\":\"ls -la\"}`"} {
		if !strings.Contains(text, want) {
			t.Errorf("message lacks %s:\n%s", want, text)
		}
	}
	if strings.Count(text, "<") != 1 {
		t.Errorf("message has another link or markup besides the approval link:\n%s", text)
	}
	for _, key := range []string{"unfurl_links", "unfurl_media"} {
		if string(msgs[0][key]) != "false" {
			t.Errorf("%s = %s, want false", key, msgs[0][key])
		}
	}
	var fallback string
	if err := json.Unmarshal(msgs[0]["text"], &fallback); err != nil || fallback == "" {
		t.Errorf("message has no notification text: %s", msgs[0]["text"])
	}
}

// KI-69 (c), KI-84: agent-controlled text cannot inject Slack markup - no
// mention (<!channel>, <@U123>, @channel, @here), no link (<url|label>, a
// bare URL Slack would link), and no end of the code span (backtick).
func TestFeedbackProvider_EscapesAgentText(t *testing.T) {
	capture := newSlackCapture(t)
	p := NewFeedbackProvider(capture.srv.URL, "https://codeforge.example.com", []string{tenantctx.DefaultTenantID})
	req := request(tenantctx.DefaultTenantID)
	req.Command = "echo `id` <!channel> & ls"
	req.Path = "<https://evil.example/login|Approve here>"
	req.ArgumentsPreview = `{"command": "echo <@U123> @channel @here see https://evil.example and www.evil.example", "x": "line1` + "\n" + `*line2*"}`
	if _, err := p.RequestFeedback(context.Background(), req); err != nil {
		t.Fatalf("RequestFeedback: %v", err)
	}
	text := blockTexts(t, capture.posted()[0])
	for _, want := range []string{"&lt;@\u2060U123&gt;", "&lt;!channel&gt;", "&amp; ls", "&lt;https:", "evil.example", "line1 *line2*"} {
		if !strings.Contains(text, want) {
			t.Errorf("message lacks %q:\n%s", want, text)
		}
	}
	for _, bad := range []string{"<!channel>", "<@U123>", "`id`", "@channel", "@here", "https://evil", "www.evil", "<https://evil", "|Approve here>", "line1\n"} {
		if strings.Contains(text, bad) {
			t.Errorf("message contains %q:\n%s", bad, text)
		}
	}
	if strings.Count(text, "<") != 1 {
		t.Errorf("message has a link besides the approval link:\n%s", text)
	}
}

// A long command is shortened, so the message stays within Slack's limit
// for a section (3000 characters) and is not refused; the approval page
// shows it in full.
func TestFeedbackProvider_ShortensLongValues(t *testing.T) {
	capture := newSlackCapture(t)
	p := NewFeedbackProvider(capture.srv.URL, "https://codeforge.example.com", []string{tenantctx.DefaultTenantID})
	req := request(tenantctx.DefaultTenantID)
	req.Command = strings.Repeat("&", 5000)
	if _, err := p.RequestFeedback(context.Background(), req); err != nil {
		t.Fatalf("RequestFeedback: %v", err)
	}
	var blocks []struct {
		Text struct {
			Text string `json:"text"`
		} `json:"text"`
	}
	if err := json.Unmarshal(capture.posted()[0]["blocks"], &blocks); err != nil {
		t.Fatal(err)
	}
	for _, b := range blocks {
		if n := len([]rune(b.Text.Text)); n > 3000 {
			t.Fatalf("a section has %d characters, Slack allows 3000", n)
		}
	}
}

func TestFeedbackProvider_ReportsSlackErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()
	p := NewFeedbackProvider(srv.URL, "https://codeforge.example.com", []string{tenantctx.DefaultTenantID})
	if _, err := p.RequestFeedback(context.Background(), request(tenantctx.DefaultTenantID)); err == nil {
		t.Fatal("a refused message is no error")
	}
}
