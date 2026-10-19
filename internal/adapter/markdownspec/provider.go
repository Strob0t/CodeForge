// Package markdownspec implements a specprovider.Provider for ROADMAP.md files.
package markdownspec

import (
	"context"

	"github.com/Strob0t/CodeForge/internal/port/specprovider"
	"github.com/Strob0t/CodeForge/internal/workspacefs"
)

// Compile-time checks for optional interface compliance.
var (
	_ specprovider.ItemParser = (*Provider)(nil)
	_ specprovider.ItemWriter = (*Provider)(nil)
)

const providerName = "markdown"

// candidates lists the filenames to detect, in priority order.
var candidates = []string{
	"ROADMAP.md", "roadmap.md",
	"TODO.md", "todo.md",
	"docs/ROADMAP.md", "docs/roadmap.md",
	"docs/TODO.md", "docs/todo.md",
}

// Provider implements specprovider.Provider for Markdown roadmap files.
type Provider struct{}

func (p *Provider) Name() string { return providerName }

func (p *Provider) Capabilities() specprovider.Capabilities {
	return specprovider.Capabilities{Read: true, Write: true, Sync: false}
}

// Detect, ListSpecs and the readers and writers below work through
// workspacefs (KI-95): a candidate must be a regular file inside the
// workspace, and a spec path never leads out of it.
func (p *Provider) Detect(ctx context.Context, workspacePath string) (bool, error) {
	specs, err := p.ListSpecs(ctx, workspacePath)
	return len(specs) > 0, err
}

func (p *Provider) ListSpecs(_ context.Context, workspacePath string) ([]specprovider.Spec, error) {
	ws, err := workspacefs.Open(workspacePath)
	if err != nil {
		return nil, nil //nolint:nilerr // a workspace that cannot be opened has no specs, as a missing file had none
	}
	defer func() { _ = ws.Close() }()
	var specs []specprovider.Spec
	for _, name := range candidates {
		info, err := ws.Stat(name)
		if err == nil && info.Mode().IsRegular() {
			specs = append(specs, specprovider.Spec{
				Path:   name,
				Format: providerName,
				Title:  name,
			})
		}
	}
	return specs, nil
}

func (p *Provider) ReadSpec(_ context.Context, workspacePath, specPath string) ([]byte, error) {
	return specprovider.ReadFile(workspacePath, specPath)
}

// ParseItems implements specprovider.ItemParser by converting internal SpecItems
// to the port-level SpecItemDetail type.
func (p *Provider) ParseItems(content []byte) ([]specprovider.SpecItemDetail, error) {
	items := ParseMarkdown(content)
	details := make([]specprovider.SpecItemDetail, 0, len(items))
	for _, item := range items {
		details = append(details, specprovider.SpecItemDetail{
			Title:      item.Title,
			Status:     string(item.Status),
			SourceLine: item.SourceLine,
			Level:      string(item.Level),
		})
	}
	return details, nil
}

// PatchItems implements specprovider.ItemWriter: it sets the checkbox
// markers of the items' lines and changes nothing else (KI-203).
func (p *Provider) PatchItems(content []byte, items []specprovider.SpecItemDetail) ([]byte, error) {
	return patchCheckboxes(content, items)
}
