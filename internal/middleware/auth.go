package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/service"
)

type authUserCtxKey struct{}
type apiKeyCtxKey struct{}

// publicPaths are exempt from authentication.
var publicPaths = map[string]bool{
	"/health":              true,
	"/health/ready":        true,
	"/api/v1/auth/login":   true,
	"/api/v1/auth/refresh": true,
}

// passwordChangeExempt paths are allowed even when MustChangePassword is true.
var passwordChangeExempt = map[string]bool{
	"/api/v1/auth/change-password": true,
	"/api/v1/auth/logout":          true,
	"/api/v1/auth/me":              true,
}

// Auth returns middleware that validates JWT or API key credentials.
// When authEnabled is false, a default admin context is injected.
func Auth(authSvc *service.AuthService, authEnabled bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// When auth is disabled, inject a default admin user context.
			if !authEnabled {
				defaultUser := &user.User{
					ID:       "00000000-0000-0000-0000-000000000000",
					Email:    "admin@localhost",
					Name:     "Admin",
					Role:     user.RoleAdmin,
					TenantID: DefaultTenantID,
					Enabled:  true,
				}
				ctx := context.WithValue(r.Context(), authUserCtxKey{}, defaultUser)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// Skip auth for public paths.
			if publicPaths[r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}

			// WebSocket auth via ?token= query parameter (P1-5)
			if r.URL.Path == "/ws" {
				tokenParam := r.URL.Query().Get("token")
				if tokenParam == "" {
					http.Error(w, `{"error":"authorization required"}`, http.StatusUnauthorized)
					return
				}
				claims, err := authSvc.ValidateAccessToken(tokenParam)
				if err != nil {
					http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusUnauthorized)
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
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// Try X-API-Key header first.
			if apiKey := r.Header.Get("X-API-Key"); apiKey != "" {
				u, key, err := authSvc.ValidateAPIKey(r.Context(), apiKey)
				if err != nil {
					http.Error(w, `{"error":"invalid api key"}`, http.StatusUnauthorized)
					return
				}
				ctx := context.WithValue(r.Context(), authUserCtxKey{}, u)
				ctx = context.WithValue(ctx, apiKeyCtxKey{}, key)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// Try Authorization: Bearer <token> header.
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, `{"error":"authorization required"}`, http.StatusUnauthorized)
				return
			}

			token := strings.TrimPrefix(authHeader, "Bearer ")
			if token == authHeader {
				http.Error(w, `{"error":"invalid authorization header"}`, http.StatusUnauthorized)
				return
			}

			claims, err := authSvc.ValidateAccessToken(token)
			if err != nil {
				http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusUnauthorized)
				return
			}

			// MustChangePassword check (P2-2): force password change except on exempt paths
			if claims.MustChangePassword && !passwordChangeExempt[r.URL.Path] {
				http.Error(w, `{"error":"password change required"}`, http.StatusForbidden)
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

// AuthUserCtxKeyForTest returns the context key used for storing the auth user.
// Exported only for use in tests that need to inject a user into the context.
func AuthUserCtxKeyForTest() any {
	return authUserCtxKey{}
}
