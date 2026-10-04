// Package speckit implements a specprovider.Provider for the Spec Kit format.
// It detects and reads Markdown spec files from the .specify/ directory.
package speckit

import (
	"bufio"
	"context"
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
	return specprovider.HasDir(workspacePath, ".specify")
}

func (p *Provider) ListSpecs(_ context.Context, workspacePath string) ([]specprovider.Spec, error) {
	return specprovider.ListFiles(workspacePath, ".specify", providerName, []string{".md", ".markdown"}, extractMarkdownTitle)
}

func (p *Provider) ReadSpec(_ context.Context, workspacePath, specPath string) ([]byte, error) {
	return specprovider.ReadFile(workspacePath, specPath)
}

// extractMarkdownTitle reads the first H1 heading (# Title) from a Markdown file.
// Falls back to the filename without extension.
func extractMarkdownTitle(ws *workspacefs.Root, relPath string) string {
	f, _, err := ws.OpenFile(relPath)
	if err != nil {
		return specprovider.FileBaseName(relPath)
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if title, ok := strings.CutPrefix(line, "# "); ok {
			return strings.TrimSpace(title)
		}
	}
	return specprovider.FileBaseName(relPath)
}
