// Package service contains application services.
package service

import (
	"context"
	"log/slog"

	"github.com/Strob0t/CodeForge/internal/port/notifier"
)

// NotificationService dispatches notifications to all registered notifiers.
type NotificationService struct {
	notifiers     []notifier.Notifier
	enabledEvents map[string]bool
}

// NewNotificationService creates a NotificationService with the given notifiers
// and list of enabled event types (e.g., "run.completed", "run.failed").
// If enabledEvents is nil or empty, all events are enabled.
func NewNotificationService(notifiers []notifier.Notifier, enabledEvents []string) *NotificationService {
	enabled := make(map[string]bool, len(enabledEvents))
	for _, e := range enabledEvents {
		enabled[e] = true
	}
	return &NotificationService{
		notifiers:     notifiers,
		enabledEvents: enabled,
	}
}

// Notify sends a notification to all registered notifiers.
// Errors are logged but do not interrupt delivery to other notifiers.
func (s *NotificationService) Notify(ctx context.Context, n notifier.Notification) {
	if len(s.enabledEvents) > 0 && !s.enabledEvents[n.Source] {
		return
	}

	for _, provider := range s.notifiers {
		if err := provider.Send(ctx, n); err != nil {
			slog.Warn("notification send failed",
				"provider", provider.Name(),
				"title", n.Title,
				"error", err,
			)
			continue
		}
		slog.Debug("notification sent", "provider", provider.Name(), "title", n.Title)
	}
}

// NotifierCount returns the number of registered notifiers.
func (s *NotificationService) NotifierCount() int {
	return len(s.notifiers)
}
