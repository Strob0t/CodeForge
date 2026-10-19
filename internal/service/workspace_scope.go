package service

import (
	"fmt"
	"strings"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/project"
)

// requireWorkspace rejects a project a run or backend task could not work in:
// without a workspace the worker would edit its own directory.
func requireWorkspace(proj *project.Project) error {
	if strings.TrimSpace(proj.WorkspacePath) == "" {
		return fmt.Errorf("%w: project %s has no workspace (clone or adopt a repository first)", domain.ErrValidation, proj.ID)
	}
	return nil
}

// requireProject rejects a task or agent of another project: the run or
// backend task works in projectID's workspace.
func requireProject(kind, id, got, projectID string) error {
	if got != projectID {
		return fmt.Errorf("%w: %s %s belongs to project %q, not %q", domain.ErrValidation, kind, id, got, projectID)
	}
	return nil
}
