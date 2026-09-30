package service

import (
	"context"

	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// Tenant scoping for work that does not run inside an HTTP request.
//
// Store queries fall back to the default tenant when ctx carries none, but
// WebSocket events without a tenant are dropped (KI-12, fail closed). NATS
// handlers and background goroutines therefore have to put the owning
// tenant into ctx before they touch the store or broadcast.

// withPayloadTenant scopes ctx to the tenant a NATS message carries. A tenant
// already in ctx (an HTTP request) wins; an empty payload tenant changes
// nothing.
func withPayloadTenant(ctx context.Context, tenantID string) context.Context {
	if _, ok := tenantctx.Lookup(ctx); ok || tenantID == "" {
		return ctx
	}
	return tenantctx.WithTenant(ctx, tenantID)
}

// withEntityTenant scopes ctx to the tenant that owns an entity loaded from
// the store. The stored row is authoritative. An empty tenant changes nothing.
func withEntityTenant(ctx context.Context, tenantID string) context.Context {
	if tenantID == "" {
		return ctx
	}
	return tenantctx.WithTenant(ctx, tenantID)
}

// detachTenant returns a context for work that outlives ctx (goroutines,
// timers): it is never cancelled and carries only ctx's tenant, if any.
func detachTenant(ctx context.Context) context.Context {
	if tenantID, ok := tenantctx.Lookup(ctx); ok {
		return tenantctx.WithTenant(context.Background(), tenantID)
	}
	return context.Background()
}
