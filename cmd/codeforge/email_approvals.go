package main

import (
	"strings"

	emailAdapter "github.com/Strob0t/CodeForge/internal/adapter/email"
	"github.com/Strob0t/CodeForge/internal/config"
	feedbackPort "github.com/Strob0t/CodeForge/internal/port/feedback"
)

// emailApprovalProvider returns the provider that emails approval requests
// to the configured recipients with a link to the web UI's approval page,
// or nil and why it is not configured (KI-57).
func emailApprovalProvider(n *config.Notification) (provider feedbackPort.Provider, disabledReason string) {
	if missing := n.EmailApprovalsMissing(); len(missing) > 0 {
		return nil, "approval emails disabled - not set: " + strings.Join(missing, ", ")
	}
	notifier := emailAdapter.NewNotifier(emailAdapter.SMTPConfig{
		Host:     n.SMTPHost,
		Port:     n.SMTPPort,
		From:     n.SMTPFrom,
		Password: n.SMTPPassword,
	})
	return emailAdapter.NewFeedbackProvider(notifier, n.ApprovalRecipients, n.WebUIURL), ""
}
