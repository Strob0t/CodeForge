package config

import (
	"fmt"
	"net/mail"
)

// validateEmailApprovals checks the approval email settings: bare email
// addresses, and an absolute http(s) base URL of the web UI.
func validateEmailApprovals(n *Notification) error {
	for _, r := range n.ApprovalRecipients {
		addr, err := mail.ParseAddress(r)
		if err != nil || addr.Address != r {
			return fmt.Errorf("notification.approval_recipients: %q must be one bare email address (name@example.com)", r)
		}
	}
	if n.WebUIURL == "" {
		return nil
	}
	return checkHTTPBaseURL("notification.web_ui_url", n.WebUIURL)
}

// EmailApprovalsMissing names the settings approval emails still need
// (none: approval requests are emailed).
func (n *Notification) EmailApprovalsMissing() []string {
	var missing []string
	if n.SMTPHost == "" {
		missing = append(missing, "notification.smtp_host")
	}
	if n.SMTPFrom == "" {
		missing = append(missing, "notification.smtp_from")
	}
	if len(n.ApprovalRecipients) == 0 {
		missing = append(missing, "notification.approval_recipients")
	}
	if n.WebUIURL == "" {
		missing = append(missing, "notification.web_ui_url")
	}
	return missing
}
