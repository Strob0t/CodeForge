package channel

import "time"

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
	WebhookKey  string      `json:"webhook_key,omitempty"`
	CreatedBy   string      `json:"created_by,omitempty"`
	CreatedAt   time.Time   `json:"created_at"`
}

// Message represents a message in a channel.
type Message struct {
	ID         string     `json:"id"`
	ChannelID  string     `json:"channel_id"`
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
