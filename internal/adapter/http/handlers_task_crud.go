package http

import (
	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/service"
)

// TaskHandlers groups HTTP handlers for task CRUD, claim,
// active work, and active agents.
type TaskHandlers struct {
	Tasks      *service.TaskService
	Agents     *service.AgentService
	ActiveWork *service.ActiveWorkService
	Limits     *config.Limits
}
