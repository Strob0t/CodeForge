// Package autospec implements a specprovider.Provider for the Autospec format.
// It detects and reads YAML spec files from the specs/ directory.
package autospec

import (
	"context"

	"gopkg.in/yaml.v3"

	"github.com/Strob0t/CodeForge/internal/port/specprovider"
	"github.com/Strob0t/CodeForge/internal/workspacefs"
)

const providerName = "autospec"

// Provider implements specprovider.Provider for Autospec (specs/) format.
type Provider struct{}

func (p *Provider) Name() string { return providerName }

func (p *Provider) Capabilities() specprovider.Capabilities {
	return specprovider.Capabilities{Read: true, Write: false, Sync: false}
}

func (p *Provider) Detect(_ context.Context, workspacePath string) (bool, error) {
	// Check for specs/spec.yaml or specs/spec.yml
	for _, name := range []string{"specs/spec.yaml", "specs/spec.yml"} {
		info, err := workspacefs.StatAt(workspacePath, name)
		if err == nil && !info.IsDir() {
			return true, nil
		}
		if err != nil && !workspacefs.IsAbsent(err) {
			return false, err
		}
	}
	return false, nil
}

func (p *Provider) ListSpecs(_ context.Context, workspacePath string) ([]specprovider.Spec, error) {
	return specprovider.ListFiles(workspacePath, "specs", providerName, []string{".yaml", ".yml"}, extractTitle)
}

func (p *Provider) ReadSpec(_ context.Context, workspacePath, specPath string) ([]byte, error) {
	return specprovider.ReadFile(workspacePath, specPath)
}

// extractTitle attempts to parse a title field from YAML content.
// Falls back to the filename without extension.
func extractTitle(ws *workspacefs.Root, relPath string) string {
	data, _, err := ws.ReadFile(relPath, specprovider.MaxSpecBytes)
	if err != nil {
		return specprovider.FileBaseName(relPath)
	}

	var doc struct {
		Title string `yaml:"title"`
	}
	if err := yaml.Unmarshal(data, &doc); err == nil && doc.Title != "" {
		return doc.Title
	}
	return specprovider.FileBaseName(relPath)
}
