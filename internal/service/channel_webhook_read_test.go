package service_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/channel"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/service"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

const webhookTenant = "aaaaaaaa-0000-4000-8000-000000000001"

// webhookStore keeps one channel's webhook key hash and read positions.
type webhookStore struct {
	chMockStore
	tenant string
	hash   []byte
	reads  []channel.ReadState
}

func (m *webhookStore) SetChannelWebhookKeyHash(_ context.Context, channelID string, hash []byte) error {
	if channelID != "ch-1" {
		return domain.ErrNotFound
	}
	m.hash = hash
	return nil
}

func (m *webhookStore) GetChannelWebhookKeyHash(_ context.Context, channelID string) (tenantID string, hash []byte, err error) {
	if channelID != "ch-1" {
		return "", nil, domain.ErrNotFound
	}
	return m.tenant, m.hash, nil
}

func (m *webhookStore) MarkChannelRead(_ context.Context, channelID, userID, messageID string) (*channel.ReadState, error) {
	if channelID != "ch-1" || messageID == "" {
		return nil, domain.ErrNotFound
	}
	rs := channel.ReadState{ChannelID: channelID, UserID: userID, LastReadMessageID: messageID, LastReadAt: time.Unix(100, 0).UTC()}
	m.reads = append(m.reads, rs)
	return &rs, nil
}

// KI-73: the webhook key is shown once; only its SHA-256 is stored, and a
// webhook call is accepted with the right key only, in the channel's tenant.
func TestChannelService_WebhookKey(t *testing.T) {
	store := &webhookStore{tenant: webhookTenant}
	svc := service.NewChannelService(store, &runtimeMockBroadcaster{})
	ctx := tenantctx.WithTenant(context.Background(), webhookTenant)

	key, err := svc.RegenerateWebhookKey(ctx, "ch-1")
	if err != nil {
		t.Fatalf("RegenerateWebhookKey: %v", err)
	}
	if len(key) != 64 {
		t.Fatalf("key length = %d, want 64", len(key))
	}
	want := sha256.Sum256([]byte(key))
	if !bytes.Equal(store.hash, want[:]) {
		t.Fatal("the stored value is not the key's SHA-256")
	}

	authorized, err := svc.AuthorizeWebhook(context.Background(), "ch-1", key)
	if err != nil {
		t.Fatalf("AuthorizeWebhook with the key: %v", err)
	}
	if got, ok := tenantctx.Explicit(authorized); !ok || got != webhookTenant {
		t.Fatalf("webhook context tenant = %q, want the channel's tenant", got)
	}

	wrong := key[:63] + "0"
	if key[63] == '0' {
		wrong = key[:63] + "1"
	}
	for name, call := range map[string]struct{ channel, key string }{
		"wrong key":       {"ch-1", wrong},
		"empty key":       {"ch-1", ""},
		"unknown channel": {"ch-x", key},
	} {
		if _, err := svc.AuthorizeWebhook(context.Background(), call.channel, call.key); !errors.Is(err, service.ErrWebhookForbidden) {
			t.Errorf("%s: err = %v, want ErrWebhookForbidden", name, err)
		}
	}

	again, err := svc.RegenerateWebhookKey(ctx, "ch-1")
	if err != nil || again == key {
		t.Fatalf("regenerate: %q, %v", again, err)
	}
	if _, err := svc.AuthorizeWebhook(context.Background(), "ch-1", key); !errors.Is(err, service.ErrWebhookForbidden) {
		t.Fatal("the replaced key still works")
	}
}

func TestChannelService_WebhookWithoutKeyIsForbidden(t *testing.T) {
	svc := service.NewChannelService(&webhookStore{tenant: webhookTenant}, &runtimeMockBroadcaster{})
	if _, err := svc.AuthorizeWebhook(context.Background(), "ch-1", "anything"); !errors.Is(err, service.ErrWebhookForbidden) {
		t.Fatalf("err = %v, want ErrWebhookForbidden", err)
	}
}

// KI-73: marking a channel read stores the position and broadcasts
// channel.read to the tenant.
func TestChannelService_MarkRead_Broadcasts(t *testing.T) {
	store := &webhookStore{tenant: webhookTenant}
	bc := &runtimeMockBroadcaster{}
	svc := service.NewChannelService(store, bc)
	ctx := tenantctx.WithTenant(context.Background(), webhookTenant)

	state, err := svc.MarkRead(ctx, "ch-1", "user-1", "msg-2")
	if err != nil || state.LastReadMessageID != "msg-2" {
		t.Fatalf("MarkRead = %+v, %v", state, err)
	}
	events := bc.snapshot()
	if len(events) != 1 || events[0].EventType != event.EventChannelRead || events[0].Tenant != webhookTenant {
		t.Fatalf("events = %+v", events)
	}
	ev, ok := events[0].Data.(event.ChannelReadEvent)
	if !ok || ev.ChannelID != "ch-1" || ev.UserID != "user-1" || ev.MessageID != "msg-2" {
		t.Fatalf("event = %+v", events[0].Data)
	}

	if _, err := svc.MarkRead(ctx, "ch-1", "user-1", ""); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("empty message id: err = %v, want a validation error", err)
	}
	if _, err := svc.MarkRead(ctx, "ch-x", "user-1", "msg-2"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown channel: err = %v", err)
	}
	if len(bc.snapshot()) != 1 {
		t.Fatal("a failed mark was broadcast")
	}
}
