package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

const a2aTestTenant = "11111111-2222-3333-4444-555555555555"

// keysOf builds A2A keys of the default tenant.
func keysOf(keys ...string) []config.A2AAPIKey {
	out := make([]config.A2AAPIKey, 0, len(keys))
	for _, k := range keys {
		out = append(out, config.A2AAPIKey{Key: k, TenantID: tenantctx.DefaultTenantID})
	}
	return out
}

func serveA2A(t *testing.T, keys []config.A2AAPIKey, authorization string, inner http.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/a2a", http.NoBody)
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	rr := httptest.NewRecorder()
	A2AAuth(keys)(inner).ServeHTTP(rr, req)
	return rr
}

func okHandler(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }

func TestA2AAuth_ValidKey(t *testing.T) {
	rr := serveA2A(t, keysOf("secret-key"), "Bearer secret-key", func(w http.ResponseWriter, r *http.Request) {
		if trust := A2ATrustFromContext(r.Context()); trust != A2ATrustPartial {
			t.Errorf("expected partial trust, got %s", trust)
		}
		w.WriteHeader(http.StatusOK)
	})
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestA2AAuth_MissingHeader(t *testing.T) {
	if rr := serveA2A(t, keysOf("secret-key"), "", okHandler); rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestA2AAuth_InvalidKey(t *testing.T) {
	if rr := serveA2A(t, keysOf("secret-key"), "Bearer wrong-key", okHandler); rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

// TestA2AAuth_NoKeysRefuses: without a2a.api_keys every A2A request is
// refused (KI-15, fail closed). It used to pass through as "untrusted" and
// create tasks in whatever tenant the caller's X-Tenant-ID header named.
func TestA2AAuth_NoKeysRefuses(t *testing.T) {
	for _, auth := range []string{"", "Bearer anything"} {
		rr := serveA2A(t, nil, auth, func(http.ResponseWriter, *http.Request) {
			t.Error("a request reached the A2A handler without a configured key")
		})
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("Authorization %q: expected 401, got %d", auth, rr.Code)
		}
	}
}

func TestA2AAuth_MalformedHeader(t *testing.T) {
	if rr := serveA2A(t, keysOf("secret-key"), "Basic secret-key", okHandler); rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestA2AAuth_MultipleValidKeys(t *testing.T) {
	if rr := serveA2A(t, keysOf("key-1", "key-2", "key-3"), "Bearer key-2", okHandler); rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestA2AAuth_EmptyToken(t *testing.T) {
	if rr := serveA2A(t, keysOf("secret-key"), "Bearer ", okHandler); rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for empty bearer token, got %d", rr.Code)
	}
}

// TestA2AAuth_KeySetsTheTenant (KI-15): an A2A caller acts in the tenant
// its key is mapped to (the default tenant for a plain key), whatever
// X-Tenant-ID header it sends; the tenant middleware ran first and took the
// header for an unauthenticated request.
func TestA2AAuth_KeySetsTheTenant(t *testing.T) {
	keys := []config.A2AAPIKey{
		{Key: "tenant-key", TenantID: a2aTestTenant},
		{Key: "plain-key", TenantID: tenantctx.DefaultTenantID},
	}
	for key, want := range map[string]string{"tenant-key": a2aTestTenant, "plain-key": tenantctx.DefaultTenantID} {
		var got string
		handler := TenantID(A2AAuth(keys)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got = tenantctx.FromContext(r.Context())
			w.WriteHeader(http.StatusOK)
		})))
		req := httptest.NewRequest("POST", "/a2a", http.NoBody)
		req.Header.Set("Authorization", "Bearer "+key)
		req.Header.Set("X-Tenant-ID", "99999999-9999-9999-9999-999999999999")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK || got != want {
			t.Errorf("key %s: status %d, tenant %q; want 200 in tenant %q", key, rr.Code, got, want)
		}
	}
}

func TestMatchA2AKey(t *testing.T) {
	keys := []config.A2AAPIKey{
		{Key: "key-alpha", TenantID: "t-a"},
		{Key: "key-beta", TenantID: "t-b"},
	}
	if k, ok := matchA2AKey(keys, "key-beta"); !ok || k.TenantID != "t-b" {
		t.Errorf("key-beta = %+v, %v; want tenant t-b", k, ok)
	}
	for _, token := range []string{"key-delta", "", "key-alph"} {
		if _, ok := matchA2AKey(keys, token); ok {
			t.Errorf("%q matched", token)
		}
	}
	if _, ok := matchA2AKey(nil, "any"); ok {
		t.Error("nil keys matched")
	}
}
