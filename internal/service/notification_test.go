package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Strob0t/CodeForge/internal/port/notifier"
)

// mockNotifier implements notifier.Notifier for testing.
type mockNotifier struct {
	name    string
	sent    []notifier.Notification
	sendErr error
}

func (m *mockNotifier) Name() string                        { return m.name }
func (m *mockNotifier) Capabilities() notifier.Capabilities { return notifier.Capabilities{} }
func (m *mockNotifier) Send(_ context.Context, n notifier.Notification) error {
	if m.sendErr != nil {
		return m.sendErr
	}
	m.sent = append(m.sent, n)
	return nil
}

func TestNotificationService_Notify(t *testing.T) {
	m1 := &mockNotifier{name: "mock1"}
	m2 := &mockNotifier{name: "mock2"}
	svc := NewNotificationService([]notifier.Notifier{m1, m2}, nil)

	svc.Notify(context.Background(), notifier.Notification{
		Title:   "Test",
		Message: "Hello",
		Level:   "info",
		Source:  "run.completed",
	})

	if len(m1.sent) != 1 {
		t.Fatalf("expected 1 notification on mock1, got %d", len(m1.sent))
	}
	if len(m2.sent) != 1 {
		t.Fatalf("expected 1 notification on mock2, got %d", len(m2.sent))
	}
}

func TestNotificationService_FilterEvents(t *testing.T) {
	m := &mockNotifier{name: "mock"}
	svc := NewNotificationService([]notifier.Notifier{m}, []string{"run.failed"})

	// This should be filtered out
	svc.Notify(context.Background(), notifier.Notification{
		Title:  "Test",
		Source: "run.completed",
	})
	if len(m.sent) != 0 {
		t.Fatalf("expected 0 notifications (filtered), got %d", len(m.sent))
	}

	// This should pass through
	svc.Notify(context.Background(), notifier.Notification{
		Title:  "Test",
		Source: "run.failed",
	})
	if len(m.sent) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(m.sent))
	}
}

func TestNotificationService_ErrorContinues(t *testing.T) {
	failer := &mockNotifier{name: "fail", sendErr: errors.New("connection refused")}
	success := &mockNotifier{name: "ok"}
	svc := NewNotificationService([]notifier.Notifier{failer, success}, nil)

	svc.Notify(context.Background(), notifier.Notification{
		Title:  "Test",
		Source: "run.completed",
	})

	// First notifier failed but second should still receive
	if len(success.sent) != 1 {
		t.Fatalf("expected 1 notification on success notifier, got %d", len(success.sent))
	}
}

func TestNotificationService_Count(t *testing.T) {
	svc := NewNotificationService([]notifier.Notifier{
		&mockNotifier{name: "a"},
		&mockNotifier{name: "b"},
	}, nil)
	if svc.NotifierCount() != 2 {
		t.Fatalf("expected 2, got %d", svc.NotifierCount())
	}
}
