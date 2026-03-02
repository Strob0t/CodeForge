package agent

import (
	"errors"
	"strings"
	"time"
)

// InboxMessage represents a message sent between agents.
type InboxMessage struct {
	ID        string    `json:"id"`
	AgentID   string    `json:"agent_id"`
	FromAgent string    `json:"from_agent"`
	Content   string    `json:"content"`
	Priority  int       `json:"priority"`
	Read      bool      `json:"read"`
	CreatedAt time.Time `json:"created_at"`
}

// Validate checks that the message has the minimum required fields.
func (m *InboxMessage) Validate() error {
	if m.AgentID == "" {
		return errors.New("agent_id required")
	}
	if strings.TrimSpace(m.Content) == "" {
		return errors.New("content required")
	}
	if m.Priority < 0 {
		return errors.New("priority must be >= 0")
	}
	return nil
}
