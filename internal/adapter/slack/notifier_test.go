package slack

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Strob0t/CodeForge/internal/port/notifier"
)

// Compile-time interface check.
var _ notifier.Notifier = (*Notifier)(nil)

func TestNotifierName(t *testing.T) {
	n := NewNotifier("")
	if n.Name() != "slack" {
		t.Fatalf("expected 'slack', got %q", n.Name())
	}
}

func TestCapabilities(t *testing.T) {
	n := NewNotifier("")
	caps := n.Capabilities()
	if !caps.RichFormatting {
		t.Fatal("expected RichFormatting=true")
	}
}

func TestSendNotConfigured(t *testing.T) {
	n := NewNotifier("")
	err := n.Send(context.Background(), notifier.Notification{Title: "test"})
	if err != notifier.ErrNotConfigured {
		t.Fatalf("expected ErrNotConfigured, got %v", err)
	}
}

func TestSendSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	n := NewNotifier(srv.URL)
	err := n.Send(context.Background(), notifier.Notification{
		Title:   "Build Passed",
		Message: "All tests green",
		Level:   "success",
		Source:  "run.completed",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSendAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	n := NewNotifier(srv.URL)
	err := n.Send(context.Background(), notifier.Notification{
		Title:   "Test",
		Message: "Test message",
		Level:   "info",
	})
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}
