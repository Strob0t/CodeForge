package a2a

import (
	"context"

	sdka2a "github.com/a2aproject/a2a-go/a2a"
)

// ModeInfo describes a CodeForge mode for AgentCard skill generation.
type ModeInfo struct {
	ID          string
	Name        string
	Description string
}

// CardBuilder builds an A2A AgentCard from registered modes.
// Implements a2asrv.AgentCardProducer.
type CardBuilder struct {
	baseURL   string
	modes     []ModeInfo
	version   string
	streaming bool // FIX-109: Configurable via config.A2A.Streaming
}

// NewCardBuilder creates a CardBuilder.
func NewCardBuilder(baseURL string, modes []ModeInfo, version string, opts ...CardOption) *CardBuilder {
	cb := &CardBuilder{baseURL: baseURL, modes: modes, version: version}
	for _, o := range opts {
		o(cb)
	}
	return cb
}

// CardOption configures optional CardBuilder behavior.
type CardOption func(*CardBuilder)

// WithStreaming sets the streaming capability flag on the AgentCard.
func WithStreaming(enabled bool) CardOption {
	return func(cb *CardBuilder) { cb.streaming = enabled }
}

// Card returns the dynamic AgentCard (implements a2asrv.AgentCardProducer).
func (b *CardBuilder) Card(_ context.Context) (*sdka2a.AgentCard, error) {
	skills := make([]sdka2a.AgentSkill, 0, len(b.modes))
	for i := range b.modes {
		skills = append(skills, sdka2a.AgentSkill{
			ID:          b.modes[i].ID,
			Name:        b.modes[i].Name,
			Description: b.modes[i].Description,
			Tags:        []string{"codeforge"},
		})
	}

	card := &sdka2a.AgentCard{
		Name:        "CodeForge",
		Description: "AI coding agent orchestration platform",
		URL:         b.baseURL,
		Version:     b.version,
		Skills:      skills,
		Capabilities: sdka2a.AgentCapabilities{
			Streaming: b.streaming,
		},
		SecuritySchemes: sdka2a.NamedSecuritySchemes{
			"apiKey": sdka2a.APIKeySecurityScheme{
				In:   sdka2a.APIKeySecuritySchemeInHeader,
				Name: "Authorization",
			},
		},
		Security: []sdka2a.SecurityRequirements{{"apiKey": {}}},
		Provider: &sdka2a.AgentProvider{
			Org: "CodeForge",
		},
	}
	return card, nil
}
