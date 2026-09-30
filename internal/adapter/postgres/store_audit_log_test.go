package postgres_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	cfhttp "github.com/Strob0t/CodeForge/internal/adapter/http"
	"github.com/Strob0t/CodeForge/internal/adapter/postgres"
	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/service"
)

// KI-53: GDPR erasure (GDPRService.DeleteUserData) nulls audit_log.admin_email
// and audit_log.ip_address (migration 089) and keeps the entries. Listing the
// audit log afterwards must work, and the erased admin's email reads as JSON
// null.

func createAuditTestUser(t *testing.T, store *postgres.Store, tenantID string) *user.User {
	t.Helper()
	u := &user.User{
		ID:           uuid.New().String(),
		Email:        "audit-" + uuid.New().String()[:8] + "@example.com",
		Name:         "Audit Test User",
		PasswordHash: "$2a$10$dummyhashforintegrationtest000000000000000000000000",
		Role:         user.RoleAdmin,
		TenantID:     tenantID,
		Enabled:      true,
	}
	if err := store.CreateUser(context.Background(), u); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteUser(context.Background(), u.ID) })
	return u
}

func insertAuditEntryFor(ctx context.Context, t *testing.T, store *postgres.Store, u *user.User, ip string) {
	t.Helper()
	email := u.Email
	if err := store.InsertAuditEntry(ctx, &database.AuditEntry{
		AdminID:    u.ID,
		AdminEmail: &email,
		Action:     "user.update",
		Resource:   "user",
		ResourceID: u.ID,
		IPAddress:  ip,
	}); err != nil {
		t.Fatalf("InsertAuditEntry: %v", err)
	}
}

// auditJSONByAdmin lists the audit log through the HTTP handler and returns
// the JSON objects keyed by admin_id.
func auditJSONByAdmin(ctx context.Context, t *testing.T, store *postgres.Store) map[string]map[string]any {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit-logs?limit=500", http.NoBody).WithContext(ctx)
	rec := httptest.NewRecorder()
	cfhttp.ListAuditLogs(store).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/audit-logs = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var entries []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &entries); err != nil {
		t.Fatalf("decode audit log: %v", err)
	}
	byAdmin := make(map[string]map[string]any, len(entries))
	for _, e := range entries {
		id, _ := e["admin_id"].(string)
		byAdmin[id] = e
	}
	return byAdmin
}

func TestStore_AuditLogListsEntriesOfErasedUsers(t *testing.T) {
	store := setupStore(t)
	tenantID := createTestTenant(t, store)
	ctx := ctxWithTenant(t, tenantID)

	erased := createAuditTestUser(t, store, tenantID)
	kept := createAuditTestUser(t, store, tenantID)
	insertAuditEntryFor(ctx, t, store, erased, "203.0.113.7")
	insertAuditEntryFor(ctx, t, store, kept, "198.51.100.9")

	if err := service.NewGDPRService(store).DeleteUserData(ctx, erased.ID); err != nil {
		t.Fatalf("DeleteUserData: %v", err)
	}

	t.Run("HTTP listing returns 200 with a null email", func(t *testing.T) {
		byAdmin := auditJSONByAdmin(ctx, t, store)

		got, ok := byAdmin[erased.ID]
		if !ok {
			t.Fatalf("erased admin's entry is missing from the audit log (the entry must be kept)")
		}
		email, present := got["admin_email"]
		if !present || email != nil {
			t.Fatalf("erased admin_email = %#v (present %v), want JSON null", email, present)
		}
		if ip, has := got["ip_address"]; has {
			t.Fatalf("erased ip_address = %#v, want it omitted", ip)
		}

		other := byAdmin[kept.ID]
		if other["admin_email"] != kept.Email {
			t.Fatalf("other admin_email = %#v, want %q (erasure must touch only the erased user)", other["admin_email"], kept.Email)
		}
		if other["ip_address"] != "198.51.100.9" {
			t.Fatalf("other ip_address = %#v, want 198.51.100.9", other["ip_address"])
		}
	})

	t.Run("store listings scan the nullable email", func(t *testing.T) {
		all, err := store.ListAuditEntries(ctx, "user.update", 500, 0)
		if err != nil {
			t.Fatalf("ListAuditEntries: %v", err)
		}
		if len(all) != 2 {
			t.Fatalf("ListAuditEntries returned %d entries in the fresh tenant, want 2", len(all))
		}

		trail, err := store.ListAuditEntriesByAdmin(ctx, erased.ID, 10)
		if err != nil {
			t.Fatalf("ListAuditEntriesByAdmin: %v", err)
		}
		if len(trail) != 1 || trail[0].AdminEmail != nil || trail[0].IPAddress != "" {
			t.Fatalf("erased admin's trail = %+v, want one entry without email and IP", trail)
		}
	})
}
