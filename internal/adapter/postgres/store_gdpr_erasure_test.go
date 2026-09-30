package postgres_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	cfhttp "github.com/Strob0t/CodeForge/internal/adapter/http"
	"github.com/Strob0t/CodeForge/internal/adapter/postgres"
	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/channel"
	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/service"
)

// GDPR erasure of a user who gave consent, wrote in channels and acted as an
// admin, through each endpoint that deletes a user (DELETE /me/data,
// DELETE /users/{id}/data, DELETE /users/{id}): the erasure succeeds, the
// consent records stay as anonymized proof (no user, IP address or user
// agent), the user's channel messages stay without a sender id and with a
// placeholder name, and the user's audit entries lose email and IP address.
// Other users' rows are untouched.

func recordConsent(ctx context.Context, t *testing.T, store *postgres.Store, userID string) {
	t.Helper()
	if err := store.RecordConsent(ctx, &database.ConsentRecord{
		UserID: userID, PurposeID: "external_llm", PurposeVersion: 1, Granted: true,
		IPAddress: "203.0.113.7", UserAgent: "Mozilla/5.0 (test)",
	}); err != nil {
		t.Fatalf("RecordConsent: %v", err)
	}
}

type consentRow struct {
	userID    *string
	ip        *netip.Addr
	userAgent *string
	granted   bool
}

// consentRowsOf returns the consent rows recorded in the tenant, keyed by
// row id, so rows are found again after their user_id is gone.
func consentRowsOf(t *testing.T, pool *pgxpool.Pool, tenantID string) map[string]consentRow {
	t.Helper()
	rows, err := pool.Query(context.Background(),
		`SELECT id, user_id::text, ip_address, user_agent, granted FROM user_consents WHERE tenant_id = $1`, tenantID)
	if err != nil {
		t.Fatalf("query consents: %v", err)
	}
	defer rows.Close()
	out := map[string]consentRow{}
	for rows.Next() {
		var id string
		var r consentRow
		if err := rows.Scan(&id, &r.userID, &r.ip, &r.userAgent, &r.granted); err != nil {
			t.Fatalf("scan consent: %v", err)
		}
		out[id] = r
	}
	return out
}

func postMessage(ctx context.Context, t *testing.T, store *postgres.Store, channelID string, u *user.User) *channel.Message {
	t.Helper()
	msg, err := store.CreateChannelMessage(ctx, &channel.Message{
		ChannelID: channelID, SenderID: u.ID, SenderType: channel.SenderUser, SenderName: u.Name, Content: "hello from " + u.Name,
	})
	if err != nil {
		t.Fatalf("CreateChannelMessage: %v", err)
	}
	return msg
}

// erasureEndpoint deletes user u in ctx's tenant through one HTTP endpoint.
type erasureEndpoint func(ctx context.Context, store *postgres.Store, u *user.User) *httptest.ResponseRecorder

var erasureEndpoints = map[string]erasureEndpoint{
	"DELETE /me/data": func(ctx context.Context, store *postgres.Store, u *user.User) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/me/data", http.NoBody).
			WithContext(middleware.ContextWithTestUser(ctx, u))
		rec := httptest.NewRecorder()
		(&cfhttp.Handlers{GDPR: service.NewGDPRService(store)}).DeleteMyData(rec, req)
		return rec
	},
	"DELETE /users/{id}/data": func(ctx context.Context, store *postgres.Store, u *user.User) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		(&cfhttp.Handlers{GDPR: service.NewGDPRService(store)}).DeleteUserData(rec, userRequest(ctx, "/api/v1/users/"+u.ID+"/data", u.ID))
		return rec
	},
	"DELETE /users/{id}": func(ctx context.Context, store *postgres.Store, u *user.User) *httptest.ResponseRecorder {
		auth := service.NewAuthService(store, &config.Auth{JWTSecret: "test-secret-key-must-be-long-enough"})
		rec := httptest.NewRecorder()
		(&cfhttp.Handlers{Auth: auth}).DeleteUserHandler(rec, userRequest(ctx, "/api/v1/users/"+u.ID, u.ID))
		return rec
	},
}

// userRequest is an admin's DELETE request for the user id, routed like chi does.
func userRequest(ctx context.Context, path, id string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	return httptest.NewRequest(http.MethodDelete, path, http.NoBody).WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))
}

// adminAuditEntry records an audit entry with u as the acting admin and returns its id.
func adminAuditEntry(ctx context.Context, t *testing.T, store *postgres.Store, pool *pgxpool.Pool, u *user.User) string {
	t.Helper()
	marker := uuid.New().String()
	email := u.Email
	if err := store.InsertAuditEntry(ctx, &database.AuditEntry{
		AdminID: u.ID, AdminEmail: &email, Action: "user.update", Resource: "user", ResourceID: marker, IPAddress: "203.0.113.9",
	}); err != nil {
		t.Fatalf("InsertAuditEntry: %v", err)
	}
	var id string
	if err := pool.QueryRow(context.Background(), `SELECT id FROM audit_log WHERE resource_id = $1`, marker).Scan(&id); err != nil {
		t.Fatalf("find audit entry: %v", err)
	}
	return id
}

func TestGDPRErasure_EveryEndpoint(t *testing.T) {
	for name, erase := range erasureEndpoints {
		t.Run(name, func(t *testing.T) { testErasure(t, erase) })
	}
}

func testErasure(t *testing.T, erase erasureEndpoint) {
	t.Helper()
	store := setupStore(t)
	pool := retentionPool(t)
	tenantID := createTestTenant(t, store)
	ctx := ctxWithTenant(t, tenantID)

	erased := createAuditTestUser(t, store, tenantID)
	kept := createAuditTestUser(t, store, tenantID)
	recordConsent(ctx, t, store, erased.ID)
	recordConsent(ctx, t, store, erased.ID)
	recordConsent(ctx, t, store, kept.ID)
	erasedAudit := adminAuditEntry(ctx, t, store, pool, erased)
	keptAudit := adminAuditEntry(ctx, t, store, pool, kept)

	ch, err := store.CreateChannel(ctx, &channel.Channel{
		Name: "erasure-" + uuid.New().String()[:8], Type: channel.TypeBot, CreatedBy: erased.ID,
	})
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteChannel(ctx, ch.ID) })
	erasedMsg := postMessage(ctx, t, store, ch.ID, erased)
	keptMsg := postMessage(ctx, t, store, ch.ID, kept)
	consentsBefore := consentRowsOf(t, pool, tenantID)

	if rec := erase(ctx, store, erased); rec.Code != http.StatusNoContent {
		t.Fatalf("erasure = %d, want 204: %s", rec.Code, rec.Body.String())
	}

	if _, err := store.GetUser(ctx, erased.ID); err == nil {
		t.Fatal("erased user still exists")
	}

	t.Run("consent records stay as anonymized proof", func(t *testing.T) {
		after := consentRowsOf(t, pool, tenantID)
		if len(after) != len(consentsBefore) {
			t.Fatalf("%d consent rows after erasure, want %d (records are kept)", len(after), len(consentsBefore))
		}
		for id, before := range consentsBefore {
			got := after[id]
			if *before.userID == kept.ID {
				if got.userID == nil || *got.userID != kept.ID || got.ip == nil || got.userAgent == nil {
					t.Errorf("other user's consent %s changed: %+v", id, got)
				}
				continue
			}
			if got.userID != nil || got.ip != nil || got.userAgent != nil {
				t.Errorf("erased user's consent %s = user %v ip %v ua %v, want all removed", id, got.userID, got.ip, got.userAgent)
			}
			if !got.granted {
				t.Errorf("erased user's consent %s lost its decision", id)
			}
		}
	})

	t.Run("channel messages keep content without the sender", func(t *testing.T) {
		msgs, err := store.ListChannelMessages(ctx, ch.ID, "", 50)
		if err != nil {
			t.Fatalf("ListChannelMessages: %v", err)
		}
		byID := map[string]channel.Message{}
		for i := range msgs {
			byID[msgs[i].ID] = msgs[i]
		}
		got := byID[erasedMsg.ID]
		if got.SenderID != "" || got.SenderName != channel.ErasedSenderName || got.Content != erasedMsg.Content {
			t.Errorf("erased user's message = sender %q name %q content %q, want no sender, %q and the content kept",
				got.SenderID, got.SenderName, got.Content, channel.ErasedSenderName)
		}
		other := byID[keptMsg.ID]
		if other.SenderID != kept.ID || other.SenderName != kept.Name {
			t.Errorf("other user's message = sender %q name %q, want unchanged", other.SenderID, other.SenderName)
		}
		chAfter, err := store.GetChannel(ctx, ch.ID)
		if err != nil {
			t.Fatalf("GetChannel: %v", err)
		}
		if chAfter.CreatedBy != "" {
			t.Errorf("channel created_by = %q, want empty after erasure", chAfter.CreatedBy)
		}
	})

	t.Run("audit entries lose email and IP address", func(t *testing.T) {
		for id, wantPersonal := range map[string]bool{erasedAudit: false, keptAudit: true} {
			var email *string
			var ip *netip.Addr
			if err := pool.QueryRow(context.Background(),
				`SELECT admin_email, ip_address FROM audit_log WHERE id = $1`, id).Scan(&email, &ip); err != nil {
				t.Fatalf("audit entry %s must be kept: %v", id, err)
			}
			if (email != nil) != wantPersonal || (ip != nil) != wantPersonal {
				t.Errorf("audit entry %s = email %v ip %v, want personal data kept %v", id, email, ip, wantPersonal)
			}
		}
	})
}
