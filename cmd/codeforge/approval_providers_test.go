package main

import (
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// KI-57: the email approval provider exists only when it can deliver:
// SMTP, sender, recipients and the web UI URL for the approval link.
func TestEmailApprovalProvider(t *testing.T) {
	full := config.Notification{
		SMTPHost: "smtp.example.com", SMTPPort: 587, SMTPFrom: "cf@example.com",
		ApprovalRecipients: []string{"ops@example.com"}, WebUIURL: "https://cf.example.com",
		ApprovalTenants: []string{tenantctx.DefaultTenantID},
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

// KI-84: the Slack approval provider posts a link to the approval page, so
// it exists only with the Slack webhook and the web UI URL.
func TestSlackApprovalProvider(t *testing.T) {
	full := config.Notification{
		SlackWebhookURL: "https://hooks.slack.com/services/T/B/x", WebUIURL: "https://cf.example.com",
		ApprovalTenants: []string{tenantctx.DefaultTenantID},
	}
	p, why := slackApprovalProvider(&full)
	if p == nil || p.Name() != "slack" || why != "" {
		t.Fatalf("configured: provider %v, reason %q", p, why)
	}

	noWebUI := full
	noWebUI.WebUIURL = ""
	p, why = slackApprovalProvider(&noWebUI)
	if p != nil || !strings.Contains(why, "notification.web_ui_url") {
		t.Fatalf("without web_ui_url: provider %v, reason %q", p, why)
	}

	nothing := config.Notification{}
	if p, why = slackApprovalProvider(&nothing); p != nil || !strings.Contains(why, "notification.slack_webhook_url") {
		t.Fatalf("unconfigured: provider %v, reason %q", p, why)
	}

	badTenants := full
	badTenants.ApprovalTenants = []string{"acme"}
	if p, why = slackApprovalProvider(&badTenants); p != nil || !strings.Contains(why, "notification.approval_tenants") {
		t.Fatalf("invalid tenants: provider %v, reason %q", p, why)
	}
}
