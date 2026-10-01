package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/channel"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/port/broadcast"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// ErrWebhookForbidden rejects a channel webhook call: the channel has no
// webhook key, does not exist, or the key is wrong (not told apart, so a
// caller learns nothing about channels it has no key for).
var ErrWebhookForbidden = errors.New("invalid webhook key")

// ChannelService manages channel operations.
type ChannelService struct {
	db  database.Store
	hub broadcast.Broadcaster
}

// NewChannelService creates a new ChannelService.
func NewChannelService(db database.Store, hub broadcast.Broadcaster) *ChannelService {
	return &ChannelService{db: db, hub: hub}
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

// List returns all channels for a project (or all tenant channels if
// projectID is empty) with the unread count of userID.
func (s *ChannelService) List(ctx context.Context, projectID, userID string) ([]channel.Channel, error) {
	return s.db.ListChannels(ctx, projectID, userID)
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

// SendMessage validates and stores a channel message, then broadcasts it to
// the clients of the tenant in ctx, which owns the channel (the store accepts
// messages only into a channel of that tenant).
func (s *ChannelService) SendMessage(ctx context.Context, msg *channel.Message) (*channel.Message, error) {
	if msg.Content == "" {
		return nil, fmt.Errorf("message content is required: %w", domain.ErrValidation)
	}
	created, err := s.db.CreateChannelMessage(ctx, msg)
	if err != nil {
		return nil, err
	}
	s.hub.BroadcastEvent(ctx, event.EventChannelMessage, event.ChannelMessageEvent{
		ChannelID: created.ChannelID,
		Message:   *created,
	})
	return created, nil
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

// RegenerateWebhookKey makes a new webhook key for a channel of the caller's
// tenant and returns it; only its SHA-256 is stored, so this is the only
// time the key is shown. A previous key stops working.
func (s *ChannelService) RegenerateWebhookKey(ctx context.Context, channelID string) (string, error) {
	key, err := s.GenerateWebhookKey()
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256([]byte(key))
	if err := s.db.SetChannelWebhookKeyHash(ctx, channelID, hash[:]); err != nil {
		return "", err
	}
	return key, nil
}

// AuthorizeWebhook checks a webhook call's key against the channel's stored
// hash (constant time) and returns ctx scoped to the channel's tenant.
func (s *ChannelService) AuthorizeWebhook(ctx context.Context, channelID, key string) (context.Context, error) {
	tenantID, stored, err := s.db.GetChannelWebhookKeyHash(ctx, channelID)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, ErrWebhookForbidden
	}
	if err != nil {
		return nil, err
	}
	presented := sha256.Sum256([]byte(key))
	if key == "" || len(stored) != len(presented) || subtle.ConstantTimeCompare(stored, presented[:]) != 1 {
		return nil, ErrWebhookForbidden
	}
	return tenantctx.WithTenant(ctx, tenantID), nil
}

// MarkRead moves the user's read position in a channel of the caller's
// tenant to a message and broadcasts channel.read to the tenant.
func (s *ChannelService) MarkRead(ctx context.Context, channelID, userID, messageID string) (*channel.ReadState, error) {
	if messageID == "" {
		return nil, fmt.Errorf("message_id is required: %w", domain.ErrValidation)
	}
	state, err := s.db.MarkChannelRead(ctx, channelID, userID, messageID)
	if err != nil {
		return nil, err
	}
	s.hub.BroadcastEvent(ctx, event.EventChannelRead, event.ChannelReadEvent{
		ChannelID:  state.ChannelID,
		UserID:     state.UserID,
		MessageID:  state.LastReadMessageID,
		LastReadAt: state.LastReadAt,
	})
	return state, nil
}

// ListReadStates returns the read positions in a channel of the caller's tenant.
func (s *ChannelService) ListReadStates(ctx context.Context, channelID string) ([]channel.ReadState, error) {
	return s.db.ListChannelReadStates(ctx, channelID)
}

// GenerateWebhookKey returns a cryptographically random 32-byte hex string.
func (s *ChannelService) GenerateWebhookKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate webhook key: %w", err)
	}
	return hex.EncodeToString(b), nil
}
