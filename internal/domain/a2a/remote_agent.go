package a2a

import (
	"fmt"
	"net/url"
	"time"
)

// RemoteAgent represents a discovered remote A2A agent.
type RemoteAgent struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	URL         string     `json:"url"`
	Description string     `json:"description"`
	TrustLevel  string     `json:"trust_level"`
	Enabled     bool       `json:"enabled"`
	Skills      []string   `json:"skills"`
	LastSeen    *time.Time `json:"last_seen,omitempty"`
	CardJSON    []byte     `json:"card_json,omitempty"`
	TenantID    string     `json:"tenant_id"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// Validate checks required fields on a RemoteAgent.
func (a *RemoteAgent) Validate() error {
	if a.Name == "" {
		return fmt.Errorf("remote agent: name is required")
	}
	if a.URL == "" {
		return fmt.Errorf("remote agent: url is required")
	}
	u, err := url.Parse(a.URL)
	if err != nil {
		return fmt.Errorf("remote agent: invalid url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("remote agent: url scheme must be http or https, got %q", u.Scheme)
	}
	return nil
}

// NewRemoteAgent returns a remote agent with sensible defaults.
func NewRemoteAgent(name, agentURL string) *RemoteAgent {
	now := time.Now().UTC()
	return &RemoteAgent{
		Name:       name,
		URL:        agentURL,
		TrustLevel: "partial",
		Enabled:    true,
		Skills:     []string{},
		TenantID:   "00000000-0000-0000-0000-000000000000",
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}
