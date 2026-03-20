package middleware

import (
	"context"
	"net/http"
	"regexp"

	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// DefaultTenantID is the single-tenant default used when no X-Tenant-ID header is set.
var DefaultTenantID = tenantctx.DefaultTenantID

const headerTenantID = "X-Tenant-ID"

// uuidPattern validates UUID v4 format for tenant IDs (P1-7).
var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// TenantID is middleware that extracts the tenant ID from the authenticated user
// context (set by Auth middleware). Falls back to X-Tenant-ID header, then
// DefaultTenantID. This allows auth-disabled mode to work via header or default.
func TenantID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// First: use tenant from authenticated user (set by Auth middleware).
		if u := UserFromContext(r.Context()); u != nil && u.TenantID != "" {
			ctx := tenantctx.WithTenant(r.Context(), u.TenantID)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// Fallback: X-Tenant-ID header or default.
		tid := r.Header.Get(headerTenantID)
		if tid == "" {
			tid = DefaultTenantID
		} else if !uuidPattern.MatchString(tid) {
			// P1-7: validate UUID format on explicit header values
			http.Error(w, `{"error":"invalid tenant ID format"}`, http.StatusBadRequest)
			return
		}
		ctx := tenantctx.WithTenant(r.Context(), tid)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// TenantIDFromContext returns the tenant ID stored in ctx, or DefaultTenantID if absent.
func TenantIDFromContext(ctx context.Context) string {
	return tenantctx.FromContext(ctx)
}
