package http

import (
	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/service"
)

// AgentHandlers groups HTTP handlers for agent CRUD, dispatch,
// inbox, and state management.
type AgentHandlers struct {
	Agents *service.AgentService
	Limits *config.Limits
}
