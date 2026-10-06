package middleware

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/tenant"
)

// TenantChecker reports whether a tenant exists and is enabled
// (service.TenantService.ValidateExists): nil, tenant.ErrDisabled,
// domain.ErrNotFound, or the store's error.
type TenantChecker interface {
	ValidateExists(ctx context.Context, id string) error
}

// tenantVerdict is a cached answer of the checker.
type tenantVerdict struct {
	err   error
	until time.Time
}

// EnabledTenant returns middleware that refuses every authenticated request
// of a disabled or deleted tenant with 403 (KI-174); requests without a user
// (the public routes) pass. Verdicts are cached for ttl per tenant, so the
// common case costs no query; a store failure is answered 503 and not
// cached. Disabling a tenant therefore takes effect within ttl.
func EnabledTenant(check TenantChecker, ttl time.Duration) func(http.Handler) http.Handler {
	var mu sync.Mutex
	verdicts := map[string]tenantVerdict{}
	lookup := func(ctx context.Context, id string) error {
		now := time.Now()
		mu.Lock()
		v, ok := verdicts[id]
		mu.Unlock()
		if ok && now.Before(v.until) {
			return v.err
		}
		err := check.ValidateExists(ctx, id)
		if err == nil || errors.Is(err, tenant.ErrDisabled) || errors.Is(err, domain.ErrNotFound) {
			mu.Lock()
			verdicts[id] = tenantVerdict{err: err, until: now.Add(ttl)}
			mu.Unlock()
		}
		return err
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u := UserFromContext(r.Context())
			if u == nil {
				next.ServeHTTP(w, r)
				return
			}
			switch err := lookup(r.Context(), u.TenantID); {
			case err == nil:
				next.ServeHTTP(w, r)
			case errors.Is(err, tenant.ErrDisabled):
				writeJSONError(w, http.StatusForbidden, "tenant is disabled")
			case errors.Is(err, domain.ErrNotFound):
				writeJSONError(w, http.StatusForbidden, "tenant not found")
			default:
				writeJSONError(w, http.StatusServiceUnavailable, "tenant check unavailable")
			}
		})
	}
}

// RequireOwnTenant restricts a route whose {param} names a tenant to that
// tenant's own users; platform admins reach every tenant (KI-174). Any other
// tenant's ID is forbidden, whether it exists or not, so the answer reveals
// nothing about other tenants.
func RequireOwnTenant(param string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u := UserFromContext(r.Context())
			if u == nil {
				http.Error(w, `{"error":"authorization required"}`, http.StatusUnauthorized)
				return
			}
			if chi.URLParam(r, param) != u.TenantID && !u.IsPlatformAdmin() {
				http.Error(w, `{"error":"forbidden: platform admin required"}`, http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
