package tenantctx

import "context"

const DefaultTenantID = "00000000-0000-0000-0000-000000000000"

type tenantCtxKey struct{}

func WithTenant(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, tenantCtxKey{}, tenantID)
}

func FromContext(ctx context.Context) string {
	if tid, ok := ctx.Value(tenantCtxKey{}).(string); ok {
		return tid
	}
	return DefaultTenantID
}
