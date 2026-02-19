package discord

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
	if n.Name() != "discord" {
		t.Fatalf("expected 'discord', got %q", n.Name())
	}
}

func TestCapabilities(t *testing.T) {
	n := NewNotifier("")
	caps := n.Capabilities()
	if !caps.RichFormatting {
		t.Fatal("expected RichFormatting=true")
	}
	if !caps.Threads {
		t.Fatal("expected Threads=true")
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
		w.WriteHeader(http.StatusNoContent) // Discord returns 204
	}))
	defer srv.Close()

	n := NewNotifier(srv.URL)
	err := n.Send(context.Background(), notifier.Notification{
		Title:   "Deploy Complete",
		Message: "Version 1.2.3 deployed successfully",
		Level:   "success",
		Source:  "run.completed",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSendAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"message":"rate limited"}`))
	}))
	defer srv.Close()

	n := NewNotifier(srv.URL)
	err := n.Send(context.Background(), notifier.Notification{
		Title:   "Test",
		Message: "Test message",
		Level:   "info",
	})
	if err == nil {
		t.Fatal("expected error for 429 response")
	}
}
