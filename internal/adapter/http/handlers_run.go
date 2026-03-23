package http

import (
	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/port/eventstore"
	"github.com/Strob0t/CodeForge/internal/service"
)

// RunHandlers groups HTTP handlers for run lifecycle (start, cancel, get)
// and run event retrieval.
type RunHandlers struct {
	Runtime *service.RuntimeService
	Events  eventstore.Store
	Limits  *config.Limits
}
