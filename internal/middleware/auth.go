package middleware

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/service"
)

// writeJSONError writes a JSON error response with the correct Content-Type.
func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

type authUserCtxKey struct{}
type apiKeyCtxKey struct{}

// publicPaths are exempt from authentication.
var publicPaths = map[string]bool{
	"/health":                      true,
	"/health/ready":                true,
	"/api/v1/auth/login":           true,
	"/api/v1/auth/refresh":         true,
	"/api/v1/auth/setup-status":    true,
	"/api/v1/auth/setup":           true,
	"/api/v1/auth/forgot-password": true,
	"/api/v1/auth/reset-password":  true,
	"/api/v1/auth/github":          true,
	"/api/v1/auth/github/callback": true,
}

// publicPrefixes are path prefixes exempt from authentication.
// Webhook endpoints use their own HMAC/token verification middleware.
var publicPrefixes = []string{
	"/api/v1/webhooks/",
}

// passwordChangeExempt paths are allowed even when MustChangePassword is true.
var passwordChangeExempt = map[string]bool{
	"/api/v1/auth/change-password": true,
	"/api/v1/auth/logout":          true,
	"/api/v1/auth/me":              true,
}

// Auth returns middleware that validates JWT or API key credentials.
// When authEnabled is false, a default admin context is injected.
// The internalKey parameter is the shared secret for Python worker API calls
// (from cfg.InternalKey / CODEFORGE_INTERNAL_KEY env var).
func Auth(authSvc *service.AuthService, authEnabled bool, internalKey ...string) func(http.Handler) http.Handler {
	var internalKeyVal string
	if len(internalKey) > 0 {
		internalKeyVal = internalKey[0]
	}
	var authDisabledOnce sync.Once
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// When auth is disabled, inject a default admin user context.
			if !authEnabled {
				authDisabledOnce.Do(func() {
					slog.Warn("authentication is disabled - all requests are treated as admin")
				})
				defaultUser := &user.User{
					ID:       "00000000-0000-0000-0000-000000000000",
					Email:    "admin@localhost",
					Name:     "Admin",
					Role:     user.RoleAdmin,
					TenantID: DefaultTenantID,
					Enabled:  true,
				}
				ctx := context.WithValue(r.Context(), authUserCtxKey{}, defaultUser)
				ctx = withUserID(ctx, defaultUser.ID)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// Skip auth for public paths.
			if publicPaths[r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}

			// Skip auth for public path prefixes (webhooks use their own auth).
			for _, prefix := range publicPrefixes {
				if strings.HasPrefix(r.URL.Path, prefix) {
					next.ServeHTTP(w, r)
					return
				}
			}

			// WebSocket auth via ?token= query parameter (P1-5).
			//
			// ACCEPTED RISK (CWE-598): Token is passed in the URL query string because
			// browsers cannot set custom headers (Authorization) on WebSocket upgrade
			// requests. This exposes the token in:
			//   - Server access logs (if URL logging is enabled)
			//   - Browser history and address bar
			//   - Proxy/CDN logs along the request path
			//
			// Mitigations in place:
			//   1. Short-lived access tokens (default: 15min TTL) limit exposure window
			//   2. HTTPS in production encrypts the URL in transit (HSTS enforced)
			//   3. Token is validated server-side on every connection
			//   4. WebSocket connections are long-lived, so the token is sent only once
			//
			// Re-evaluate if:
			//   - Token lifetime is extended beyond 30min
			//   - Non-HTTPS deployments become supported
			//   - URL logging is enabled in production reverse proxies
			if r.URL.Path == "/ws" {
				tokenParam := r.URL.Query().Get("token")
				if tokenParam == "" {
					writeJSONError(w, http.StatusUnauthorized, "authorization required")
					return
				}
				claims, err := authSvc.ValidateAccessToken(tokenParam)
				if err != nil {
					writeJSONError(w, http.StatusUnauthorized, "invalid token")
					return
				}
				u := &user.User{
					ID:       claims.UserID,
					Email:    claims.Email,
					Name:     claims.Name,
					Role:     claims.Role,
					TenantID: claims.TenantID,
					Enabled:  true,
				}
				ctx := context.WithValue(r.Context(), authUserCtxKey{}, u)
				ctx = withUserID(ctx, u.ID)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// Try X-API-Key header first.
			if apiKey := r.Header.Get("X-API-Key"); apiKey != "" {
				// Internal service key — Python workers use this for API calls back to Go Core.
				// By design, the internal service key grants admin access for inter-service
				// communication (Go Core <-> Python workers). The key is never exposed to
				// external clients and must be set via CODEFORGE_INTERNAL_KEY env var.
				if internalKeyVal != "" && subtle.ConstantTimeCompare([]byte(apiKey), []byte(internalKeyVal)) == 1 {
					slog.Debug("internal service key authenticated", "path", r.URL.Path)
					svcUser := &user.User{
						ID:       "00000000-0000-0000-0000-000000000001",
						Email:    "service@internal",
						Name:     "Internal Service",
						Role:     user.RoleAdmin,
						TenantID: DefaultTenantID,
						Enabled:  true,
					}
					ctx := context.WithValue(r.Context(), authUserCtxKey{}, svcUser)
					ctx = withUserID(ctx, svcUser.ID)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}

				// User-created API key.
				u, key, err := authSvc.ValidateAPIKey(r.Context(), apiKey)
				if err != nil {
					writeJSONError(w, http.StatusUnauthorized, "invalid api key")
					return
				}
				if u.MustChangePassword && !passwordChangeExempt[r.URL.Path] {
					writeJSONError(w, http.StatusForbidden, "password change required")
					return
				}
				ctx := context.WithValue(r.Context(), authUserCtxKey{}, u)
				ctx = context.WithValue(ctx, apiKeyCtxKey{}, key)
				ctx = withUserID(ctx, u.ID)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// Try Authorization: Bearer <token> header.
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				writeJSONError(w, http.StatusUnauthorized, "authorization required")
				return
			}

			token := strings.TrimPrefix(authHeader, "Bearer ")
			if token == authHeader {
				writeJSONError(w, http.StatusUnauthorized, "invalid authorization header")
				return
			}

			claims, err := authSvc.ValidateAccessToken(token)
			if err != nil {
				writeJSONError(w, http.StatusUnauthorized, "invalid token")
				return
			}

			// MustChangePassword check (P2-2): force password change except on exempt paths
			if claims.MustChangePassword && !passwordChangeExempt[r.URL.Path] {
				writeJSONError(w, http.StatusForbidden, "password change required")
				return
			}

			u := &user.User{
				ID:                 claims.UserID,
				Email:              claims.Email,
				Name:               claims.Name,
				Role:               claims.Role,
				TenantID:           claims.TenantID,
				Enabled:            true,
				MustChangePassword: claims.MustChangePassword,
			}

			ctx := context.WithValue(r.Context(), authUserCtxKey{}, u)
			ctx = withUserID(ctx, u.ID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// UserFromContext returns the authenticated user from the request context.
func UserFromContext(ctx context.Context) *user.User {
	u, _ := ctx.Value(authUserCtxKey{}).(*user.User)
	return u
}

// APIKeyFromContext returns the API key used for authentication, or nil for JWT auth.
func APIKeyFromContext(ctx context.Context) *user.APIKey {
	key, _ := ctx.Value(apiKeyCtxKey{}).(*user.APIKey)
	return key
}

// ContextWithTestUser returns a context with the given user injected, matching
// the key used by the Auth middleware. Use only in tests.
func ContextWithTestUser(ctx context.Context, u *user.User) context.Context {
	return context.WithValue(ctx, authUserCtxKey{}, u)
}
