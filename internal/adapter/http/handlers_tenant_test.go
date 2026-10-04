package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	cfhttp "github.com/Strob0t/CodeForge/internal/adapter/http"
	"github.com/Strob0t/CodeForge/internal/domain/tenant"
	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/service"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

const otherTenantID = "11111111-2222-3333-4444-555555555555"

var (
	tenantsPlatformAdmin = &user.User{ID: "pa", Role: user.RoleAdmin, TenantID: tenantctx.DefaultTenantID}
	tenantsTenantAdmin   = &user.User{ID: "ta", Role: user.RoleAdmin, TenantID: otherTenantID}
)

func tenantRequest(t *testing.T, method, path, body string, u *user.User, store *mockStore) *httptest.ResponseRecorder {
	t.Helper()
	r := newTestRouterWithStore(store)
	req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(middleware.ContextWithTestUser(req.Context(), u))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

type exhaustedToolUIDs struct{}

func (exhaustedToolUIDs) AllocateToolUID(context.Context, string) (int, error) {
	return 0, fmt.Errorf("allocate: %w", tenant.ErrToolUIDRangeExhausted)
}
func (exhaustedToolUIDs) AdvanceToolUIDSequence(context.Context, int) (bool, error) {
	return false, nil
}

// TestStartRunWithoutToolUIDsIs503: with every tool UID taken, tool work of
// a tenant without one is refused as unavailable (KI-96), and no run exists.
func TestStartRunWithoutToolUIDsIs503(t *testing.T) {
	store := newExecModeRunStore(nil)
	r := newTestRouterWithLLM(store, service.NewPolicyService("headless-safe-sandbox", nil), "http://localhost:4000",
		func(h *cfhttp.Handlers) { h.Runtime.SetToolUIDs(service.NewToolUIDService(exhaustedToolUIDs{}, true)) })
	body := `{"task_id":"task-1","agent_id":"agent-1","project_id":"proj-1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/runs", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 (body %s)", w.Code, w.Body.String())
	}
	if msg := decodeErrorMessage(t, w); !strings.Contains(msg, "tool UID range exhausted") {
		t.Fatalf("error = %q", msg)
	}
	if len(store.runs) != 0 {
		t.Fatalf("runs created: %d", len(store.runs))
	}
}

// TestCreateTenantIsPlatformAdminOnly: tool UIDs are a deployment-wide
// resource (KI-96), so only platform admins create tenants.
func TestCreateTenantIsPlatformAdminOnly(t *testing.T) {
	body := `{"name":"Acme","slug":"acme"}`
	if w := tenantRequest(t, http.MethodPost, "/api/v1/tenants", body, tenantsTenantAdmin, &mockStore{}); w.Code != http.StatusForbidden {
		t.Fatalf("tenant admin: status = %d, want 403 (body %s)", w.Code, w.Body.String())
	}
	if w := tenantRequest(t, http.MethodPost, "/api/v1/tenants", body, tenantsPlatformAdmin, &mockStore{}); w.Code != http.StatusCreated {
		t.Fatalf("platform admin: status = %d, want 201 (body %s)", w.Code, w.Body.String())
	}
}

// TestTenantToolUIDOnlyForPlatformAdmins: tool_uid (a number or null) is in
// the tenant JSON for platform admins only.
func TestTenantToolUIDOnlyForPlatformAdmins(t *testing.T) {
	uid := 20007
	store := &mockStore{tenants: []tenant.Tenant{
		{ID: "t1", Name: "With", Slug: "with", ToolUID: &uid},
		{ID: "t2", Name: "Without", Slug: "without"},
	}}

	decodeOne := func(t *testing.T, w *httptest.ResponseRecorder) map[string]json.RawMessage {
		t.Helper()
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d (body %s)", w.Code, w.Body.String())
		}
		var got map[string]json.RawMessage
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		return got
	}

	got := decodeOne(t, tenantRequest(t, http.MethodGet, "/api/v1/tenants/t1", "", tenantsPlatformAdmin, store))
	if string(got["tool_uid"]) != "20007" {
		t.Fatalf("platform admin: tool_uid = %s, want 20007", got["tool_uid"])
	}
	got = decodeOne(t, tenantRequest(t, http.MethodGet, "/api/v1/tenants/t2", "", tenantsPlatformAdmin, store))
	if raw, ok := got["tool_uid"]; !ok || string(raw) != "null" {
		t.Fatalf("platform admin, no uid yet: tool_uid = %s (present %v), want null", raw, ok)
	}
	got = decodeOne(t, tenantRequest(t, http.MethodGet, "/api/v1/tenants/t1", "", tenantsTenantAdmin, store))
	if _, ok := got["tool_uid"]; ok {
		t.Fatalf("tenant admin sees tool_uid: %v", got)
	}

	for _, tc := range []struct {
		name    string
		u       *user.User
		present bool
	}{{"platform admin", tenantsPlatformAdmin, true}, {"tenant admin", tenantsTenantAdmin, false}} {
		w := tenantRequest(t, http.MethodGet, "/api/v1/tenants", "", tc.u, store)
		if w.Code != http.StatusOK {
			t.Fatalf("%s list: status = %d", tc.name, w.Code)
		}
		var list []map[string]json.RawMessage
		if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
			t.Fatal(err)
		}
		if len(list) != 2 {
			t.Fatalf("%s list: %d tenants, want 2", tc.name, len(list))
		}
		for _, item := range list {
			if _, ok := item["tool_uid"]; ok != tc.present {
				t.Fatalf("%s list: tool_uid present = %v, want %v", tc.name, ok, tc.present)
			}
		}
	}
}
