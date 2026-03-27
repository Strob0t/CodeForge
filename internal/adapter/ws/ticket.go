package ws

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
)

// DefaultTicketTTL is the maximum age of a WebSocket ticket before it expires.
const DefaultTicketTTL = 30 * time.Second

// Ticket holds the identity claims associated with a single-use WebSocket ticket.
type Ticket struct {
	UserID    string
	TenantID  string
	CreatedAt time.Time
}

// TicketStore issues and redeems single-use WebSocket upgrade tickets.
// Tickets prevent credentials from appearing in query strings (CWE-598).
type TicketStore struct {
	mu      sync.Mutex
	tickets map[string]Ticket
	ttl     time.Duration
}

// NewTicketStore creates a TicketStore with the given ticket TTL.
func NewTicketStore(ttl time.Duration) *TicketStore {
	return &TicketStore{
		tickets: make(map[string]Ticket),
		ttl:     ttl,
	}
}

// Issue creates a new single-use ticket for the given user and tenant.
// The returned string is a UUID that the client exchanges for a WebSocket upgrade.
func (s *TicketStore) Issue(userID, tenantID string) string {
	id := uuid.New().String()

	s.mu.Lock()
	s.tickets[id] = Ticket{
		UserID:    userID,
		TenantID:  tenantID,
		CreatedAt: time.Now(),
	}
	s.mu.Unlock()

	return id
}

// Redeem validates and consumes a ticket. It returns the ticket claims and true
// on success, or nil and false if the ticket does not exist or has expired.
// The ticket is deleted on every call (single-use).
func (s *TicketStore) Redeem(ticket string) (*Ticket, bool) {
	s.mu.Lock()
	t, ok := s.tickets[ticket]
	if ok {
		delete(s.tickets, ticket)
	}
	s.mu.Unlock()

	if !ok {
		return nil, false
	}

	if time.Since(t.CreatedAt) > s.ttl {
		return nil, false
	}

	return &t, true
}

// StartCleanup runs a background goroutine that periodically removes expired
// tickets. It stops when ctx is cancelled.
func (s *TicketStore) StartCleanup(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(s.ttl)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.removeExpired()
			}
		}
	}()
}

// removeExpired deletes all tickets older than the configured TTL.
func (s *TicketStore) removeExpired() {
	now := time.Now()
	var removed int

	s.mu.Lock()
	for id, t := range s.tickets {
		if now.Sub(t.CreatedAt) > s.ttl {
			delete(s.tickets, id)
			removed++
		}
	}
	s.mu.Unlock()

	if removed > 0 {
		slog.Debug("ws ticket cleanup", "removed", removed)
	}
}
