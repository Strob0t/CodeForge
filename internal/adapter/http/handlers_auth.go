package http

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/middleware"
)

const refreshCookieName = "codeforge_refresh"

// Login handles POST /api/v1/auth/login
func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	var req user.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	tenantID := middleware.TenantIDFromContext(r.Context())
	resp, rawRefresh, err := h.Auth.Login(r.Context(), req, tenantID)
	if err != nil {
		slog.Debug("login failed", "email", req.Email, "error", err)
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	// Set refresh token as httpOnly cookie.
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    rawRefresh,
		Path:     "/api/v1/auth",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(7 * 24 * time.Hour / time.Second),
	})

	writeJSON(w, http.StatusOK, resp)
}

// Refresh handles POST /api/v1/auth/refresh
func (h *Handlers) Refresh(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(refreshCookieName)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "no refresh token")
		return
	}

	resp, newRawRefresh, err := h.Auth.RefreshTokens(r.Context(), cookie.Value)
	if err != nil {
		slog.Debug("token refresh failed", "error", err)
		// Clear invalid cookie.
		http.SetCookie(w, &http.Cookie{
			Name:     refreshCookieName,
			Value:    "",
			Path:     "/api/v1/auth",
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteStrictMode,
			MaxAge:   -1,
		})
		writeError(w, http.StatusUnauthorized, "invalid or expired refresh token")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    newRawRefresh,
		Path:     "/api/v1/auth",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(7 * 24 * time.Hour / time.Second),
	})

	writeJSON(w, http.StatusOK, resp)
}

// Logout handles POST /api/v1/auth/logout
func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	u := middleware.UserFromContext(r.Context())
	if u == nil {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	// Extract JTI from the current access token for revocation.
	var jti string
	var tokenExpiry time.Time
	if authHeader := r.Header.Get("Authorization"); authHeader != "" {
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token != authHeader {
			if claims, err := h.Auth.ValidateAccessToken(token); err == nil {
				jti = claims.JTI
				tokenExpiry = time.Unix(claims.Expiry, 0)
			}
		}
	}

	if err := h.Auth.Logout(r.Context(), u.ID, jti, tokenExpiry); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Clear refresh cookie.
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    "",
		Path:     "/api/v1/auth",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}

// ChangePassword handles POST /api/v1/auth/change-password
func (h *Handlers) ChangePassword(w http.ResponseWriter, r *http.Request) {
	u := middleware.UserFromContext(r.Context())
	if u == nil {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req user.ChangePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.Auth.ChangePassword(r.Context(), u.ID, req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "password_changed"})
}

// GetCurrentUser handles GET /api/v1/auth/me
func (h *Handlers) GetCurrentUser(w http.ResponseWriter, r *http.Request) {
	u := middleware.UserFromContext(r.Context())
	if u == nil {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	writeJSON(w, http.StatusOK, u)
}

// CreateAPIKeyHandler handles POST /api/v1/auth/api-keys
func (h *Handlers) CreateAPIKeyHandler(w http.ResponseWriter, r *http.Request) {
	u := middleware.UserFromContext(r.Context())
	if u == nil {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req user.CreateAPIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	resp, err := h.Auth.CreateAPIKey(r.Context(), u.ID, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, resp)
}

// ListAPIKeysHandler handles GET /api/v1/auth/api-keys
func (h *Handlers) ListAPIKeysHandler(w http.ResponseWriter, r *http.Request) {
	u := middleware.UserFromContext(r.Context())
	if u == nil {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	keys, err := h.Auth.ListAPIKeys(r.Context(), u.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if keys == nil {
		keys = []user.APIKey{}
	}

	writeJSON(w, http.StatusOK, keys)
}

// DeleteAPIKeyHandler handles DELETE /api/v1/auth/api-keys/{id}
func (h *Handlers) DeleteAPIKeyHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.Auth.DeleteAPIKey(r.Context(), id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListUsersHandler handles GET /api/v1/users (admin only)
func (h *Handlers) ListUsersHandler(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.TenantIDFromContext(r.Context())
	users, err := h.Auth.ListUsers(r.Context(), tenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if users == nil {
		users = []user.User{}
	}
	writeJSON(w, http.StatusOK, users)
}

// CreateUserHandler handles POST /api/v1/users (admin only)
func (h *Handlers) CreateUserHandler(w http.ResponseWriter, r *http.Request) {
	var req user.CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	tenantID := middleware.TenantIDFromContext(r.Context())
	req.TenantID = tenantID

	u, err := h.Auth.Register(r.Context(), &req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, u)
}

// UpdateUserHandler handles PUT /api/v1/users/{id} (admin only)
func (h *Handlers) UpdateUserHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req user.UpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	u, err := h.Auth.UpdateUser(r.Context(), id, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, u)
}

// DeleteUserHandler handles DELETE /api/v1/users/{id} (admin only)
func (h *Handlers) DeleteUserHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.Auth.DeleteUser(r.Context(), id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
