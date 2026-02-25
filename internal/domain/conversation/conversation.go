package conversation

import (
	"encoding/json"
	"time"
)

// Conversation represents a chat thread tied to a project.
type Conversation struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	ProjectID string    `json:"project_id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Message represents a single message in a conversation.
type Message struct {
	ID             string          `json:"id"`
	ConversationID string          `json:"conversation_id"`
	Role           string          `json:"role"` // "user", "assistant", "system", "tool"
	Content        string          `json:"content"`
	ToolCalls      json.RawMessage `json:"tool_calls,omitempty"`
	ToolCallID     string          `json:"tool_call_id,omitempty"`
	ToolName       string          `json:"tool_name,omitempty"`
	TokensIn       int             `json:"tokens_in,omitempty"`
	TokensOut      int             `json:"tokens_out,omitempty"`
	Model          string          `json:"model,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
}

// CreateRequest is the request body for creating a new conversation.
type CreateRequest struct {
	ProjectID string `json:"project_id"`
	Title     string `json:"title"`
}

// SendMessageRequest is the request body for sending a message.
type SendMessageRequest struct {
	Content string `json:"content"`
	Agentic *bool  `json:"agentic,omitempty"` // Override agentic mode (nil = use project default).
}
