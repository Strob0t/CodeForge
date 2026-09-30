package tenantctx

import "context"

const DefaultTenantID = "00000000-0000-0000-0000-000000000000"

type tenantCtxKey struct{}

func WithTenant(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, tenantCtxKey{}, tenantID)
}

// FromContext returns the tenant ID stored in ctx, or DefaultTenantID if absent.
func FromContext(ctx context.Context) string {
	if tid, ok := ctx.Value(tenantCtxKey{}).(string); ok {
		return tid
	}
	return DefaultTenantID
}

// Lookup returns the tenant ID stored in ctx and whether a non-empty one was
// set. Unlike FromContext it never falls back to DefaultTenantID, so callers
// that must fail closed (such as WebSocket fan-out) can tell a missing tenant
// apart from the default tenant.
func Lookup(ctx context.Context) (string, bool) {
	tid, ok := ctx.Value(tenantCtxKey{}).(string)
	if !ok || tid == "" {
		return "", false
	}
	return tid, true
}
