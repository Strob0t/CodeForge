package database

import (
	"context"

	"github.com/Strob0t/CodeForge/internal/domain/channel"
)

// ChannelStore defines database operations for real-time channels.
type ChannelStore interface {
	CreateChannel(ctx context.Context, ch *channel.Channel) (*channel.Channel, error)
	GetChannel(ctx context.Context, id string) (*channel.Channel, error)
	// ListChannels lists the tenant's channels (of a project, when projectID
	// is set) with the unread count of userID (0 without a user).
	ListChannels(ctx context.Context, projectID, userID string) ([]channel.Channel, error)
	DeleteChannel(ctx context.Context, id string) error
	CreateChannelMessage(ctx context.Context, msg *channel.Message) (*channel.Message, error)
	ListChannelMessages(ctx context.Context, channelID string, cursor string, limit int) ([]channel.Message, error)
	AddChannelMember(ctx context.Context, m *channel.Member) error
	UpdateChannelMemberNotify(ctx context.Context, channelID, userID string, notify channel.NotifySetting) error
	// AnonymizeChannelMessagesForUser replaces the sender name of the user's
	// messages with channel.ErasedSenderName (GDPR erasure, before the user
	// row is deleted) and returns how many messages it changed.
	AnonymizeChannelMessagesForUser(ctx context.Context, userID string) (int64, error)
	// SetChannelWebhookKeyHash stores the hash of a channel's new webhook key.
	SetChannelWebhookKeyHash(ctx context.Context, channelID string, hash []byte) error
	// GetChannelWebhookKeyHash returns the tenant and the webhook key hash of
	// a channel of any tenant (nil when no key was generated): the webhook
	// caller has no tenant, its key authenticates it.
	GetChannelWebhookKeyHash(ctx context.Context, channelID string) (tenantID string, hash []byte, err error)
	// MarkChannelRead moves userID's read position in the channel to the
	// message (never backwards) and returns the position.
	MarkChannelRead(ctx context.Context, channelID, userID, messageID string) (*channel.ReadState, error)
	ListChannelReadStates(ctx context.Context, channelID string) ([]channel.ReadState, error)
}
