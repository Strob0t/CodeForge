package middleware_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/tenant"
	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

const gateOtherTenant = "11111111-2222-3333-4444-555555555555"

// fakeTenantChecker answers ValidateExists per tenant and counts the calls.
type fakeTenantChecker struct {
	mu      sync.Mutex
	answers map[string]error
	calls   int
}

func (f *fakeTenantChecker) ValidateExists(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.answers[id]
}

func serveGated(gate func(http.Handler) http.Handler, u *user.User) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", http.NoBody)
	if u != nil {
		req = req.WithContext(middleware.ContextWithTestUser(req.Context(), u))
	}
	rec := httptest.NewRecorder()
	gate(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })).ServeHTTP(rec, req)
	return rec
}

// KI-174: a disabled (or deleted) tenant is refused on every authenticated
// request with a clear error; a request without a user (public route) and
// an enabled tenant pass.
func TestEnabledTenant_RefusesDisabledAndUnknownTenants(t *testing.T) {
	check := &fakeTenantChecker{answers: map[string]error{
		gateOtherTenant:                        tenant.ErrDisabled,
		"33333333-3333-3333-3333-333333333333": domain.ErrNotFound,
	}}
	gate := middleware.EnabledTenant(middleware.NewTenantGate(check, time.Minute))
	for _, tc := range []struct {
		name string
		u    *user.User
		want int
		body string
	}{
		{"no user", nil, http.StatusNoContent, ""},
		{"enabled", &user.User{ID: "u1", Role: user.RoleViewer, TenantID: tenantctx.DefaultTenantID}, http.StatusNoContent, ""},
		{"disabled", &user.User{ID: "u2", Role: user.RoleAdmin, TenantID: gateOtherTenant}, http.StatusForbidden, "tenant is disabled"},
		{"unknown", &user.User{ID: "u3", Role: user.RoleAdmin, TenantID: "33333333-3333-3333-3333-333333333333"}, http.StatusForbidden, "tenant not found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := serveGated(gate, tc.u)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, tc.want, rec.Body.String())
			}
			if tc.body != "" && !strings.Contains(rec.Body.String(), tc.body) {
				t.Fatalf("body = %s, want %q", rec.Body.String(), tc.body)
			}
		})
	}
}

// A verdict is cached for the TTL, so the common case costs no query; the
// cache holds both verdicts and expires.
func TestEnabledTenant_CachesVerdicts(t *testing.T) {
	check := &fakeTenantChecker{answers: map[string]error{gateOtherTenant: tenant.ErrDisabled}}
	gate := middleware.EnabledTenant(middleware.NewTenantGate(check, 30*time.Millisecond))
	enabled := &user.User{ID: "u1", Role: user.RoleViewer, TenantID: tenantctx.DefaultTenantID}
	disabled := &user.User{ID: "u2", Role: user.RoleViewer, TenantID: gateOtherTenant}

	for range 5 {
		if rec := serveGated(gate, enabled); rec.Code != http.StatusNoContent {
			t.Fatalf("enabled: status %d", rec.Code)
		}
		if rec := serveGated(gate, disabled); rec.Code != http.StatusForbidden {
			t.Fatalf("disabled: status %d", rec.Code)
		}
	}
	if check.calls != 2 {
		t.Fatalf("checker called %d times within the TTL, want 2 (one per tenant)", check.calls)
	}

	time.Sleep(50 * time.Millisecond)
	check.answers[gateOtherTenant] = nil
	if rec := serveGated(gate, disabled); rec.Code != http.StatusNoContent {
		t.Fatalf("re-enabled tenant after the TTL: status %d", rec.Code)
	}
	if check.calls != 3 {
		t.Fatalf("checker called %d times, want 3 after the TTL", check.calls)
	}
}

// A store failure is answered 503 and not cached: the next request asks
// again.
func TestEnabledTenant_StoreErrorIs503AndNotCached(t *testing.T) {
	check := &fakeTenantChecker{answers: map[string]error{tenantctx.DefaultTenantID: errors.New("connection refused")}}
	gate := middleware.EnabledTenant(middleware.NewTenantGate(check, time.Minute))
	u := &user.User{ID: "u1", Role: user.RoleAdmin, TenantID: tenantctx.DefaultTenantID}
	for range 2 {
		if rec := serveGated(gate, u); rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("status %d, want 503 (%s)", rec.Code, rec.Body.String())
		}
	}
	if check.calls != 2 {
		t.Fatalf("checker called %d times, want 2 (errors are not cached)", check.calls)
	}
}

// KI-174: a route whose {id} names a tenant is open to that tenant's own
// users and to platform admins; any other tenant's ID is forbidden, whether
// it exists or not.
func TestRequireOwnTenant(t *testing.T) {
	platformAdmin := &user.User{ID: "pa", Role: user.RoleAdmin, TenantID: tenantctx.DefaultTenantID}
	tenantAdmin := &user.User{ID: "ta", Role: user.RoleAdmin, TenantID: gateOtherTenant}
	r := chi.NewRouter()
	r.With(middleware.RequireOwnTenant("id")).Get("/tenants/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	for _, tc := range []struct {
		name string
		u    *user.User
		id   string
		want int
	}{
		{"no user", nil, gateOtherTenant, http.StatusUnauthorized},
		{"tenant admin, own", tenantAdmin, gateOtherTenant, http.StatusNoContent},
		{"tenant admin, default tenant", tenantAdmin, tenantctx.DefaultTenantID, http.StatusForbidden},
		{"tenant admin, unknown", tenantAdmin, "no-such-tenant", http.StatusForbidden},
		{"platform admin, own", platformAdmin, tenantctx.DefaultTenantID, http.StatusNoContent},
		{"platform admin, other", platformAdmin, gateOtherTenant, http.StatusNoContent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/tenants/"+tc.id, http.NoBody)
			if tc.u != nil {
				req = req.WithContext(middleware.ContextWithTestUser(req.Context(), tc.u))
			}
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}
