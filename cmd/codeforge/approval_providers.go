package main

import (
	"strings"

	emailAdapter "github.com/Strob0t/CodeForge/internal/adapter/email"
	slackAdapter "github.com/Strob0t/CodeForge/internal/adapter/slack"
	"github.com/Strob0t/CodeForge/internal/config"
	feedbackPort "github.com/Strob0t/CodeForge/internal/port/feedback"
)

// emailApprovalProvider returns the provider that emails approval requests
// of the configured tenants to the configured recipients with a link to the
// web UI's approval page, or nil and why it is not configured (KI-57,
// KI-84).
func emailApprovalProvider(n *config.Notification) (provider feedbackPort.Provider, disabledReason string) {
	if missing := n.EmailApprovalsMissing(); len(missing) > 0 {
		return nil, "approval emails disabled - not set: " + strings.Join(missing, ", ")
	}
	tenants, err := n.ApprovalTenantIDs()
	if err != nil {
		return nil, "approval emails disabled - " + err.Error()
	}
	notifier := emailAdapter.NewNotifier(emailAdapter.SMTPConfig{
		Host:     n.SMTPHost,
		Port:     n.SMTPPort,
		From:     n.SMTPFrom,
		Password: n.SMTPPassword,
	})
	return emailAdapter.NewFeedbackProvider(notifier, n.ApprovalRecipients, n.WebUIURL, tenants), ""
}

// slackApprovalProvider returns the provider that posts approval requests of
// the configured tenants to the Slack channel with a link to the web UI's
// approval page, or nil and why it is not configured (KI-84).
func slackApprovalProvider(n *config.Notification) (provider feedbackPort.Provider, disabledReason string) {
	if missing := n.SlackApprovalsMissing(); len(missing) > 0 {
		return nil, "slack approval messages disabled - not set: " + strings.Join(missing, ", ")
	}
	tenants, err := n.ApprovalTenantIDs()
	if err != nil {
		return nil, "slack approval messages disabled - " + err.Error()
	}
	return slackAdapter.NewFeedbackProvider(n.SlackWebhookURL, n.WebUIURL, tenants), ""
}
