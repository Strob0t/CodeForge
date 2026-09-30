package postgres_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	cfhttp "github.com/Strob0t/CodeForge/internal/adapter/http"
	"github.com/Strob0t/CodeForge/internal/adapter/postgres"
	"github.com/Strob0t/CodeForge/internal/domain/channel"
	"github.com/Strob0t/CodeForge/internal/domain/user"
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/service"
)

// GDPR erasure (DELETE /api/v1/me/data) of a user who gave consent and wrote
// in channels: the erasure succeeds, the consent records stay as anonymized
// proof (no user, IP address or user agent), and the user's channel messages
// stay without a sender id and with a placeholder name. Other users' rows are
// untouched.

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

func TestGDPRErasure_ConsentsAndChannelMessages(t *testing.T) {
	store := setupStore(t)
	pool := retentionPool(t)
	tenantID := createTestTenant(t, store)
	ctx := ctxWithTenant(t, tenantID)

	erased := createAuditTestUser(t, store, tenantID)
	kept := createAuditTestUser(t, store, tenantID)
	recordConsent(ctx, t, store, erased.ID)
	recordConsent(ctx, t, store, erased.ID)
	recordConsent(ctx, t, store, kept.ID)

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

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/me/data", http.NoBody).
		WithContext(middleware.ContextWithTestUser(ctx, erased))
	rec := httptest.NewRecorder()
	(&cfhttp.Handlers{GDPR: service.NewGDPRService(store)}).DeleteMyData(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE /me/data = %d, want 204: %s", rec.Code, rec.Body.String())
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
}
