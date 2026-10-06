package http_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

func batchDeleteAs(t *testing.T, r http.Handler, u *user.User, ids []string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string][]string{"ids": ids})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/batch/delete", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(middleware.ContextWithTestUser(req.Context(), u))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func decodeBatchResults(t *testing.T, w *httptest.ResponseRecorder) []struct {
	ID    string `json:"id"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
} {
	t.Helper()
	if w.Code != http.StatusOK {
		t.Fatalf("status %d (%s)", w.Code, w.Body.String())
	}
	var results []struct {
		ID    string `json:"id"`
		OK    bool   `json:"ok"`
		Error string `json:"error,omitempty"`
	}
	if err := json.NewDecoder(w.Body).Decode(&results); err != nil {
		t.Fatal(err)
	}
	return results
}

// KI-173: POST /projects/batch/delete deletes projects like DELETE
// /projects/{id}, so it is admin-only too.
func TestBatchDelete_IsAdminOnly(t *testing.T) {
	for _, tc := range []struct {
		role user.Role
		want int
	}{
		{user.RoleViewer, http.StatusForbidden},
		{user.RoleEditor, http.StatusForbidden},
		{user.RoleAdmin, http.StatusOK},
	} {
		t.Run(string(tc.role), func(t *testing.T) {
			store := &mockStore{projects: []project.Project{{ID: "p1", TenantID: tenantctx.DefaultTenantID}}}
			r := newTestRouterWithStore(store)
			u := &user.User{ID: "u-" + string(tc.role), Role: tc.role, TenantID: tenantctx.DefaultTenantID}
			w := batchDeleteAs(t, r, u, []string{"p1"})
			if w.Code != tc.want {
				t.Fatalf("status %d, want %d (%s)", w.Code, tc.want, w.Body.String())
			}
			if deleted := len(store.projects) == 0; deleted != (tc.want == http.StatusOK) {
				t.Fatalf("project deleted = %v", deleted)
			}
		})
	}
}

// Each project of the batch gets its own audit entry, duplicate IDs are
// deleted and audited once, and the answer has one result per unique ID.
func TestBatchDelete_AuditsEachProjectOnce(t *testing.T) {
	store := &mockStore{projects: []project.Project{
		{ID: "p1", TenantID: tenantctx.DefaultTenantID},
		{ID: "p2", TenantID: tenantctx.DefaultTenantID},
	}}
	audit := &auditStoreMock{}
	admin := &user.User{ID: "ad", Email: "ad@example.com", Role: user.RoleAdmin, TenantID: tenantctx.DefaultTenantID}
	r := newAuditTestRouterWithStore(audit, admin, store)

	results := decodeBatchResults(t, batchDeleteAs(t, r, admin, []string{"p1", "p2", "p1", "p2"}))
	if len(results) != 2 || results[0].ID != "p1" || results[1].ID != "p2" || !results[0].OK || !results[1].OK {
		t.Fatalf("results = %+v, want p1 and p2 once, both ok", results)
	}
	if len(store.projects) != 0 {
		t.Fatalf("projects left: %+v", store.projects)
	}
	if len(audit.inserted) != 2 {
		t.Fatalf("audit entries = %+v, want one per project", audit.inserted)
	}
	ids := map[string]bool{}
	for _, e := range audit.inserted {
		if e.Action != "delete" || e.Resource != "project" || e.AdminID != "ad" || string(e.Details) != `{"batch":"true"}` {
			t.Fatalf("entry = %+v", e)
		}
		ids[e.ResourceID] = true
	}
	if !ids["p1"] || !ids["p2"] {
		t.Fatalf("audited projects = %v", ids)
	}
}

// A project whose audit entry cannot be written is not deleted (fail
// closed, as the single delete's audit is written before the change).
func TestBatchDelete_RefusesAProjectWithoutItsAuditEntry(t *testing.T) {
	store := &mockStore{projects: []project.Project{{ID: "p1", TenantID: tenantctx.DefaultTenantID}}}
	audit := &auditStoreMock{insertErr: errors.New("connection refused")}
	admin := &user.User{ID: "ad", Role: user.RoleAdmin, TenantID: tenantctx.DefaultTenantID}
	r := newAuditTestRouterWithStore(audit, admin, store)

	results := decodeBatchResults(t, batchDeleteAs(t, r, admin, []string{"p1"}))
	if len(results) != 1 || results[0].OK || results[0].Error != "audit log unavailable" {
		t.Fatalf("results = %+v, want p1 refused as audit log unavailable", results)
	}
	if len(store.projects) != 1 {
		t.Fatal("the project was deleted without its audit entry")
	}
}
