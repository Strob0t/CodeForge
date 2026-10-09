package tenantctx

import "context"

const DefaultTenantID = "00000000-0000-0000-0000-000000000000"

type tenantCtxKey struct{}

// messageTenantKey holds the tenant a queue message was published under (its
// tenant header).
type messageTenantKey struct{}

func WithTenant(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, tenantCtxKey{}, tenantID)
}

// WithMessageTenant records the tenant of a queue message's header. It
// scopes ctx where no tenant was set explicitly: a tenant set with
// WithTenant (a request's, or the one the message payload names) overrides
// it.
func WithMessageTenant(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, messageTenantKey{}, tenantID)
}

// FromContext returns the tenant ID stored in ctx, or DefaultTenantID if absent.
func FromContext(ctx context.Context) string {
	if tid, ok := Lookup(ctx); ok {
		return tid
	}
	return DefaultTenantID
}

// Lookup returns the tenant ID stored in ctx and whether a non-empty one was
// set: the explicit tenant, else the message tenant. Unlike FromContext it
// never falls back to DefaultTenantID, so callers that must fail closed (such
// as WebSocket fan-out) can tell a missing tenant apart from the default
// tenant.
func Lookup(ctx context.Context) (string, bool) {
	if tid, ok := Explicit(ctx); ok {
		return tid, true
	}
	tid, ok := ctx.Value(messageTenantKey{}).(string)
	if !ok || tid == "" {
		return "", false
	}
	return tid, true
}

// Explicit returns the tenant set with WithTenant, ignoring a message tenant.
func Explicit(ctx context.Context) (string, bool) {
	tid, ok := ctx.Value(tenantCtxKey{}).(string)
	if !ok || tid == "" {
		return "", false
	}
	return tid, true
}
