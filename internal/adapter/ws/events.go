package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// BroadcastEvent marshals a typed event and sends it to the clients of the
// tenant carried in ctx. Without a tenant in ctx the event is dropped and
// logged: it is never sent to every tenant (fail closed).
func (h *Hub) BroadcastEvent(ctx context.Context, eventType string, payload any) {
	tenantID, ok := tenantctx.Lookup(ctx)
	if !ok {
		slog.Warn("websocket event dropped: no tenant in context", "type", eventType)
		return
	}
	msg, err := newMessage(eventType, payload)
	if err != nil {
		slog.Error("marshal ws event payload", "type", eventType, "error", err)
		return
	}
	h.BroadcastToTenant(tenantID, msg)
}

// BroadcastGlobal sends an event that carries no tenant data to every client.
func (h *Hub) BroadcastGlobal(_ context.Context, eventType string, payload any) {
	msg, err := newMessage(eventType, payload)
	if err != nil {
		slog.Error("marshal ws event payload", "type", eventType, "error", err)
		return
	}
	data, err := json.Marshal(msg)
	if err != nil {
		slog.Error("websocket marshal failed", "type", eventType, "error", err)
		return
	}
	h.deliver(data, func(*conn) bool { return true })
}

// BroadcastToTenant sends a message only to clients of the specified tenant.
// An empty tenant ID matches no client.
func (h *Hub) BroadcastToTenant(tenantID string, msg Message) {
	if tenantID == "" {
		slog.Warn("websocket event dropped: empty tenant", "type", msg.Type)
		return
	}
	data, err := json.Marshal(msg)
	if err != nil {
		slog.Error("websocket marshal failed", "type", msg.Type, "error", err)
		return
	}
	h.deliver(data, func(c *conn) bool { return c.tenantID == tenantID })
}

func newMessage(eventType string, payload any) (Message, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return Message{}, fmt.Errorf("marshal %s payload: %w", eventType, err)
	}
	return Message{Type: eventType, Payload: json.RawMessage(data)}, nil
}
