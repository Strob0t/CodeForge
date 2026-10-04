// Package broadcast defines the port for broadcasting real-time events to connected clients.
package broadcast

import "context"

// Broadcaster sends tenant-scoped real-time events to connected clients.
type Broadcaster interface {
	// BroadcastEvent sends a typed event to the clients of the tenant carried
	// in ctx (tenantctx). An event whose ctx carries no tenant is dropped and
	// logged, never sent to every tenant (fail closed).
	BroadcastEvent(ctx context.Context, eventType string, payload any)
}

// GlobalBroadcaster sends events that carry no tenant data (for example the
// health of the shared LLM proxy) to every connected client.
type GlobalBroadcaster interface {
	BroadcastGlobal(ctx context.Context, eventType string, payload any)
}
