package postgres_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/Strob0t/CodeForge/internal/adapter/postgres"
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/channel"
	"github.com/Strob0t/CodeForge/internal/domain/user"
)

func createChannelTestUser(t *testing.T, store *postgres.Store, tenantID string) string {
	t.Helper()
	u := &user.User{
		ID:           uuid.New().String(),
		Email:        "channel-" + uuid.New().String()[:8] + "@example.com",
		Name:         "Channel Test User",
		PasswordHash: "$2a$10$dummyhashforintegrationtest000000000000000000000000",
		Role:         user.RoleEditor,
		TenantID:     tenantID,
		Enabled:      true,
	}
	if err := store.CreateUser(context.Background(), u); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	tenantCtx := ctxWithTenant(t, tenantID)
	t.Cleanup(func() { _ = store.DeleteUser(tenantCtx, u.ID) })
	return u.ID
}

func createStateTestChannel(ctx context.Context, t *testing.T, store *postgres.Store) *channel.Channel {
	t.Helper()
	ch, err := store.CreateChannel(ctx, &channel.Channel{Name: "state-" + uuid.New().String()[:8], Type: channel.TypeBot})
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteChannel(ctx, ch.ID) })
	return ch
}

func postTestMessage(ctx context.Context, t *testing.T, store *postgres.Store, channelID, senderID, content string) *channel.Message {
	t.Helper()
	msg, err := store.CreateChannelMessage(ctx, &channel.Message{
		ChannelID: channelID, SenderID: senderID, SenderType: channel.SenderUser, SenderName: "U", Content: content,
	})
	if err != nil {
		t.Fatalf("CreateChannelMessage: %v", err)
	}
	return msg
}

// KI-73: only the SHA-256 of a channel's webhook key is stored. The webhook
// caller has no tenant, so the hash is looked up by channel across tenants and
// returns the channel's tenant.
func TestStore_ChannelWebhookKeyHash(t *testing.T) {
	store := setupStore(t)
	tenantA := createTestTenant(t, store)
	tenantB := createTestTenant(t, store)
	ctxA, ctxB := ctxWithTenant(t, tenantA), ctxWithTenant(t, tenantB)
	ch := createStateTestChannel(ctxA, t, store)

	tenant, hash, err := store.GetChannelWebhookKeyHash(context.Background(), ch.ID)
	if err != nil || tenant != tenantA || hash != nil {
		t.Fatalf("before a key: tenant %q hash %v err %v", tenant, hash, err)
	}
	got, _ := store.GetChannel(ctxA, ch.ID)
	if got.HasWebhookKey {
		t.Fatal("has_webhook_key before a key was generated")
	}

	want := bytes.Repeat([]byte{7}, 32)
	if err := store.SetChannelWebhookKeyHash(ctxB, ch.ID, want); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("another tenant set the key: err = %v", err)
	}
	if err := store.SetChannelWebhookKeyHash(ctxA, ch.ID, want); err != nil {
		t.Fatalf("SetChannelWebhookKeyHash: %v", err)
	}
	tenant, hash, err = store.GetChannelWebhookKeyHash(context.Background(), ch.ID)
	if err != nil || tenant != tenantA || !bytes.Equal(hash, want) {
		t.Fatalf("after a key: tenant %q hash %v err %v", tenant, hash, err)
	}
	got, _ = store.GetChannel(ctxA, ch.ID)
	listed, _ := store.ListChannels(ctxA, "", "")
	if !got.HasWebhookKey || len(listed) == 0 || !listed[0].HasWebhookKey {
		t.Fatal("has_webhook_key not reported")
	}

	if _, _, err := store.GetChannelWebhookKeyHash(context.Background(), uuid.New().String()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown channel: err = %v", err)
	}
}

// KI-73: each user's read position per channel; it never moves backwards,
// and unread counts cover the top-level messages of others after it.
func TestStore_ChannelReadState(t *testing.T) {
	store := setupStore(t)
	tenantA := createTestTenant(t, store)
	tenantB := createTestTenant(t, store)
	ctxA, ctxB := ctxWithTenant(t, tenantA), ctxWithTenant(t, tenantB)
	alice := createChannelTestUser(t, store, tenantA)
	bob := createChannelTestUser(t, store, tenantA)
	ch := createStateTestChannel(ctxA, t, store)
	other := createStateTestChannel(ctxA, t, store)

	m1 := postTestMessage(ctxA, t, store, ch.ID, bob, "one")
	m2 := postTestMessage(ctxA, t, store, ch.ID, bob, "two")
	_ = postTestMessage(ctxA, t, store, ch.ID, alice, "mine")
	if _, err := store.CreateChannelMessage(ctxA, &channel.Message{
		ChannelID: ch.ID, SenderID: bob, SenderType: channel.SenderUser, SenderName: "B", Content: "reply", ParentID: m1.ID,
	}); err != nil {
		t.Fatal(err)
	}
	otherMsg := postTestMessage(ctxA, t, store, other.ID, bob, "elsewhere")

	unread := func() map[string]int {
		t.Helper()
		list, err := store.ListChannels(ctxA, "", alice)
		if err != nil {
			t.Fatal(err)
		}
		counts := map[string]int{}
		for i := range list {
			counts[list[i].ID] = list[i].UnreadCount
		}
		return counts
	}
	if got := unread(); got[ch.ID] != 2 || got[other.ID] != 1 {
		t.Fatalf("unread before reading = %v, want 2 in the channel (own and thread replies excluded) and 1 elsewhere", got)
	}

	state, err := store.MarkChannelRead(ctxA, ch.ID, alice, m2.ID)
	if err != nil {
		t.Fatalf("MarkChannelRead: %v", err)
	}
	if state.UserID != alice || state.LastReadMessageID != m2.ID || !state.LastReadAt.Equal(m2.CreatedAt) {
		t.Fatalf("state = %+v", state)
	}
	if got := unread(); got[ch.ID] != 0 {
		t.Fatalf("unread after reading = %v", got)
	}

	back, err := store.MarkChannelRead(ctxA, ch.ID, alice, m1.ID)
	if err != nil || back.LastReadMessageID != m2.ID {
		t.Fatalf("an older message moved the read position back: %+v, %v", back, err)
	}

	if _, err := store.MarkChannelRead(ctxA, ch.ID, alice, otherMsg.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("a message of another channel: err = %v", err)
	}
	if _, err := store.MarkChannelRead(ctxB, ch.ID, alice, m2.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("another tenant marked the channel read: err = %v", err)
	}

	states, err := store.ListChannelReadStates(ctxA, ch.ID)
	if err != nil || len(states) != 1 || states[0].UserID != alice {
		t.Fatalf("read states = %+v, %v", states, err)
	}
	if states, _ := store.ListChannelReadStates(ctxB, ch.ID); len(states) != 0 {
		t.Fatalf("another tenant sees read states: %+v", states)
	}
}
