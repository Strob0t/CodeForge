package email

import (
	"context"
	"errors"
	"strings"
	"testing"

	fb "github.com/Strob0t/CodeForge/internal/domain/feedback"
)

// KI-57: approval emails reach the configured recipients and link to the
// web UI's approval page - no API call, no state-changing GET.

type sentMail struct{ to, subject, body string }

type recordingSender struct {
	sent   []sentMail
	failTo string
}

func (r *recordingSender) Send(_ context.Context, to, subject, body string) error {
	if to == r.failTo {
		return errors.New("mailbox unavailable")
	}
	r.sent = append(r.sent, sentMail{to, subject, body})
	return nil
}

func TestFeedbackProvider_MailsEveryRecipientALinkToTheApprovalPage(t *testing.T) {
	sender := &recordingSender{}
	p := NewFeedbackProvider(sender, []string{"ops@example.com", "lead@example.com"}, "https://cf.example.com/")

	res, err := p.RequestFeedback(context.Background(), fb.FeedbackRequest{RunID: "run 1", CallID: "call/2", Tool: "Bash", Command: "make deploy", Path: "."})
	if err != nil {
		t.Fatalf("RequestFeedback: %v", err)
	}
	// The decision is made on the web page, not by the email.
	if res.Decision != "" || res.Provider != fb.ProviderEmail {
		t.Fatalf("result = %+v", res)
	}
	if len(sender.sent) != 2 || sender.sent[0].to != "ops@example.com" || sender.sent[1].to != "lead@example.com" {
		t.Fatalf("sent = %+v", sender.sent)
	}
	body := sender.sent[0].body
	if !strings.Contains(body, `href="https://cf.example.com/approvals/run%201/call%2F2"`) {
		t.Fatalf("body has no link to the approval page: %s", body)
	}
	if strings.Contains(body, "/api/") || strings.Contains(body, "decision=") {
		t.Fatalf("body links to the API: %s", body)
	}
}

func TestFeedbackProvider_EscapesWhatTheAgentAsks(t *testing.T) {
	sender := &recordingSender{}
	p := NewFeedbackProvider(sender, []string{"ops@example.com"}, "https://cf.example.com")

	_, err := p.RequestFeedback(context.Background(), fb.FeedbackRequest{
		RunID: "r", CallID: "c",
		Tool:    "Bash\r\nBcc: victim@example.com",
		Command: `curl x <a href="https://evil.example">Approve</a>`,
		Path:    "<script>alert(1)</script>",
	})
	if err != nil {
		t.Fatalf("RequestFeedback: %v", err)
	}
	mail := sender.sent[0]
	if strings.ContainsAny(mail.subject, "\r\n") {
		t.Fatalf("subject carries a line break: %q", mail.subject)
	}
	if strings.Contains(mail.body, "<script>") || strings.Contains(mail.body, `<a href="https://evil.example">`) {
		t.Fatalf("body is not escaped: %s", mail.body)
	}
	if !strings.Contains(mail.body, "&lt;script&gt;") {
		t.Fatalf("body lost the path: %s", mail.body)
	}
}

func TestFeedbackProvider_OneFailedRecipientDoesNotStopTheOthers(t *testing.T) {
	sender := &recordingSender{failTo: "ops@example.com"}
	p := NewFeedbackProvider(sender, []string{"ops@example.com", "lead@example.com"}, "https://cf.example.com")

	_, err := p.RequestFeedback(context.Background(), fb.FeedbackRequest{RunID: "r", CallID: "c", Tool: "Edit"})
	if err == nil || !strings.Contains(err.Error(), "ops@example.com") {
		t.Fatalf("RequestFeedback = %v, want the failed recipient named", err)
	}
	if len(sender.sent) != 1 || sender.sent[0].to != "lead@example.com" {
		t.Fatalf("sent = %+v", sender.sent)
	}
}

func TestNotifier_RefusesHeaderInjection(t *testing.T) {
	n := NewNotifier(SMTPConfig{Host: "smtp.invalid", Port: 25, From: "cf@example.com"})
	for _, tc := range []struct{ to, subject string }{
		{"ops@example.com", "hi\r\nBcc: x@example.com"},
		{"ops@example.com\nBcc: x@example.com", "hi"},
	} {
		if err := n.Send(context.Background(), tc.to, tc.subject, "body"); err == nil || !strings.Contains(err.Error(), "line break") {
			t.Fatalf("Send(%q, %q) = %v, want a line-break refusal", tc.to, tc.subject, err)
		}
	}
}

// KI-69 (c): the approval email shows the deciding profile and the arguments
// preview; agent-controlled text is HTML-escaped.
func TestFeedbackProvider_ShowsProfileAndPreviewEscaped(t *testing.T) {
	sender := &recordingSender{}
	p := NewFeedbackProvider(sender, []string{"ops@example.com"}, "https://cf.example.com")

	_, err := p.RequestFeedback(context.Background(), fb.FeedbackRequest{
		RunID: "run-1", CallID: "c-1", Tool: "Bash", Command: `echo "<script>alert(1)</script>"`,
		Path: "a<b>.txt", Profile: "supervised-ask-all", ArgumentsPreview: `{"command": "<img src=x onerror=alert(1)>"}`,
	})
	if err != nil {
		t.Fatalf("RequestFeedback: %v", err)
	}
	body := sender.sent[0].body
	for _, want := range []string{"supervised-ask-all", "Arguments", "&lt;script&gt;", "&lt;img src=x onerror=alert(1)&gt;", "a&lt;b&gt;.txt"} {
		if !strings.Contains(body, want) {
			t.Errorf("body lacks %q:\n%s", want, body)
		}
	}
	for _, bad := range []string{"<script>", "<img src=x"} {
		if strings.Contains(body, bad) {
			t.Errorf("body contains unescaped %q", bad)
		}
	}
}
