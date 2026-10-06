package http_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Strob0t/CodeForge/internal/crypto"
	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

func postAuthJSON(t *testing.T, r chi.Router, path string, body map[string]string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// lastEntry returns the newest audit entry and asserts its action.
func lastEntry(t *testing.T, audit *auditStoreMock, action string) database.AuditEntry {
	t.Helper()
	if len(audit.inserted) == 0 {
		t.Fatalf("no audit entry written, want %q", action)
	}
	e := audit.inserted[len(audit.inserted)-1]
	if e.Action != action || e.Resource != "auth" {
		t.Fatalf("last entry = %s %s, want %s auth (all: %+v)", e.Action, e.Resource, action, audit.inserted)
	}
	return e
}

// KI-172: the public auth routes have no user in the request context, so
// their handlers write the audit entries with the actor they resolved from
// the outcome: setup, login (success and failure, with the attempted email
// and never the password), refresh and password reset.
func TestAuthRoutes_AreAudited(t *testing.T) {
	const email = "admin@test.com"
	store := &mockStore{}
	audit := &auditStoreMock{}
	r := newAuditTestRouterWithStore(audit, nil, store)

	_, cookie := httpSetupAdmin(t, r, email)
	adminID := store.users[0].ID
	e := lastEntry(t, audit, "setup")
	if e.AdminID != adminID || e.AdminEmail == nil || *e.AdminEmail != email || e.ResourceID != adminID {
		t.Fatalf("setup entry = %+v, want the created admin %s", e, adminID)
	}
	if strings.Contains(string(e.Details), validPassword) {
		t.Fatalf("setup entry holds the password: %s", e.Details)
	}

	if w := postAuthJSON(t, r, "/api/v1/auth/login", map[string]string{"email": email, "password": validPassword}); w.Code != http.StatusOK {
		t.Fatalf("login: status %d (%s)", w.Code, w.Body.String())
	}
	e = lastEntry(t, audit, "login")
	if e.AdminID != adminID || e.ResourceID != adminID || !strings.Contains(string(e.Details), `"result":"success"`) {
		t.Fatalf("login entry = %+v, want success by %s", e, adminID)
	}

	if w := postAuthJSON(t, r, "/api/v1/auth/login", map[string]string{"email": "nobody@test.com", "password": "WrongPassword99"}); w.Code != http.StatusUnauthorized {
		t.Fatalf("failed login: status %d", w.Code)
	}
	e = lastEntry(t, audit, "login")
	if e.AdminID != middleware.AnonymousActorID || e.AdminEmail == nil || *e.AdminEmail != "nobody@test.com" || e.ResourceID != "" ||
		!strings.Contains(string(e.Details), `"result":"failure"`) {
		t.Fatalf("failed login entry = %+v, want the anonymous actor with the attempted email", e)
	}
	if strings.Contains(string(e.Details), "WrongPassword99") {
		t.Fatalf("failed login entry holds the password: %s", e.Details)
	}

	if w := postAuthJSON(t, r, "/api/v1/auth/refresh", nil, cookie); w.Code != http.StatusOK {
		t.Fatalf("refresh: status %d (%s)", w.Code, w.Body.String())
	}
	e = lastEntry(t, audit, "refresh")
	if e.AdminID != adminID || e.AdminEmail == nil || *e.AdminEmail != email {
		t.Fatalf("refresh entry = %+v, want %s", e, adminID)
	}

	if w := postAuthJSON(t, r, "/api/v1/auth/forgot-password", map[string]string{"email": email}); w.Code != http.StatusOK {
		t.Fatalf("forgot password: status %d", w.Code)
	}
	e = lastEntry(t, audit, "forgot_password")
	if e.AdminID != middleware.AnonymousActorID || e.AdminEmail == nil || *e.AdminEmail != email {
		t.Fatalf("forgot password entry = %+v, want the anonymous actor with the email", e)
	}

	raw := "reset-token-raw-value"
	store.passwordResetTokens = append(store.passwordResetTokens, user.PasswordResetToken{
		ID: "prt-1", UserID: adminID, TokenHash: crypto.HashSHA256(raw), ExpiresAt: time.Now().Add(time.Hour),
	})
	if w := postAuthJSON(t, r, "/api/v1/auth/reset-password", map[string]string{"token": raw, "new_password": "AnotherPass123"}); w.Code != http.StatusOK {
		t.Fatalf("reset password: status %d (%s)", w.Code, w.Body.String())
	}
	e = lastEntry(t, audit, "reset_password")
	if e.AdminID != adminID || e.AdminEmail == nil || *e.AdminEmail != email || !strings.Contains(string(e.Details), `"result":"success"`) {
		t.Fatalf("reset password entry = %+v, want success by %s", e, adminID)
	}
	if strings.Contains(string(e.Details), "AnotherPass123") {
		t.Fatalf("reset password entry holds the password: %s", e.Details)
	}

	if w := postAuthJSON(t, r, "/api/v1/auth/reset-password", map[string]string{"token": "bogus", "new_password": "AnotherPass123"}); w.Code != http.StatusBadRequest {
		t.Fatalf("bad reset: status %d", w.Code)
	}
	e = lastEntry(t, audit, "reset_password")
	if e.AdminID != middleware.AnonymousActorID || !strings.Contains(string(e.Details), `"result":"failure"`) {
		t.Fatalf("failed reset entry = %+v, want a failure by the anonymous actor", e)
	}
}

// A login with an invalid body is refused before any credential is read:
// nothing is audited, as there is neither an actor nor an attempted email.
func TestAuthRoutes_InvalidLoginBodyIsNotAudited(t *testing.T) {
	audit := &auditStoreMock{}
	r := newAuditTestRouterWithStore(audit, nil, &mockStore{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader([]byte("{invalid")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status %d", w.Code)
	}
	if len(audit.inserted) != 0 {
		t.Fatalf("entries = %+v, want none", audit.inserted)
	}
}
