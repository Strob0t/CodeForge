// Package markdownspec implements a specprovider.Provider for ROADMAP.md files.
package markdownspec

import (
	"context"

	"github.com/Strob0t/CodeForge/internal/domain/project"
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

// ParseSpec reads a spec file and returns parsed structured items.
func (p *Provider) ParseSpec(_ context.Context, workspacePath, specPath string) ([]SpecItem, error) {
	content, err := specprovider.ReadFile(workspacePath, specPath)
	if err != nil {
		return nil, err
	}
	return ParseMarkdown(content), nil
}

// WriteSpec writes structured items back to a spec file as markdown.
func (p *Provider) WriteSpec(_ context.Context, workspacePath, specPath string, items []SpecItem) error {
	ws, err := workspacefs.Open(workspacePath)
	if err != nil {
		return err
	}
	defer func() { _ = ws.Close() }()
	// Shared with the worker's tool user (KI-71).
	return ws.WriteFile(specPath, RenderMarkdown(items), project.WorkspaceFilePerm)
}

// ParseItems implements specprovider.ItemParser by converting internal SpecItems
// to the port-level SpecItemDetail type.
func (p *Provider) ParseItems(ctx context.Context, workspacePath, specPath string) ([]specprovider.SpecItemDetail, error) {
	items, err := p.ParseSpec(ctx, workspacePath, specPath)
	if err != nil {
		return nil, err
	}

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

// WriteItems implements specprovider.ItemWriter by converting port-level
// SpecItemDetail back to internal SpecItems and writing the file.
func (p *Provider) WriteItems(ctx context.Context, workspacePath, specPath string, items []specprovider.SpecItemDetail) error {
	specItems := make([]SpecItem, 0, len(items))
	for i, item := range items {
		specItems = append(specItems, SpecItem{
			Title:      item.Title,
			Status:     ItemStatus(item.Status),
			SortOrder:  i + 1,
			Level:      ItemLevel(item.Level),
			SourceLine: item.SourceLine,
		})
	}
	return p.WriteSpec(ctx, workspacePath, specPath, specItems)
}
