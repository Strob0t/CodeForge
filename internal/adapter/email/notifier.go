// Package email provides an SMTP-based notifier for the notification subsystem.
package email

import (
	"context"
	"fmt"
	"net/smtp"
)

// SMTPConfig holds the configuration for SMTP connections.
type SMTPConfig struct {
	Host     string
	Port     int
	From     string
	Password string
}

// Notifier sends email notifications via SMTP.
type Notifier struct {
	cfg SMTPConfig
}

// NewNotifier creates a new email notifier.
func NewNotifier(cfg SMTPConfig) *Notifier {
	return &Notifier{cfg: cfg}
}

// Send sends an email notification.
func (n *Notifier) Send(_ context.Context, to, subject, body string) error {
	addr := fmt.Sprintf("%s:%d", n.cfg.Host, n.cfg.Port)

	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n%s",
		n.cfg.From, to, subject, body)

	var auth smtp.Auth
	if n.cfg.Password != "" {
		auth = smtp.PlainAuth("", n.cfg.From, n.cfg.Password, n.cfg.Host)
	}

	return smtp.SendMail(addr, auth, n.cfg.From, []string{to}, []byte(msg))
}
