package ws

import (
	"context"
	"testing"
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

func TestHubBroadcastNoConnections(t *testing.T) {
	hub := NewHub("", nil)

	// Broadcast with no connections should not panic.
	hub.Broadcast(context.Background(), Message{
		Type:    "test",
		Payload: []byte(`{"key":"value"}`),
	})
}

func TestHubBroadcastEventNoConnections(t *testing.T) {
	hub := NewHub("", nil)

	// BroadcastEvent with no connections should not panic.
	hub.BroadcastEvent(context.Background(), EventTaskStatus, TaskStatusEvent{
		TaskID:    "t1",
		ProjectID: "p1",
		Status:    "completed",
	})
}

func TestHubBroadcastEventMarshalError(t *testing.T) {
	hub := NewHub("", nil)

	// A channel cannot be marshaled to JSON — should log error, not panic.
	hub.BroadcastEvent(context.Background(), "bad", make(chan int))
}

func TestHubRemoveNonexistent(t *testing.T) {
	hub := NewHub("", nil)

	// Removing a connection that was never added should not panic.
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := &conn{ws: nil, cancel: cancel, tenantID: "test-tenant"}
	hub.remove(c)
}

func TestHubBroadcastToTenantNoConnections(t *testing.T) {
	hub := NewHub("", nil)

	// BroadcastToTenant with no connections should not panic.
	hub.BroadcastToTenant(context.Background(), "tenant-1", Message{
		Type:    "test",
		Payload: []byte(`{"key":"value"}`),
	})
}
