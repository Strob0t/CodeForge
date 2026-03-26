package database

import (
	"context"

	"github.com/Strob0t/CodeForge/internal/domain/channel"
)

// ChannelStore defines database operations for real-time channels.
type ChannelStore interface {
	CreateChannel(ctx context.Context, ch *channel.Channel) (*channel.Channel, error)
	GetChannel(ctx context.Context, id string) (*channel.Channel, error)
	ListChannels(ctx context.Context, projectID string) ([]channel.Channel, error)
	DeleteChannel(ctx context.Context, id string) error
	CreateChannelMessage(ctx context.Context, msg *channel.Message) (*channel.Message, error)
	ListChannelMessages(ctx context.Context, channelID string, cursor string, limit int) ([]channel.Message, error)
	AddChannelMember(ctx context.Context, m *channel.Member) error
	UpdateChannelMemberNotify(ctx context.Context, channelID, userID string, notify channel.NotifySetting) error
}
