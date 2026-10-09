package middleware

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// A2ATrustLevel represents the trust level for an A2A request.
type A2ATrustLevel string

const (
	// A2ATrustUntrusted means the request is unauthenticated.
	A2ATrustUntrusted A2ATrustLevel = "untrusted"
	// A2ATrustPartial means the request has a valid API key.
	A2ATrustPartial A2ATrustLevel = "partial"
)

type (
	ctxKeyA2ATrust  struct{}
	ctxKeyA2ACaller struct{}
)

// A2AAuth returns middleware that authenticates A2A requests by their
// Bearer token against the A2A API keys (KI-15); the global JWT middleware
// leaves the A2A routes to it. A caller with a valid key acts in the key's
// tenant, which replaces any tenant the request named (X-Tenant-ID), with
// "partial" trust. Without configured keys every request is refused (fail
// closed). Tokens are compared in constant time. The key's tenant must be
// enabled (tenants, the TenantGate the users' requests pass; checked after
// the key, so an invalid key learns nothing about a tenant): a key of a
// disabled or deleted tenant is refused like the tenant's users.
func A2AAuth(keys []config.A2AAPIKey, tenants TenantChecker) func(http.Handler) http.Handler {
	valid := make([]config.A2AAPIKey, len(keys))
	copy(valid, keys)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			key, matched := matchA2AKey(valid, token)
			if !ok || !matched {
				w.Header().Set("WWW-Authenticate", "Bearer")
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			if refuseTenant(w, tenants.ValidateExists(r.Context(), key.TenantID)) {
				return
			}

			ctx := tenantctx.WithTenant(r.Context(), key.TenantID)
			ctx = ContextWithA2ATrust(ctx, A2ATrustPartial)
			ctx = ContextWithA2ACaller(ctx, key.ID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// matchA2AKey returns the key that token is. Every key is compared, without
// early return, so the time taken does not leak which key matched.
func matchA2AKey(keys []config.A2AAPIKey, token string) (config.A2AAPIKey, bool) {
	var found config.A2AAPIKey
	match := 0
	for _, k := range keys {
		if subtle.ConstantTimeCompare([]byte(k.Key), []byte(token)) == 1 {
			found = k
			match = 1
		}
	}
	return found, match == 1 && token != ""
}

// ContextWithA2ATrust returns ctx carrying the A2A trust level of its caller.
func ContextWithA2ATrust(ctx context.Context, level A2ATrustLevel) context.Context {
	return context.WithValue(ctx, ctxKeyA2ATrust{}, level)
}

// ContextWithA2ACaller returns ctx carrying the ID of the A2A key that
// authenticated its caller.
func ContextWithA2ACaller(ctx context.Context, keyID string) context.Context {
	return context.WithValue(ctx, ctxKeyA2ACaller{}, keyID)
}

// A2ACallerFromContext returns the ID of the A2A key that authenticated the
// request ("" without one).
func A2ACallerFromContext(ctx context.Context) string {
	id, _ := ctx.Value(ctxKeyA2ACaller{}).(string)
	return id
}

// A2ATrustFromContext returns the A2A trust level from the request context.
func A2ATrustFromContext(ctx context.Context) A2ATrustLevel {
	v, ok := ctx.Value(ctxKeyA2ATrust{}).(A2ATrustLevel)
	if !ok {
		return A2ATrustUntrusted
	}
	return v
}
