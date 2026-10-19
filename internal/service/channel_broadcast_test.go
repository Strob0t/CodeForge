package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/channel"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/service"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// KI-42: a stored channel message is broadcast as channel.message to the
// tenant in ctx (the store only accepts messages into a channel of that
// tenant), so other members see it without a reload.
func TestChannelService_SendMessage_Broadcasts(t *testing.T) {
	const tenant = "aaaaaaaa-0000-0000-0000-000000000001"

	tests := []struct {
		name     string
		msg      channel.Message
		storeErr error
		wantErr  error // checked with errors.Is when set
		failure  bool
	}{
		{
			name: "user message",
			msg:  channel.Message{ChannelID: "ch-1", SenderType: channel.SenderUser, SenderName: "alice", Content: "hi"},
		},
		{
			name: "thread reply",
			msg:  channel.Message{ChannelID: "ch-1", SenderType: channel.SenderUser, SenderName: "bob", Content: "re", ParentID: "msg-0"},
		},
		{
			name: "webhook message",
			msg:  channel.Message{ChannelID: "ch-1", SenderType: channel.SenderWebhook, SenderName: "ci", Content: "build ok"},
		},
		{
			name:    "empty content is rejected",
			msg:     channel.Message{ChannelID: "ch-1", SenderType: channel.SenderUser, SenderName: "alice"},
			failure: true,
		},
		{
			name:     "channel of another tenant is not found",
			msg:      channel.Message{ChannelID: "ch-x", SenderType: channel.SenderUser, SenderName: "alice", Content: "hi"},
			storeErr: domain.ErrNotFound,
			wantErr:  domain.ErrNotFound,
			failure:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &chMockStore{createMessageErr: tt.storeErr}
			bc := &runtimeMockBroadcaster{}
			svc := service.NewChannelService(store, bc)
			msg := tt.msg

			got, err := svc.SendMessage(tenantctx.WithTenant(context.Background(), tenant), &msg)

			events := bc.snapshot()
			if tt.failure {
				if err == nil {
					t.Fatal("expected an error")
				}
				if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				if len(events) != 0 {
					t.Fatalf("failed send broadcast %d events", len(events))
				}
				return
			}
			if err != nil {
				t.Fatalf("SendMessage: %v", err)
			}
			if len(events) != 1 {
				t.Fatalf("got %d events, want 1", len(events))
			}
			ev := events[0]
			if ev.EventType != event.EventChannelMessage {
				t.Fatalf("event type = %q, want %q", ev.EventType, event.EventChannelMessage)
			}
			if ev.Tenant != tenant {
				t.Fatalf("event scoped to tenant %q, want %q", ev.Tenant, tenant)
			}
			payload, ok := ev.Data.(event.ChannelMessageEvent)
			if !ok {
				t.Fatalf("payload type %T, want event.ChannelMessageEvent", ev.Data)
			}
			if payload.ChannelID != tt.msg.ChannelID || payload.Message.ID != got.ID ||
				payload.Message.Content != tt.msg.Content || payload.Message.ParentID != tt.msg.ParentID ||
				payload.Message.SenderType != tt.msg.SenderType {
				t.Fatalf("payload = %+v, want the stored message %+v", payload, got)
			}
		})
	}
}
