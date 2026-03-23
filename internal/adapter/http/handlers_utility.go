package http

import (
	"github.com/Strob0t/CodeForge/internal/config"
)

// UtilityHandlers groups miscellaneous HTTP handlers for provider listings,
// agent config, and other utility endpoints.
type UtilityHandlers struct {
	AgentConfig   *config.Agent
	OllamaBaseURL string
}
