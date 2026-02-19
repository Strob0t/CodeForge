package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/middleware"
)

func TestRequireRole_AdminAllowed(t *testing.T) {
	// Auth disabled injects admin user.
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := middleware.Auth(nil, false)(
		middleware.RequireRole(user.RoleAdmin)(inner),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", http.NoBody)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestRequireRole_NoUser_Returns401(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// No auth middleware, so no user in context.
	handler := middleware.RequireRole(user.RoleAdmin)(inner)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", http.NoBody)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestRequireRole_WrongRole_Returns403(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	viewerUser := &user.User{
		ID:       "viewer-1",
		Email:    "viewer@test.com",
		Role:     user.RoleViewer,
		TenantID: "tid-1",
		Enabled:  true,
	}

	// Inject user into context before RBAC check.
	injectUser := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), middleware.AuthUserCtxKeyForTest(), viewerUser)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}

	handler := injectUser(middleware.RequireRole(user.RoleAdmin)(inner))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", http.NoBody)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

func TestRequireRole_EditorAllowedForEditorRoute(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	editorUser := &user.User{
		ID:       "editor-1",
		Email:    "editor@test.com",
		Role:     user.RoleEditor,
		TenantID: "tid-1",
		Enabled:  true,
	}

	injectUser := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), middleware.AuthUserCtxKeyForTest(), editorUser)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}

	handler := injectUser(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)(inner))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", http.NoBody)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}
