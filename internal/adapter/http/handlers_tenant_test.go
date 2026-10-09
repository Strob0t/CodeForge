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
		{ID: otherTenantID, Name: "Own", Slug: "own", ToolUID: &uid},
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
	// A tenant admin reads its own tenant (KI-174), without the tool UID.
	got = decodeOne(t, tenantRequest(t, http.MethodGet, "/api/v1/tenants/"+otherTenantID, "", tenantsTenantAdmin, store))
	if _, ok := got["tool_uid"]; ok {
		t.Fatalf("tenant admin sees tool_uid: %v", got)
	}

	w := tenantRequest(t, http.MethodGet, "/api/v1/tenants", "", tenantsPlatformAdmin, store)
	if w.Code != http.StatusOK {
		t.Fatalf("platform admin list: status = %d", w.Code)
	}
	var list []map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("platform admin list: %d tenants, want 3", len(list))
	}
	for _, item := range list {
		if _, ok := item["tool_uid"]; !ok {
			t.Fatalf("platform admin list: tool_uid missing in %v", item)
		}
	}
}

// KI-174: listing tenants and reading or updating another tenant are for
// platform admins; a tenant admin reads and updates its own tenant only,
// and gets 403 for any other ID, whether it exists or not.
func TestTenantRoutes_OtherTenantsArePlatformAdminOnly(t *testing.T) {
	newStore := func() *mockStore {
		return &mockStore{tenants: []tenant.Tenant{
			{ID: tenantctx.DefaultTenantID, Name: "Default", Slug: "default", Enabled: true},
			{ID: otherTenantID, Name: "Other", Slug: "other", Enabled: true},
		}}
	}
	for _, tc := range []struct {
		name   string
		method string
		path   string
		body   string
		u      *user.User
		want   int
	}{
		{"tenant admin lists", http.MethodGet, "/api/v1/tenants", "", tenantsTenantAdmin, http.StatusForbidden},
		{"platform admin lists", http.MethodGet, "/api/v1/tenants", "", tenantsPlatformAdmin, http.StatusOK},
		{"tenant admin reads own", http.MethodGet, "/api/v1/tenants/" + otherTenantID, "", tenantsTenantAdmin, http.StatusOK},
		{"tenant admin reads default", http.MethodGet, "/api/v1/tenants/" + tenantctx.DefaultTenantID, "", tenantsTenantAdmin, http.StatusForbidden},
		{"tenant admin reads unknown", http.MethodGet, "/api/v1/tenants/no-such", "", tenantsTenantAdmin, http.StatusForbidden},
		{"tenant admin updates own", http.MethodPut, "/api/v1/tenants/" + otherTenantID, `{"name":"Mine"}`, tenantsTenantAdmin, http.StatusOK},
		{"tenant admin updates default", http.MethodPut, "/api/v1/tenants/" + tenantctx.DefaultTenantID, `{"name":"x","enabled":false}`, tenantsTenantAdmin, http.StatusForbidden},
		{"platform admin reads other", http.MethodGet, "/api/v1/tenants/" + otherTenantID, "", tenantsPlatformAdmin, http.StatusOK},
		{"platform admin updates other", http.MethodPut, "/api/v1/tenants/" + otherTenantID, `{"name":"Renamed"}`, tenantsPlatformAdmin, http.StatusOK},
		{"platform admin disables default", http.MethodPut, "/api/v1/tenants/" + tenantctx.DefaultTenantID, `{"enabled":false}`, tenantsPlatformAdmin, http.StatusBadRequest},
		{"editor reads own", http.MethodGet, "/api/v1/tenants/" + otherTenantID, "", &user.User{ID: "ed", Role: user.RoleEditor, TenantID: otherTenantID}, http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := tenantRequest(t, tc.method, tc.path, tc.body, tc.u, newStore())
			if w.Code != tc.want {
				t.Fatalf("status = %d, want %d (body %s)", w.Code, tc.want, w.Body.String())
			}
		})
	}
}
