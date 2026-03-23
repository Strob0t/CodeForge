package http

import (
	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/service"
)

// PolicyHandlers groups HTTP handlers for policy profile CRUD,
// evaluation, and the allow-always mechanism.
type PolicyHandlers struct {
	Policies  *service.PolicyService
	Projects  *service.ProjectService
	PolicyDir string
	Limits    *config.Limits
}
