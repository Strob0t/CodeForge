// Package specprovider defines the port interface for specification providers
// (OpenSpec, Spec Kit, Autospec, etc.).
package specprovider

import (
	"context"
	"errors"
)

// MaxSpecBytes caps a spec file a provider reads from a workspace (KI-95:
// workspace files are read through workspacefs, never through a symlink that
// leaves the workspace).
const MaxSpecBytes = 1 << 20

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

// ErrItemMoved is the error of ItemWriter.PatchItems when the line of an
// item no longer holds that item: the file changed since it was parsed.
var ErrItemMoved = errors.New("spec item is no longer on its line")

// ItemParser is an optional interface that providers can implement to support
// parsing individual items from spec files. When supported, ImportSpecs()
// imports the file's checkbox items as features (KI-203).
type ItemParser interface {
	// ParseItems returns the items of a spec file's content (as ReadSpec
	// returns it) in file order; SourceLine counts from 1.
	ParseItems(content []byte) ([]SpecItemDetail, error)
}

// ItemWriter is an optional interface that providers can implement to support
// writing feature status changes back to spec files.
type ItemWriter interface {
	// PatchItems returns content with the status marker of each item's line
	// set to the item's Status ("done" checks it, any other status unchecks
	// it). Nothing else changes, byte for byte: the file is never rendered
	// anew (KI-203). Each item's SourceLine must still hold a checkbox with
	// the item's Title, or PatchItems returns no content and an error
	// wrapping ErrItemMoved.
	PatchItems(content []byte, items []SpecItemDetail) ([]byte, error)
}
