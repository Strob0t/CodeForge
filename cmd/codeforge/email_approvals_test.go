package main

import (
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/config"
)

// KI-57: the email approval provider exists only when it can deliver:
// SMTP, sender, recipients and the web UI URL for the approval link.
func TestEmailApprovalProvider(t *testing.T) {
	full := config.Notification{
		SMTPHost: "smtp.example.com", SMTPPort: 587, SMTPFrom: "cf@example.com",
		ApprovalRecipients: []string{"ops@example.com"}, WebUIURL: "https://cf.example.com",
	}
	p, why := emailApprovalProvider(&full)
	if p == nil || p.Name() != "email" || why != "" {
		t.Fatalf("configured: provider %v, reason %q", p, why)
	}

	noRecipients := full
	noRecipients.ApprovalRecipients = nil
	p, why = emailApprovalProvider(&noRecipients)
	if p != nil || !strings.Contains(why, "notification.approval_recipients") {
		t.Fatalf("without recipients: provider %v, reason %q", p, why)
	}

	nothing := config.Notification{}
	p, why = emailApprovalProvider(&nothing)
	if p != nil || !strings.Contains(why, "notification.smtp_host") || !strings.Contains(why, "notification.web_ui_url") {
		t.Fatalf("unconfigured: provider %v, reason %q", p, why)
	}
}
