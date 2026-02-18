// Package specprovider defines the port interface for specification providers
// (OpenSpec, Spec Kit, Autospec, etc.).
package specprovider

import "context"

// Spec represents a specification document discovered in a repository.
type Spec struct {
	Path   string `json:"path"`
	Format string `json:"format"`
	Title  string `json:"title"`
}

// Capabilities declares what a spec provider supports.
type Capabilities struct {
	Read  bool `json:"read"`
	Write bool `json:"write"`
	Sync  bool `json:"sync"`
}

// Provider is the port interface for repo-based specification providers.
type Provider interface {
	// Name returns the provider identifier (e.g., "openspec", "speckit", "autospec").
	Name() string

	// Capabilities returns what this provider supports.
	Capabilities() Capabilities

	// Detect checks whether this provider's format exists in the workspace.
	Detect(ctx context.Context, workspacePath string) (bool, error)

	// ListSpecs returns all specs found in the workspace.
	ListSpecs(ctx context.Context, workspacePath string) ([]Spec, error)

	// ReadSpec returns the raw content of a spec file.
	ReadSpec(ctx context.Context, workspacePath, specPath string) ([]byte, error)
}
