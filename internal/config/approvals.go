package config

import (
	"fmt"
	"net/mail"
	"slices"
	"strings"
)

// validateApprovals checks the settings of the operator's approval channels
// (email, Slack): bare email addresses, an absolute http(s) base URL of the
// web UI, and tenant IDs in notification.approval_tenants.
func validateApprovals(n *Notification) error {
	for _, r := range n.ApprovalRecipients {
		addr, err := mail.ParseAddress(r)
		if err != nil || addr.Address != r {
			return fmt.Errorf("notification.approval_recipients: %q must be one bare email address (name@example.com)", r)
		}
	}
	if _, err := n.ApprovalTenantIDs(); err != nil {
		return err
	}
	if n.WebUIURL == "" {
		return nil
	}
	return checkHTTPBaseURL("notification.web_ui_url", n.WebUIURL)
}

// ApprovalTenantIDs returns the tenants whose approval requests reach the
// operator's approval channels (KI-84): the UUIDs of
// notification.approval_tenants in lower case, each once. An entry that is
// not a UUID is an error.
func (n *Notification) ApprovalTenantIDs() ([]string, error) {
	ids := make([]string, 0, len(n.ApprovalTenants))
	for _, entry := range n.ApprovalTenants {
		id := strings.ToLower(strings.TrimSpace(entry))
		if !tenantIDPattern.MatchString(id) {
			return nil, fmt.Errorf("notification.approval_tenants: %q is not a tenant ID (UUID)", entry)
		}
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	return ids, nil
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

// SlackApprovalsMissing names the settings Slack approval messages still
// need (none: approval requests are posted with a link to the approval
// page).
func (n *Notification) SlackApprovalsMissing() []string {
	var missing []string
	if n.SlackWebhookURL == "" {
		missing = append(missing, "notification.slack_webhook_url")
	}
	if n.WebUIURL == "" {
		missing = append(missing, "notification.web_ui_url")
	}
	return missing
}
