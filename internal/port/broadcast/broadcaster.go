// Package broadcast defines the port for broadcasting real-time events to connected clients.
package broadcast

import "context"

// Broadcaster sends real-time events to all connected clients.
type Broadcaster interface {
	// BroadcastEvent sends a typed event to all connected clients.
	BroadcastEvent(ctx context.Context, eventType string, payload any)
}
