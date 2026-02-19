// Package autospec implements a specprovider.Provider for the Autospec format.
// It detects and reads YAML spec files from the specs/ directory.
package autospec

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Strob0t/CodeForge/internal/port/specprovider"
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
	for _, name := range []string{"spec.yaml", "spec.yml"} {
		info, err := os.Stat(filepath.Join(workspacePath, "specs", name))
		if err == nil && !info.IsDir() {
			return true, nil
		}
		if err != nil && !os.IsNotExist(err) {
			return false, err
		}
	}
	return false, nil
}

func (p *Provider) ListSpecs(_ context.Context, workspacePath string) ([]specprovider.Spec, error) {
	root := filepath.Join(workspacePath, "specs")
	var specs []specprovider.Spec

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".yaml" && ext != ".yml" {
			return nil
		}

		rel, _ := filepath.Rel(workspacePath, path)
		title := extractTitle(path, rel)
		specs = append(specs, specprovider.Spec{
			Path:   rel,
			Format: providerName,
			Title:  title,
		})
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("walk specs/: %w", err)
	}
	return specs, nil
}

func (p *Provider) ReadSpec(_ context.Context, workspacePath, specPath string) ([]byte, error) {
	full := filepath.Join(workspacePath, specPath)

	// Prevent path traversal: resolved path must be under workspace.
	resolved, err := filepath.Abs(full)
	if err != nil {
		return nil, fmt.Errorf("resolve path: %w", err)
	}
	wsAbs, err := filepath.Abs(workspacePath)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace: %w", err)
	}
	if !strings.HasPrefix(resolved, wsAbs+string(filepath.Separator)) {
		return nil, fmt.Errorf("path traversal rejected: %s", specPath)
	}

	return os.ReadFile(full) //nolint:gosec // Path validated above against traversal.
}

// extractTitle attempts to parse a title field from YAML content.
// Falls back to the filename without extension.
func extractTitle(absPath, relPath string) string {
	data, err := os.ReadFile(absPath) //nolint:gosec // Internal helper, path from Walk.
	if err != nil {
		return fileBaseName(relPath)
	}

	var doc struct {
		Title string `yaml:"title"`
	}
	if err := yaml.Unmarshal(data, &doc); err == nil && doc.Title != "" {
		return doc.Title
	}
	return fileBaseName(relPath)
}

func fileBaseName(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext)
}
