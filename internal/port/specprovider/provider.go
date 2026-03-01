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

// SpecItemDetail represents a single actionable item within a spec file
// (e.g., a checkbox, list entry, or heading from a TODO/roadmap document).
type SpecItemDetail struct {
	Title      string `json:"title"`
	Status     string `json:"status"` // "todo", "done", "in_progress"
	SourceLine int    `json:"source_line"`
	Level      string `json:"level"` // "h1", "h2", "h3", "checkbox", "list_item"
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

// ItemParser is an optional interface that providers can implement to support
// parsing individual items from spec files. When supported, ImportSpecs()
// creates one Feature per item instead of one Feature per file.
type ItemParser interface {
	// ParseItems returns individual actionable items from a spec file.
	ParseItems(ctx context.Context, workspacePath, specPath string) ([]SpecItemDetail, error)
}

// ItemWriter is an optional interface that providers can implement to support
// writing feature status changes back to spec files.
type ItemWriter interface {
	// WriteItems writes spec items back to the spec file, updating statuses.
	WriteItems(ctx context.Context, workspacePath, specPath string, items []SpecItemDetail) error
}
