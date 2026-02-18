package middleware

import (
	"context"
	"net/http"
)

// DefaultTenantID is the single-tenant default used when no X-Tenant-ID header is set.
const DefaultTenantID = "00000000-0000-0000-0000-000000000000"

const headerTenantID = "X-Tenant-ID"

type tenantCtxKey struct{}

// TenantID is middleware that extracts the tenant ID from the X-Tenant-ID header
// and stores it in the request context. Falls back to DefaultTenantID if absent.
func TenantID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tid := r.Header.Get(headerTenantID)
		if tid == "" {
			tid = DefaultTenantID
		}
		ctx := context.WithValue(r.Context(), tenantCtxKey{}, tid)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// TenantIDFromContext returns the tenant ID stored in ctx, or DefaultTenantID if absent.
func TenantIDFromContext(ctx context.Context) string {
	if tid, ok := ctx.Value(tenantCtxKey{}).(string); ok {
		return tid
	}
	return DefaultTenantID
}
