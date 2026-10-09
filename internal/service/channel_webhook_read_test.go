package service_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/channel"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/service"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

const (
	webhookTenant  = "aaaaaaaa-0000-4000-8000-000000000001"
	webhookChannel = "c0ffee00-0000-4000-8000-000000000001"
)

// webhookStore keeps one channel's webhook key hash and read positions.
type webhookStore struct {
	chMockStore
	tenant  string
	hash    []byte
	reads   []channel.ReadState
	lookups int
}

func (m *webhookStore) SetChannelWebhookKeyHash(_ context.Context, channelID string, hash []byte) error {
	if channelID != webhookChannel {
		return domain.ErrNotFound
	}
	m.hash = hash
	return nil
}

func (m *webhookStore) GetChannelWebhookKeyHash(_ context.Context, channelID string) (tenantID string, hash []byte, err error) {
	m.lookups++
	if channelID != webhookChannel {
		return "", nil, domain.ErrNotFound
	}
	return m.tenant, m.hash, nil
}

func (m *webhookStore) MarkChannelRead(_ context.Context, channelID, userID, messageID string) (*channel.ReadState, error) {
	if channelID != webhookChannel || messageID == "" {
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

	key, err := svc.RegenerateWebhookKey(ctx, webhookChannel)
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

	authorized, err := svc.AuthorizeWebhook(context.Background(), webhookChannel, key)
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
		"wrong key":       {webhookChannel, wrong},
		"empty key":       {webhookChannel, ""},
		"unknown channel": {"c0ffee00-0000-4000-8000-0000000000ff", key},
	} {
		if _, err := svc.AuthorizeWebhook(context.Background(), call.channel, call.key); !errors.Is(err, service.ErrWebhookForbidden) {
			t.Errorf("%s: err = %v, want ErrWebhookForbidden", name, err)
		}
	}

	again, err := svc.RegenerateWebhookKey(ctx, webhookChannel)
	if err != nil || again == key {
		t.Fatalf("regenerate: %q, %v", again, err)
	}
	if _, err := svc.AuthorizeWebhook(context.Background(), webhookChannel, key); !errors.Is(err, service.ErrWebhookForbidden) {
		t.Fatal("the replaced key still works")
	}
}

// S6-H review 5: a channel ID that is not a UUID would fail in Postgres
// (22P02) and answer 500; it is refused like any other unknown channel,
// before the lookup.
func TestChannelService_WebhookChannelIDIsAUUID(t *testing.T) {
	store := &webhookStore{tenant: webhookTenant}
	svc := service.NewChannelService(store, &runtimeMockBroadcaster{})
	for _, id := range []string{"ch-1", "", "' OR 1=1 --", "{" + webhookChannel + "}", "urn:uuid:" + webhookChannel, strings.ReplaceAll(webhookChannel, "-", "")} {
		if _, err := svc.AuthorizeWebhook(context.Background(), id, "anything"); !errors.Is(err, service.ErrWebhookForbidden) {
			t.Errorf("channel %q: err = %v, want ErrWebhookForbidden", id, err)
		}
	}
	if store.lookups != 0 {
		t.Fatalf("%d lookups for channel IDs that are not UUIDs, want none", store.lookups)
	}
}

func TestChannelService_WebhookWithoutKeyIsForbidden(t *testing.T) {
	svc := service.NewChannelService(&webhookStore{tenant: webhookTenant}, &runtimeMockBroadcaster{})
	if _, err := svc.AuthorizeWebhook(context.Background(), webhookChannel, "anything"); !errors.Is(err, service.ErrWebhookForbidden) {
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

	state, err := svc.MarkRead(ctx, webhookChannel, "user-1", "msg-2")
	if err != nil || state.LastReadMessageID != "msg-2" {
		t.Fatalf("MarkRead = %+v, %v", state, err)
	}
	events := bc.snapshot()
	if len(events) != 1 || events[0].EventType != event.EventChannelRead || events[0].Tenant != webhookTenant {
		t.Fatalf("events = %+v", events)
	}
	ev, ok := events[0].Data.(event.ChannelReadEvent)
	if !ok || ev.ChannelID != webhookChannel || ev.UserID != "user-1" || ev.MessageID != "msg-2" {
		t.Fatalf("event = %+v", events[0].Data)
	}

	if _, err := svc.MarkRead(ctx, webhookChannel, "user-1", ""); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("empty message id: err = %v, want a validation error", err)
	}
	if _, err := svc.MarkRead(ctx, "c0ffee00-0000-4000-8000-0000000000ff", "user-1", "msg-2"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown channel: err = %v", err)
	}
	if len(bc.snapshot()) != 1 {
		t.Fatal("a failed mark was broadcast")
	}
}
