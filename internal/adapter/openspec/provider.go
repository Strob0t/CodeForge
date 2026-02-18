// Package openspec implements a specprovider.Provider for the OpenSpec format.
// It detects and reads YAML/JSON spec files from the openspec/ directory.
package openspec

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Strob0t/CodeForge/internal/port/specprovider"
)

const providerName = "openspec"

// Provider implements specprovider.Provider for OpenSpec format.
type Provider struct{}

func (p *Provider) Name() string { return providerName }

func (p *Provider) Capabilities() specprovider.Capabilities {
	return specprovider.Capabilities{Read: true, Write: false, Sync: false}
}

func (p *Provider) Detect(_ context.Context, workspacePath string) (bool, error) {
	info, err := os.Stat(filepath.Join(workspacePath, "openspec"))
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return info.IsDir(), nil
}

func (p *Provider) ListSpecs(_ context.Context, workspacePath string) ([]specprovider.Spec, error) {
	root := filepath.Join(workspacePath, "openspec")
	var specs []specprovider.Spec

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".yaml" && ext != ".yml" && ext != ".json" {
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
		return nil, fmt.Errorf("walk openspec/: %w", err)
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

// extractTitle attempts to parse a title from YAML front-matter.
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
