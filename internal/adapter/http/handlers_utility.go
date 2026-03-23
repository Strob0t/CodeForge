package http

import (
	"net/http"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/port/agentbackend"
	"github.com/Strob0t/CodeForge/internal/port/gitprovider"
)

// UtilityHandlers groups miscellaneous HTTP handlers for provider listings,
// agent config, and other utility endpoints.
type UtilityHandlers struct {
	AgentConfig   *config.Agent
	OllamaBaseURL string
}

// ListGitProviders handles GET /api/v1/providers/git
func (uh *UtilityHandlers) ListGitProviders(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string][]string{
		"providers": gitprovider.Available(),
	})
}

// ListAgentBackends handles GET /api/v1/providers/agent
func (uh *UtilityHandlers) ListAgentBackends(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string][]string{
		"backends": agentbackend.Available(),
	})
}
