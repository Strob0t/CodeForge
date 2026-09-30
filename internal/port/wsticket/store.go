// Package wsticket defines the port interface for single-use WebSocket
// authentication tickets. This decouples the HTTP adapter from the concrete
// ws.TicketStore implementation.
package wsticket

import "time"

// Store manages single-use WebSocket authentication tickets.
// Tickets prevent credentials from appearing in query strings (CWE-598).
type Store interface {
	// Issue creates a new single-use ticket for the given user and tenant.
	Issue(userID, tenantID string) string
	// TTL returns how long an issued ticket stays valid.
	TTL() time.Duration
}
