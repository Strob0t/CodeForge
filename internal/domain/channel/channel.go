package channel

import (
	"errors"
	"time"
)

// ErrReadStateNotTracked reports that read positions are not kept for the
// user: it has no account row (the default user while auth is disabled, the
// internal service key user), and one identity shared by every such request
// has no personal read position.
var ErrReadStateNotTracked = errors.New("read state is not tracked for a user without an account")

// ChannelType distinguishes project channels from bot channels.
type ChannelType string

const (
	TypeProject ChannelType = "project"
	TypeBot     ChannelType = "bot"
)

// SenderType identifies who sent a channel message.
type SenderType string

const (
	SenderUser    SenderType = "user"
	SenderAgent   SenderType = "agent"
	SenderBot     SenderType = "bot"
	SenderWebhook SenderType = "webhook"
)

// NotifySetting controls per-member notification behavior.
type NotifySetting string

const (
	NotifyAll      NotifySetting = "all"
	NotifyMentions NotifySetting = "mentions"
	NotifyNothing  NotifySetting = "nothing"
)

// MemberRole is the role of a member in a channel.
type MemberRole string

const (
	RoleOwner  MemberRole = "owner"
	RoleAdmin  MemberRole = "admin"
	RoleMember MemberRole = "member"
)

// Channel represents a project or bot channel.
type Channel struct {
	ID          string      `json:"id"`
	TenantID    string      `json:"tenant_id"`
	ProjectID   string      `json:"project_id,omitempty"`
	Name        string      `json:"name"`
	Type        ChannelType `json:"type"`
	Description string      `json:"description"`
	// HasWebhookKey reports whether a webhook key was generated; the key
	// itself is shown once and only its hash is stored.
	HasWebhookKey bool `json:"has_webhook_key"`
	// CreatedBy is the user who created the channel; empty when the creator
	// has no account row (auth disabled, internal service key) or was erased.
	CreatedBy string    `json:"created_by,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	// UnreadCount is the number of top-level messages of others after the
	// calling user's read position (only set when channels are listed for a user).
	UnreadCount int `json:"unread_count"`
}

// ReadState is a user's read position in a channel.
type ReadState struct {
	ChannelID         string    `json:"channel_id"`
	UserID            string    `json:"user_id"`
	LastReadMessageID string    `json:"last_read_message_id,omitempty"`
	LastReadAt        time.Time `json:"last_read_at"`
}

// ErasedSenderName replaces the sender name of the messages of a user whose
// data was erased (GDPR Art. 17); the messages stay in the channel.
const ErasedSenderName = "Deleted user"

// Message represents a message in a channel.
type Message struct {
	ID        string `json:"id"`
	ChannelID string `json:"channel_id"`
	// SenderID is the user who posted the message. It is empty for agents,
	// bots and webhooks, for an erased user and for a caller without an
	// account row (auth disabled, internal service key; KI-89): SenderType
	// and SenderName tell who sent it.
	SenderID   string     `json:"sender_id,omitempty"`
	SenderType SenderType `json:"sender_type"`
	SenderName string     `json:"sender_name"`
	Content    string     `json:"content"`
	Metadata   string     `json:"metadata,omitempty"` // JSONB stored as string
	ParentID   string     `json:"parent_id,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// Member represents a user's membership in a channel.
type Member struct {
	ChannelID string        `json:"channel_id"`
	UserID    string        `json:"user_id"`
	Role      MemberRole    `json:"role"`
	Notify    NotifySetting `json:"notify"`
	JoinedAt  time.Time     `json:"joined_at"`
}
