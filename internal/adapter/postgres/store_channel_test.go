package postgres_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/channel"
)

// TestStore_ChannelMessages: channel_messages.tenant_id is NOT NULL since
// migration 079, but the insert never set it, so every message failed. The
// message takes the tenant of its channel, and only a channel of the caller's
// tenant accepts messages.
func TestStore_ChannelMessages(t *testing.T) {
	store := setupStore(t)
	tenantA := createTestTenant(t, store)
	tenantB := createTestTenant(t, store)
	ctxA := ctxWithTenant(t, tenantA)
	ctxB := ctxWithTenant(t, tenantB)

	ch, err := store.CreateChannel(ctxA, &channel.Channel{
		Name: "general-" + uuid.New().String()[:8],
		Type: channel.TypeBot,
	})
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteChannel(ctxA, ch.ID) })

	msg, err := store.CreateChannelMessage(ctxA, &channel.Message{
		ChannelID:  ch.ID,
		SenderType: channel.SenderUser,
		SenderName: "Alice",
		Content:    "hello",
	})
	if err != nil {
		t.Fatalf("CreateChannelMessage: %v", err)
	}
	if msg.ID == "" || msg.ChannelID != ch.ID || msg.Content != "hello" || msg.CreatedAt.IsZero() {
		t.Fatalf("unexpected message %+v", msg)
	}

	reply, err := store.CreateChannelMessage(ctxA, &channel.Message{
		ChannelID:  ch.ID,
		SenderType: channel.SenderAgent,
		SenderName: "Bot",
		Content:    "reply",
		ParentID:   msg.ID,
	})
	if err != nil {
		t.Fatalf("CreateChannelMessage (reply): %v", err)
	}
	if reply.ParentID != msg.ID {
		t.Fatalf("reply parent = %q, want %q", reply.ParentID, msg.ID)
	}

	t.Run("another tenant cannot post into the channel", func(t *testing.T) {
		_, err := store.CreateChannelMessage(ctxB, &channel.Message{
			ChannelID:  ch.ID,
			SenderType: channel.SenderUser,
			SenderName: "Mallory",
			Content:    "intrusion",
		})
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
	})

	t.Run("unknown channel is not found", func(t *testing.T) {
		_, err := store.CreateChannelMessage(ctxA, &channel.Message{
			ChannelID:  uuid.New().String(),
			SenderType: channel.SenderUser,
			SenderName: "Alice",
			Content:    "lost",
		})
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
	})

	t.Run("messages are listed for the owning tenant only", func(t *testing.T) {
		got, err := store.ListChannelMessages(ctxA, ch.ID, "", 50)
		if err != nil {
			t.Fatalf("ListChannelMessages: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("got %d messages, want 2", len(got))
		}
		other, err := store.ListChannelMessages(ctxB, ch.ID, "", 50)
		if err != nil {
			t.Fatalf("ListChannelMessages (other tenant): %v", err)
		}
		if len(other) != 0 {
			t.Fatalf("other tenant sees %d messages", len(other))
		}
	})
}
