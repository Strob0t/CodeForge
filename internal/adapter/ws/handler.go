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
	ws     *websocket.Conn
	cancel context.CancelFunc
}

// Hub manages all active WebSocket connections and broadcasts messages.
type Hub struct {
	mu    sync.RWMutex
	conns map[*conn]struct{}
}

// NewHub creates a new WebSocket hub.
func NewHub() *Hub {
	return &Hub{
		conns: make(map[*conn]struct{}),
	}
}

// HandleWS returns an http.HandlerFunc that upgrades connections to WebSocket.
func (h *Hub) HandleWS(w http.ResponseWriter, r *http.Request) {
	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // CORS handled by middleware
	})
	if err != nil {
		slog.Error("websocket accept failed", "error", err)
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	c := &conn{ws: ws, cancel: cancel}

	h.mu.Lock()
	h.conns[c] = struct{}{}
	h.mu.Unlock()

	slog.Info("websocket connected", "remote", r.RemoteAddr)

	// Read loop (to detect disconnects and consume pings)
	go func() {
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
	}()
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
