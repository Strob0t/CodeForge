// Package gitprovider defines the Git provider port (interface) and capabilities.
package gitprovider

import "context"

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
}
