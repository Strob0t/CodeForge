// Package ws implements the WebSocket adapter for real-time client communication.
package ws

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
)

const (
	// DefaultSendQueueSize is the number of messages buffered per client. A
	// client whose queue is full is too slow to keep up and is dropped. Each
	// streamed LLM chunk fans out to about three messages, so the queue covers a
	// burst of several hundred chunks while a healthy client's link stalls briefly.
	DefaultSendQueueSize = 1024
	// DefaultWriteTimeout bounds a single write to one client. A client that
	// does not accept a message within it is dropped.
	DefaultWriteTimeout = 10 * time.Second
)

// Message is the envelope for all WebSocket messages.
type Message struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// socket is the part of *websocket.Conn the hub writes through.
type socket interface {
	Write(ctx context.Context, typ websocket.MessageType, p []byte) error
	CloseNow() error
}

// conn is one registered client. Broadcasts only enqueue into send; the
// client's own writer goroutine drains it, so a slow client never blocks
// the hub or other clients.
type conn struct {
	sock     socket
	send     chan []byte
	ctx      context.Context
	cancel   context.CancelFunc
	tenantID string
	userID   string
}

// Hub manages all active WebSocket connections and broadcasts messages.
// Every connection belongs to the tenant of the single-use ticket it was
// opened with, and tenant-scoped events reach only that tenant's clients.
type Hub struct {
	mu           sync.RWMutex
	conns        map[*conn]struct{}
	allowOrigin  string       // allowed WebSocket origin (from CORS config)
	tickets      *TicketStore // redeems the ?ticket= of the upgrade request
	queueSize    int
	writeTimeout time.Duration
}

// Option configures a Hub.
type Option func(*Hub)

// WithSendQueueSize sets the per-client send queue size (default DefaultSendQueueSize).
func WithSendQueueSize(n int) Option {
	return func(h *Hub) {
		if n > 0 {
			h.queueSize = n
		}
	}
}

// WithWriteTimeout sets the per-write timeout (default DefaultWriteTimeout).
func WithWriteTimeout(d time.Duration) Option {
	return func(h *Hub) {
		if d > 0 {
			h.writeTimeout = d
		}
	}
}

// NewHub creates a WebSocket hub that validates the origin and authenticates
// upgrades with single-use tickets from tickets.
func NewHub(allowOrigin string, tickets *TicketStore, opts ...Option) *Hub {
	h := &Hub{
		conns:        make(map[*conn]struct{}),
		allowOrigin:  allowOrigin,
		tickets:      tickets,
		queueSize:    DefaultSendQueueSize,
		writeTimeout: DefaultWriteTimeout,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// HandleWS upgrades a request carrying a valid single-use ?ticket= (issued by
// POST /api/v1/ws/ticket) to a WebSocket connection. The connection belongs
// to the ticket's tenant; request headers cannot change it.
func (h *Hub) HandleWS(w http.ResponseWriter, r *http.Request) {
	ticket, ok := h.redeem(r.URL.Query().Get("ticket"))
	if !ok {
		slog.Warn("websocket upgrade rejected: invalid or expired ticket", "remote", r.RemoteAddr)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid or expired websocket ticket"}` + "\n"))
		return
	}

	opts := &websocket.AcceptOptions{}
	if h.allowOrigin != "" {
		opts.OriginPatterns = []string{h.allowOrigin}
	}

	ws, err := websocket.Accept(w, r, opts)
	if err != nil {
		slog.Error("websocket accept failed", "error", err)
		return
	}

	// The read loop below blocks the handler to keep r.Context() alive:
	// returning would cancel it and tear down the hijacked connection.
	c := h.register(r.Context(), ws, ticket.TenantID, ticket.UserID)
	slog.Info("websocket connected", "remote", r.RemoteAddr, "tenant", c.tenantID, "user", c.userID)

	defer func() {
		if h.remove(c) {
			slog.Info("websocket disconnected", "tenant", c.tenantID, "user", c.userID)
		}
		_ = ws.Close(websocket.StatusNormalClosure, "")
	}()
	for {
		if _, _, err := ws.Read(c.ctx); err != nil {
			return
		}
	}
}

// redeem consumes a ticket; a hub without a ticket store accepts none.
func (h *Hub) redeem(ticket string) (*Ticket, bool) {
	if h.tickets == nil {
		return nil, false
	}
	return h.tickets.Redeem(ticket)
}

// register adds a client and starts its writer goroutine. The client's
// context ends when the client is removed or parent ends.
func (h *Hub) register(parent context.Context, sock socket, tenantID, userID string) *conn {
	ctx, cancel := context.WithCancel(parent) //nolint:gosec // G118: cancel stored in conn.cancel, called via h.remove(c)
	c := &conn{
		sock:     sock,
		send:     make(chan []byte, h.queueSize),
		ctx:      ctx,
		cancel:   cancel,
		tenantID: tenantID,
		userID:   userID,
	}

	h.mu.Lock()
	h.conns[c] = struct{}{}
	h.mu.Unlock()

	go h.writeLoop(c)
	return c
}

// writeLoop writes queued messages to one client until the client is removed.
// A write that fails or exceeds the write timeout drops the client.
func (h *Hub) writeLoop(c *conn) {
	for {
		select {
		case <-c.ctx.Done():
			return
		case data := <-c.send:
			if err := h.write(c, data); err != nil {
				h.drop(c, "write failed", err)
				return
			}
		}
	}
}

func (h *Hub) write(c *conn, data []byte) error {
	ctx, cancel := context.WithTimeout(c.ctx, h.writeTimeout)
	defer cancel()
	return c.sock.Write(ctx, websocket.MessageText, data)
}

// deliver enqueues data for every client that match selects. It never writes
// to a socket and never blocks; a client whose queue is full is dropped.
func (h *Hub) deliver(data []byte, match func(*conn) bool) {
	h.mu.RLock()
	targets := make([]*conn, 0, len(h.conns))
	for c := range h.conns {
		if match(c) {
			targets = append(targets, c)
		}
	}
	h.mu.RUnlock()

	for _, c := range targets {
		select {
		case c.send <- data:
		default:
			h.drop(c, "send queue full", nil)
		}
	}
}

// drop disconnects a client that cannot keep up.
func (h *Hub) drop(c *conn, reason string, err error) {
	if !h.remove(c) {
		return
	}
	slog.Warn("websocket client dropped", "reason", reason, "tenant", c.tenantID, "user", c.userID, "error", err)
	_ = c.sock.CloseNow()
}

// DropUser closes every connection of the user and returns how many it
// closed (KI-143): a user whose tokens were invalidated must open a new
// connection with a new ticket. Connections without a user stay.
func (h *Hub) DropUser(userID string) int {
	if userID == "" {
		return 0
	}
	h.mu.RLock()
	var targets []*conn
	for c := range h.conns {
		if c.userID == userID {
			targets = append(targets, c)
		}
	}
	h.mu.RUnlock()

	for _, c := range targets {
		h.drop(c, "the user's sessions ended", nil)
	}
	return len(targets)
}

// ConnectionCount returns the number of active connections.
func (h *Hub) ConnectionCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.conns)
}

// remove unregisters c and stops its writer. It reports whether c was registered.
func (h *Hub) remove(c *conn) bool {
	h.mu.Lock()
	_, ok := h.conns[c]
	delete(h.conns, c)
	h.mu.Unlock()

	if ok {
		c.cancel()
	}
	return ok
}
