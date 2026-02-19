// Package notifier defines the notification port (interface) and capabilities.
package notifier

import (
	"context"
	"errors"
)

// ErrNotConfigured is returned when a notifier is not properly configured.
var ErrNotConfigured = errors.New("notifier: not configured")

// Notification is the payload sent through a Notifier.
type Notification struct {
	Title   string `json:"title"`
	Message string `json:"message"`
	Level   string `json:"level"`  // "info", "success", "warning", "error"
	Source  string `json:"source"` // e.g. "run.completed", "build.failed"
}

// Capabilities declares which features a notifier supports.
type Capabilities struct {
	RichFormatting bool `json:"rich_formatting"`
	Threads        bool `json:"threads"`
}

// Notifier is the port interface for sending notifications.
type Notifier interface {
	// Name returns the unique identifier for this notifier (e.g. "slack", "email").
	Name() string

	// Capabilities returns what this notifier supports.
	Capabilities() Capabilities

	// Send delivers a notification.
	Send(ctx context.Context, notification Notification) error
}
