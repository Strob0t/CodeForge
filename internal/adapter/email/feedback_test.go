package email

import (
	"strings"
	"testing"

	fb "github.com/Strob0t/CodeForge/internal/domain/feedback"
)

// KI-69 (c): the approval email shows the deciding profile and the arguments
// preview; agent-controlled text is HTML-escaped.
func TestApprovalEmail_ShowsProfileAndPreviewEscaped(t *testing.T) {
	subject, body := approvalEmail(fb.FeedbackRequest{
		RunID: "run-1", CallID: "c-1", Tool: "Bash", Command: `echo "<script>alert(1)</script>"`,
		Path: "a<b>.txt", Profile: "supervised-ask-all", ArgumentsPreview: `{"command": "<img src=x onerror=alert(1)>"}`,
	}, "https://cf.example/approve", "https://cf.example/deny")

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
	if strings.ContainsAny(subject, "\r\n") {
		t.Errorf("subject contains a line break: %q", subject)
	}
}

// A path with a line break must not add mail headers.
func TestApprovalEmail_SubjectHasNoLineBreaks(t *testing.T) {
	subject, _ := approvalEmail(fb.FeedbackRequest{Tool: "Write", Path: "x\r\nBcc: evil@example.com"}, "a", "d")
	if strings.ContainsAny(subject, "\r\n") {
		t.Fatalf("subject contains a line break: %q", subject)
	}
}
