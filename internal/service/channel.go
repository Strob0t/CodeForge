package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/Strob0t/CodeForge/internal/domain/channel"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

// ChannelService manages channel operations.
type ChannelService struct {
	db database.Store
}

// NewChannelService creates a new ChannelService.
func NewChannelService(db database.Store) *ChannelService {
	return &ChannelService{db: db}
}

// Create validates and creates a new channel.
func (s *ChannelService) Create(ctx context.Context, ch *channel.Channel) (*channel.Channel, error) {
	if ch.Name == "" {
		return nil, fmt.Errorf("channel name is required")
	}
	if ch.Type != channel.TypeProject && ch.Type != channel.TypeBot {
		return nil, fmt.Errorf("invalid channel type: %s", ch.Type)
	}
	return s.db.CreateChannel(ctx, ch)
}

// Get returns a channel by ID.
func (s *ChannelService) Get(ctx context.Context, id string) (*channel.Channel, error) {
	return s.db.GetChannel(ctx, id)
}

// List returns all channels for a project (or all tenant channels if projectID is empty).
func (s *ChannelService) List(ctx context.Context, projectID string) ([]channel.Channel, error) {
	return s.db.ListChannels(ctx, projectID)
}

// Delete removes a channel. Only bot channels can be deleted.
func (s *ChannelService) Delete(ctx context.Context, id string) error {
	ch, err := s.db.GetChannel(ctx, id)
	if err != nil {
		return err
	}
	if ch.Type != channel.TypeBot {
		return fmt.Errorf("only bot channels can be deleted")
	}
	return s.db.DeleteChannel(ctx, id)
}

// SendMessage validates and stores a channel message.
func (s *ChannelService) SendMessage(ctx context.Context, msg *channel.Message) (*channel.Message, error) {
	if msg.Content == "" {
		return nil, fmt.Errorf("message content is required")
	}
	return s.db.CreateChannelMessage(ctx, msg)
}

// ListMessages returns paginated messages for a channel.
// Limit is clamped to [1, 100] with a default of 50.
func (s *ChannelService) ListMessages(ctx context.Context, channelID, cursor string, limit int) ([]channel.Message, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	return s.db.ListChannelMessages(ctx, channelID, cursor, limit)
}

// AddMember adds a member to a channel.
func (s *ChannelService) AddMember(ctx context.Context, member *channel.Member) error {
	return s.db.AddChannelMember(ctx, member)
}

// UpdateMemberNotify updates a member's notification setting.
func (s *ChannelService) UpdateMemberNotify(ctx context.Context, channelID, userID string, notify channel.NotifySetting) error {
	return s.db.UpdateChannelMemberNotify(ctx, channelID, userID, notify)
}

// GenerateWebhookKey returns a cryptographically random 32-byte hex string.
func (s *ChannelService) GenerateWebhookKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate webhook key: %w", err)
	}
	return hex.EncodeToString(b), nil
}
