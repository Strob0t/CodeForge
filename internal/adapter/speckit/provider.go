// Package speckit implements a specprovider.Provider for the Spec Kit format.
// It detects and reads Markdown spec files from the .specify/ directory.
package speckit

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"github.com/Strob0t/CodeForge/internal/port/specprovider"
	"github.com/Strob0t/CodeForge/internal/workspacefs"
)

const providerName = "speckit"

// Provider implements specprovider.Provider for Spec Kit (.specify/) format.
type Provider struct{}

func (p *Provider) Name() string { return providerName }

func (p *Provider) Capabilities() specprovider.Capabilities {
	return specprovider.Capabilities{Read: true, Write: false, Sync: false}
}

func (p *Provider) Detect(_ context.Context, workspacePath string) (bool, error) {
	info, err := workspacefs.StatAt(workspacePath, ".specify")
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, workspacefs.ErrLeavesWorkspace) {
			return false, nil
		}
		return false, err
	}
	return info.IsDir(), nil
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

	err = ws.WalkDir(".specify", func(rel string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		ext := strings.ToLower(path.Ext(rel))
		if ext != ".md" && ext != ".markdown" {
			return nil
		}
		if info, statErr := ws.Stat(rel); statErr != nil || !info.Mode().IsRegular() {
			return nil
		}

		specs = append(specs, specprovider.Spec{
			Path:   rel,
			Format: providerName,
			Title:  extractMarkdownTitle(ws, rel),
		})
		return nil
	})
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, workspacefs.ErrLeavesWorkspace) {
			return nil, nil
		}
		return nil, fmt.Errorf("walk .specify/: %w", err)
	}
	return specs, nil
}

func (p *Provider) ReadSpec(_ context.Context, workspacePath, specPath string) ([]byte, error) {
	// Resolved inside the workspace by os.Root: "..", absolute paths and
	// symlinks that leave the workspace are refused.
	return workspacefs.ReadFileAt(workspacePath, specPath, specprovider.MaxSpecBytes)
}

// extractMarkdownTitle reads the first H1 heading (# Title) from a Markdown file.
// Falls back to the filename without extension.
func extractMarkdownTitle(ws *workspacefs.Root, relPath string) string {
	f, _, err := ws.OpenFile(relPath)
	if err != nil {
		return fileBaseName(relPath)
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if title, ok := strings.CutPrefix(line, "# "); ok {
			return strings.TrimSpace(title)
		}
	}
	return fileBaseName(relPath)
}

func fileBaseName(name string) string {
	base := path.Base(name)
	return strings.TrimSuffix(base, path.Ext(base))
}
