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

// TenantGate is a TenantChecker that caches another checker's verdicts per
// tenant for a TTL, so the common case costs no query; a store failure is
// not cached. One gate serves every path that checks a tenant (EnabledTenant
// for users, A2AAuth for A2A keys), so disabling a tenant takes effect on
// all of them within the TTL.
type TenantGate struct {
	check    TenantChecker
	ttl      time.Duration
	mu       sync.Mutex
	verdicts map[string]tenantVerdict
}

// NewTenantGate returns a gate over check whose verdicts hold for ttl.
func NewTenantGate(check TenantChecker, ttl time.Duration) *TenantGate {
	return &TenantGate{check: check, ttl: ttl, verdicts: map[string]tenantVerdict{}}
}

// ValidateExists is the checker's verdict for the tenant, from the cache
// while it holds: nil, tenant.ErrDisabled, domain.ErrNotFound (these three
// are cached) or the store's error.
func (g *TenantGate) ValidateExists(ctx context.Context, id string) error {
	now := time.Now()
	g.mu.Lock()
	v, ok := g.verdicts[id]
	g.mu.Unlock()
	if ok && now.Before(v.until) {
		return v.err
	}
	err := g.check.ValidateExists(ctx, id)
	if err == nil || errors.Is(err, tenant.ErrDisabled) || errors.Is(err, domain.ErrNotFound) {
		g.mu.Lock()
		g.verdicts[id] = tenantVerdict{err: err, until: now.Add(g.ttl)}
		g.mu.Unlock()
	}
	return err
}

// refuseTenant answers a failed tenant check (TenantChecker.ValidateExists):
// 403 for a disabled or unknown tenant, 503 when the check itself failed.
// It reports whether it answered; a nil err is not answered.
func refuseTenant(w http.ResponseWriter, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, tenant.ErrDisabled):
		writeJSONError(w, http.StatusForbidden, "tenant is disabled")
	case errors.Is(err, domain.ErrNotFound):
		writeJSONError(w, http.StatusForbidden, "tenant not found")
	default:
		writeJSONError(w, http.StatusServiceUnavailable, "tenant check unavailable")
	}
	return true
}

// EnabledTenant returns middleware that refuses every authenticated request
// of a disabled or deleted tenant with 403 (KI-174); requests without a user
// (the public routes) pass. A store failure is answered 503. The checker is
// a TenantGate in production, so the common case costs no query.
func EnabledTenant(check TenantChecker) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u := UserFromContext(r.Context())
			if u == nil {
				next.ServeHTTP(w, r)
				return
			}
			if refuseTenant(w, check.ValidateExists(r.Context(), u.TenantID)) {
				return
			}
			next.ServeHTTP(w, r)
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
