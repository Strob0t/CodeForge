// Package gitprovider defines the Git provider port (interface) and capabilities.
package gitprovider

import (
	"context"

	"github.com/Strob0t/CodeForge/internal/domain/project"
)

// Capabilities declares which operations a git provider supports.
type Capabilities struct {
	Clone       bool `json:"clone"`
	Push        bool `json:"push"`
	PullRequest bool `json:"pull_request"`
	Webhook     bool `json:"webhook"`
	Issues      bool `json:"issues"`
}

// Provider is the port interface for interacting with a Git hosting platform.
type Provider interface {
	// Name returns the unique identifier for this provider (e.g. "github", "gitlab").
	Name() string

	// Capabilities returns what this provider supports.
	Capabilities() Capabilities

	// CloneURL returns the clone URL for a given repository identifier.
	CloneURL(ctx context.Context, repo string) (string, error)

	// ListRepos returns a list of repository identifiers accessible to the user.
	ListRepos(ctx context.Context) ([]string, error)

	// Clone clones a repository to the given local path.
	Clone(ctx context.Context, url, destPath string) error

	// Status returns the git status of a local repository.
	Status(ctx context.Context, repoPath string) (*project.GitStatus, error)

	// Pull fetches and merges updates for the given repository.
	Pull(ctx context.Context, repoPath string) error

	// ListBranches returns all branches of a local repository.
	ListBranches(ctx context.Context, repoPath string) ([]project.Branch, error)

	// Checkout switches to the specified branch.
	Checkout(ctx context.Context, repoPath, branch string) error
}
