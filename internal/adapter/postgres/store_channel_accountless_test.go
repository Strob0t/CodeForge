package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	cfhttp "github.com/Strob0t/CodeForge/internal/adapter/http"
	"github.com/Strob0t/CodeForge/internal/adapter/postgres"
	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/channel"
	"github.com/Strob0t/CodeForge/internal/middleware"
	"github.com/Strob0t/CodeForge/internal/service"
)

// noopBroadcaster drops every event.
type noopBroadcaster struct{}

func (noopBroadcaster) BroadcastEvent(context.Context, string, any) {}

// unreadOf returns the unread count of channelID in userID's channel list.
func unreadOf(ctx context.Context, t *testing.T, store *postgres.Store, channelID, userID string) int {
	t.Helper()
	list, err := store.ListChannels(ctx, "", userID)
	if err != nil {
		t.Fatalf("ListChannels: %v", err)
	}
	for i := range list {
		if list[i].ID == channelID {
			return list[i].UnreadCount
		}
	}
	t.Fatalf("channel %s not listed", channelID)
	return 0
}

// KI-89: channels.created_by and channel_messages.sender_id reference users,
// and a caller without an account row (the default user while auth is
// disabled, the internal service key user) has none, so creating a channel or
// posting a message as that caller failed on the foreign key. Such a caller
// is recorded by kind (sender_type) and name, without a user ID; a caller
// with an account stays linked. Tenant scoping and the read state of the
// users with an account keep working.
func TestStore_ChannelWrites_AccountlessCaller(t *testing.T) {
	store := setupStore(t)
	tenant := createTestTenant(t, store)
	ctx := ctxWithTenant(t, tenant)
	otherCtx := ctxWithTenant(t, createTestTenant(t, store))
	bob := createChannelTestUser(t, store, tenant)

	for _, id := range accountlessUserIDs {
		t.Run(id, func(t *testing.T) {
			created, err := store.CreateChannel(ctx, &channel.Channel{
				Name: "accountless-" + uuid.NewString()[:8], Type: channel.TypeBot, CreatedBy: id,
			})
			if err != nil {
				t.Errorf("CreateChannel as an accountless caller: %v", err)
			} else {
				t.Cleanup(func() { _ = store.DeleteChannel(ctx, created.ID) })
				if created.CreatedBy != "" {
					t.Errorf("created_by = %q, want none (no account)", created.CreatedBy)
				}
			}

			ch := createStateTestChannel(ctx, t, store)
			msg, err := store.CreateChannelMessage(ctx, &channel.Message{
				ChannelID: ch.ID, SenderID: id, SenderType: channel.SenderUser, SenderName: "Admin", Content: "hello",
			})
			if err != nil {
				t.Fatalf("CreateChannelMessage as an accountless caller: %v", err)
			}
			if msg.SenderID != "" || msg.SenderType != channel.SenderUser || msg.SenderName != "Admin" {
				t.Errorf("message sender = %q/%s/%q, want no ID, user, Admin", msg.SenderID, msg.SenderType, msg.SenderName)
			}
			reply, err := store.CreateChannelMessage(ctx, &channel.Message{
				ChannelID: ch.ID, SenderID: id, SenderType: channel.SenderUser, SenderName: "Admin", Content: "re", ParentID: msg.ID,
			})
			if err != nil || reply.ParentID != msg.ID {
				t.Fatalf("thread reply as an accountless caller = %+v, %v", reply, err)
			}

			// Another tenant still cannot post into the channel.
			if _, err := store.CreateChannelMessage(otherCtx, &channel.Message{
				ChannelID: ch.ID, SenderID: id, SenderType: channel.SenderUser, SenderName: "Admin", Content: "x",
			}); err == nil {
				t.Fatal("another tenant posted into the channel")
			}

			listed, err := store.ListChannelMessages(ctx, ch.ID, "", 10)
			if err != nil || len(listed) != 2 {
				t.Fatalf("ListChannelMessages = %d messages, %v; want 2", len(listed), err)
			}
			for i := range listed {
				if listed[i].SenderID != "" || listed[i].SenderName != "Admin" {
					t.Errorf("listed sender = %q/%q, want no ID, Admin", listed[i].SenderID, listed[i].SenderName)
				}
			}

			// The read state of a user with an account counts the
			// accountless caller's top-level message as unread, and a
			// read moves it; the accountless caller's own is not tracked.
			if got := unreadOf(ctx, t, store, ch.ID, bob); got != 1 {
				t.Errorf("bob's unread count = %d, want 1", got)
			}
			if _, err := store.MarkChannelRead(ctx, ch.ID, bob, msg.ID); err != nil {
				t.Fatalf("MarkChannelRead(bob): %v", err)
			}
			if got := unreadOf(ctx, t, store, ch.ID, bob); got != 0 {
				t.Errorf("bob's unread count after reading = %d, want 0", got)
			}
			if got := unreadOf(ctx, t, store, ch.ID, id); got != 0 {
				t.Errorf("accountless unread count = %d, want 0 (not tracked)", got)
			}
			if _, err := store.MarkChannelRead(ctx, ch.ID, id, msg.ID); !errors.Is(err, channel.ErrReadStateNotTracked) {
				t.Errorf("MarkChannelRead(accountless) = %v, want ErrReadStateNotTracked", err)
			}
		})
	}

	// A caller with an account stays linked.
	ch, err := store.CreateChannel(ctx, &channel.Channel{Name: "linked-" + uuid.NewString()[:8], Type: channel.TypeBot, CreatedBy: bob})
	if err != nil {
		t.Fatalf("CreateChannel(bob): %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteChannel(ctx, ch.ID) })
	if ch.CreatedBy != bob {
		t.Errorf("created_by = %q, want bob %q", ch.CreatedBy, bob)
	}
	if msg := postTestMessage(ctx, t, store, ch.ID, bob, "mine"); msg.SenderID != bob {
		t.Errorf("sender_id = %q, want bob %q", msg.SenderID, bob)
	}
}

// KI-89 through the API: with auth disabled (the all-zero default user) and
// with the internal service key, creating a channel and posting a message
// answered 500 (foreign key). The channel's creator and the message's sender
// are the caller, never a value from the request body.
func TestChannels_AccountlessCallersOnPostgres(t *testing.T) {
	store := setupStore(t)
	ctx := ctxWithTenant(t, middleware.DefaultTenantID)
	handlers := &cfhttp.Handlers{
		Channels: service.NewChannelService(store, noopBroadcaster{}),
		Limits:   &config.Limits{MaxRequestBodySize: 1 << 20},
	}
	const internalKey = "internal-service-key-for-the-test"

	for _, tc := range []struct {
		name       string
		authOn     bool
		apiKey     string
		senderName string
	}{
		{name: "auth disabled", senderName: "Admin"},
		{name: "internal service key", authOn: true, apiKey: internalKey, senderName: "Internal Service"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := chi.NewRouter()
			r.Use(middleware.Auth(nil, tc.authOn, internalKey), middleware.TenantID)
			cfhttp.MountRoutes(r, handlers)
			call := func(path, body string) *httptest.ResponseRecorder {
				t.Helper()
				req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				if tc.apiKey != "" {
					req.Header.Set("X-API-Key", tc.apiKey)
				}
				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)
				return w
			}

			spoofedCreator := uuid.NewString()
			w := call("/api/v1/channels", `{"name":"accountless-`+uuid.NewString()[:8]+`","type":"bot","created_by":"`+spoofedCreator+`"}`)
			if w.Code != http.StatusCreated {
				t.Fatalf("create channel: status %d: %s", w.Code, w.Body.String())
			}
			var ch channel.Channel
			if err := json.Unmarshal(w.Body.Bytes(), &ch); err != nil {
				t.Fatalf("decode channel: %v", err)
			}
			t.Cleanup(func() { _ = store.DeleteChannel(ctx, ch.ID) })
			if ch.CreatedBy != "" || ch.TenantID != middleware.DefaultTenantID {
				t.Errorf("channel created_by %q tenant %q, want none (the caller has no account) in the default tenant", ch.CreatedBy, ch.TenantID)
			}

			w = call("/api/v1/channels/"+ch.ID+"/messages", `{"content":"hello"}`)
			if w.Code != http.StatusCreated {
				t.Fatalf("post message: status %d: %s", w.Code, w.Body.String())
			}
			var msg channel.Message
			if err := json.Unmarshal(w.Body.Bytes(), &msg); err != nil {
				t.Fatalf("decode message: %v", err)
			}
			if msg.SenderType != channel.SenderUser || msg.SenderName != tc.senderName || msg.SenderID != "" {
				t.Errorf("sender = %s/%q/%q, want user/%q without an ID", msg.SenderType, msg.SenderName, msg.SenderID, tc.senderName)
			}
		})
	}
}
