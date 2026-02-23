// Package ws implements the WebSocket adapter for real-time client communication.
package ws

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"

	"github.com/coder/websocket"
)

// Message is the envelope for all WebSocket messages.
type Message struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// conn wraps a single WebSocket connection.
type conn struct {
	ws       *websocket.Conn
	cancel   context.CancelFunc
	tenantID string
}

// Hub manages all active WebSocket connections and broadcasts messages.
type Hub struct {
	mu            sync.RWMutex
	conns         map[*conn]struct{}
	allowOrigin   string                       // allowed WebSocket origin (from CORS config)
	tenantFromCtx func(context.Context) string // extracts tenant ID from request context
}

// NewHub creates a new WebSocket hub with origin validation and tenant extraction.
func NewHub(allowOrigin string, tenantFromCtx func(context.Context) string) *Hub {
	return &Hub{
		conns:         make(map[*conn]struct{}),
		allowOrigin:   allowOrigin,
		tenantFromCtx: tenantFromCtx,
	}
}

// HandleWS returns an http.HandlerFunc that upgrades connections to WebSocket.
func (h *Hub) HandleWS(w http.ResponseWriter, r *http.Request) {
	opts := &websocket.AcceptOptions{}
	if h.allowOrigin != "" {
		opts.OriginPatterns = []string{h.allowOrigin}
	}

	ws, err := websocket.Accept(w, r, opts)
	if err != nil {
		slog.Error("websocket accept failed", "error", err)
		return
	}

	// Extract tenant ID from context (set by TenantID middleware)
	tenantID := ""
	if h.tenantFromCtx != nil {
		tenantID = h.tenantFromCtx(r.Context())
	}
	if tenantID == "" {
		tenantID = "00000000-0000-0000-0000-000000000000"
	}

	ctx, cancel := context.WithCancel(r.Context())
	c := &conn{ws: ws, cancel: cancel, tenantID: tenantID}

	h.mu.Lock()
	h.conns[c] = struct{}{}
	h.mu.Unlock()

	slog.Info("websocket connected", "remote", r.RemoteAddr, "tenant", tenantID)

	// Read loop blocks the handler to keep r.Context() alive.
	// Returning from the handler would cancel the request context and
	// immediately tear down the hijacked connection.
	defer func() {
		h.remove(c)
		_ = ws.Close(websocket.StatusNormalClosure, "")
	}()
	for {
		_, _, err := ws.Read(ctx)
		if err != nil {
			return
		}
	}
}

// Broadcast sends a message to all connected clients.
func (h *Hub) Broadcast(ctx context.Context, msg Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		slog.Error("websocket marshal failed", "error", err)
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for c := range h.conns {
		if err := c.ws.Write(ctx, websocket.MessageText, data); err != nil {
			slog.Debug("websocket write failed", "error", err)
			go h.remove(c)
		}
	}
}

// BroadcastToTenant sends a message only to clients of the specified tenant.
func (h *Hub) BroadcastToTenant(ctx context.Context, tenantID string, msg Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		slog.Error("websocket marshal failed", "error", err)
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for c := range h.conns {
		if c.tenantID != tenantID {
			continue
		}
		if err := c.ws.Write(ctx, websocket.MessageText, data); err != nil {
			slog.Debug("websocket write failed", "error", err)
			go h.remove(c)
		}
	}
}

// ConnectionCount returns the number of active connections.
func (h *Hub) ConnectionCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.conns)
}

func (h *Hub) remove(c *conn) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.conns[c]; ok {
		c.cancel()
		delete(h.conns, c)
		slog.Info("websocket disconnected")
	}
}
