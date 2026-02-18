// Package markdownspec implements a specprovider.Provider for ROADMAP.md files.
package markdownspec

import (
	"context"
	"os"
	"path/filepath"

	"github.com/Strob0t/CodeForge/internal/port/specprovider"
)

const providerName = "markdown"

// candidates lists the filenames to detect, in priority order.
var candidates = []string{"ROADMAP.md", "roadmap.md"}

// Provider implements specprovider.Provider for Markdown roadmap files.
type Provider struct{}

func (p *Provider) Name() string { return providerName }

func (p *Provider) Capabilities() specprovider.Capabilities {
	return specprovider.Capabilities{Read: true, Write: false, Sync: false}
}

func (p *Provider) Detect(_ context.Context, workspacePath string) (bool, error) {
	for _, name := range candidates {
		info, err := os.Stat(filepath.Join(workspacePath, name))
		if err == nil && !info.IsDir() {
			return true, nil
		}
	}
	return false, nil
}

func (p *Provider) ListSpecs(_ context.Context, workspacePath string) ([]specprovider.Spec, error) {
	var specs []specprovider.Spec
	for _, name := range candidates {
		info, err := os.Stat(filepath.Join(workspacePath, name))
		if err == nil && !info.IsDir() {
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
	return os.ReadFile(filepath.Join(workspacePath, specPath)) //nolint:gosec // Path from known candidates list.
}
