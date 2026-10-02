// Package autospec implements a specprovider.Provider for the Autospec format.
// It detects and reads YAML spec files from the specs/ directory.
package autospec

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"

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
	for _, name := range []string{"spec.yaml", "spec.yml"} {
		info, err := workspacefs.StatAt(workspacePath, path.Join("specs", name))
		if err == nil && !info.IsDir() {
			return true, nil
		}
		if err != nil && !errors.Is(err, fs.ErrNotExist) && !errors.Is(err, workspacefs.ErrLeavesWorkspace) {
			return false, err
		}
	}
	return false, nil
}

func (p *Provider) ListSpecs(_ context.Context, workspacePath string) ([]specprovider.Spec, error) {
	// The workspace is read through workspacefs (KI-95): the walk never
	// descends into a symlink, and only regular files inside it are listed.
	ws, err := workspacefs.Open(workspacePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = ws.Close() }()
	var specs []specprovider.Spec

	err = ws.WalkDir("specs", func(rel string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		ext := strings.ToLower(path.Ext(rel))
		if ext != ".yaml" && ext != ".yml" {
			return nil
		}
		if info, statErr := ws.Stat(rel); statErr != nil || !info.Mode().IsRegular() {
			return nil
		}

		specs = append(specs, specprovider.Spec{
			Path:   rel,
			Format: providerName,
			Title:  extractTitle(ws, rel),
		})
		return nil
	})
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, workspacefs.ErrLeavesWorkspace) {
			return nil, nil
		}
		return nil, fmt.Errorf("walk specs/: %w", err)
	}
	return specs, nil
}

func (p *Provider) ReadSpec(_ context.Context, workspacePath, specPath string) ([]byte, error) {
	// Resolved inside the workspace by os.Root: "..", absolute paths and
	// symlinks that leave the workspace are refused.
	return workspacefs.ReadFileAt(workspacePath, specPath, specprovider.MaxSpecBytes)
}

// extractTitle attempts to parse a title field from YAML content.
// Falls back to the filename without extension.
func extractTitle(ws *workspacefs.Root, relPath string) string {
	data, _, err := ws.ReadFile(relPath, specprovider.MaxSpecBytes)
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

func fileBaseName(name string) string {
	base := path.Base(name)
	return strings.TrimSuffix(base, path.Ext(base))
}
