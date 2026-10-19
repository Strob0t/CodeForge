package ws

import (
	"context"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

func TestNewHub(t *testing.T) {
	hub := NewHub("", nil)
	if hub == nil {
		t.Fatal("expected non-nil hub")
	}
	if hub.ConnectionCount() != 0 {
		t.Fatalf("expected 0 connections, got %d", hub.ConnectionCount())
	}
}

func TestHubConnectionCount(t *testing.T) {
	hub := NewHub("", nil)

	if got := hub.ConnectionCount(); got != 0 {
		t.Fatalf("expected 0, got %d", got)
	}
}

func TestHubBroadcastGlobalNoConnections(t *testing.T) {
	hub := NewHub("", nil)

	// BroadcastGlobal with no connections should not panic.
	hub.BroadcastGlobal(context.Background(), "test", map[string]string{"key": "value"})
}

func TestHubBroadcastEventNoConnections(t *testing.T) {
	hub := NewHub("", nil)

	// BroadcastEvent with no connections should not panic.
	hub.BroadcastEvent(context.Background(), event.EventTaskStatus, event.TaskStatusEvent{
		TaskID:    "t1",
		ProjectID: "p1",
		Status:    "completed",
	})
}

func TestHubBroadcastEventMarshalError(t *testing.T) {
	hub := NewHub("", nil)

	// A channel cannot be marshaled to JSON — should log error, not panic.
	hub.BroadcastEvent(tenantctx.WithTenant(context.Background(), "tenant-1"), "bad", make(chan int))
	hub.BroadcastGlobal(context.Background(), "bad", make(chan int))
}

func TestHubRemoveNonexistent(t *testing.T) {
	hub := NewHub("", nil)

	// Removing a connection that was never added should not panic.
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := &conn{sock: nil, cancel: cancel, tenantID: "test-tenant"}
	if hub.remove(c) {
		t.Fatal("remove() reported an unregistered connection as removed")
	}
}

func TestHubBroadcastToTenantNoConnections(t *testing.T) {
	hub := NewHub("", nil)

	// BroadcastToTenant with no connections should not panic.
	hub.BroadcastToTenant("tenant-1", Message{
		Type:    "test",
		Payload: []byte(`{"key":"value"}`),
	})
}
