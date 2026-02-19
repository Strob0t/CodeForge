// Package speckit implements a specprovider.Provider for the Spec Kit format.
// It detects and reads Markdown spec files from the .specify/ directory.
package speckit

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Strob0t/CodeForge/internal/port/specprovider"
)

const providerName = "speckit"

// Provider implements specprovider.Provider for Spec Kit (.specify/) format.
type Provider struct{}

func (p *Provider) Name() string { return providerName }

func (p *Provider) Capabilities() specprovider.Capabilities {
	return specprovider.Capabilities{Read: true, Write: false, Sync: false}
}

func (p *Provider) Detect(_ context.Context, workspacePath string) (bool, error) {
	info, err := os.Stat(filepath.Join(workspacePath, ".specify"))
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return info.IsDir(), nil
}

func (p *Provider) ListSpecs(_ context.Context, workspacePath string) ([]specprovider.Spec, error) {
	root := filepath.Join(workspacePath, ".specify")
	var specs []specprovider.Spec

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".md" && ext != ".markdown" {
			return nil
		}

		rel, _ := filepath.Rel(workspacePath, path)
		title := extractMarkdownTitle(path, rel)
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
		return nil, fmt.Errorf("walk .specify/: %w", err)
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

// extractMarkdownTitle reads the first H1 heading (# Title) from a Markdown file.
// Falls back to the filename without extension.
func extractMarkdownTitle(absPath, relPath string) string {
	f, err := os.Open(absPath) //nolint:gosec // Internal helper, path from Walk.
	if err != nil {
		return fileBaseName(relPath)
	}
	defer f.Close() //nolint:errcheck // Best-effort read for title extraction.

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if title, ok := strings.CutPrefix(line, "# "); ok {
			return strings.TrimSpace(title)
		}
	}
	return fileBaseName(relPath)
}

func fileBaseName(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext)
}
