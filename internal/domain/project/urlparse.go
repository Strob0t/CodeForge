package project

import (
	"fmt"
	"net/url"
	"path"
	"strings"
)

// ParsedRepoURL contains the components extracted from a git repository URL.
type ParsedRepoURL struct {
	Owner    string `json:"owner"`
	Repo     string `json:"repo"`
	Provider string `json:"provider"`
	Host     string `json:"host"`
}

// knownProviders maps hostnames to provider names.
var knownProviders = map[string]string{
	"github.com":    "github",
	"gitlab.com":    "gitlab",
	"bitbucket.org": "bitbucket",
}

// ParseRepoURL extracts owner, repo name, and provider from a git URL.
// Supports HTTPS URLs (https://github.com/org/repo) and SSH URLs (git@github.com:org/repo.git).
func ParseRepoURL(rawURL string) (*ParsedRepoURL, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, fmt.Errorf("empty URL")
	}

	// SSH format: git@host:owner/repo.git
	if strings.HasPrefix(rawURL, "git@") {
		return parseSSHURL(rawURL)
	}

	// HTTPS format: https://host/owner/repo[.git]
	if strings.HasPrefix(rawURL, "https://") || strings.HasPrefix(rawURL, "http://") {
		return parseHTTPSURL(rawURL)
	}

	return nil, fmt.Errorf("unsupported URL scheme: must start with https://, http://, or git@")
}

func parseSSHURL(rawURL string) (*ParsedRepoURL, error) {
	// git@github.com:owner/repo.git
	withoutPrefix := strings.TrimPrefix(rawURL, "git@")
	colonIdx := strings.Index(withoutPrefix, ":")
	if colonIdx < 0 {
		return nil, fmt.Errorf("invalid SSH URL: missing colon separator")
	}

	host := withoutPrefix[:colonIdx]
	pathPart := withoutPrefix[colonIdx+1:]
	pathPart = strings.TrimSuffix(pathPart, ".git")

	parts := strings.SplitN(pathPart, "/", 3)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return nil, fmt.Errorf("invalid SSH URL: expected owner/repo after host")
	}

	return &ParsedRepoURL{
		Owner:    parts[0],
		Repo:     parts[1],
		Provider: providerFromHost(host),
		Host:     host,
	}, nil
}

func parseHTTPSURL(rawURL string) (*ParsedRepoURL, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("invalid URL: missing host")
	}

	// Clean the path: /owner/repo[.git]
	p := strings.TrimSuffix(path.Clean(u.Path), ".git")
	p = strings.Trim(p, "/")

	parts := strings.SplitN(p, "/", 3)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return nil, fmt.Errorf("invalid URL: expected /owner/repo path")
	}

	return &ParsedRepoURL{
		Owner:    parts[0],
		Repo:     parts[1],
		Provider: providerFromHost(u.Hostname()),
		Host:     u.Hostname(),
	}, nil
}

// providerFromHost returns the provider name for a known host, or empty string.
func providerFromHost(host string) string {
	if p, ok := knownProviders[strings.ToLower(host)]; ok {
		return p
	}
	// Check for self-hosted instances with common subdomains.
	lower := strings.ToLower(host)
	if strings.Contains(lower, "gitlab") {
		return "gitlab"
	}
	if strings.Contains(lower, "gitea") || strings.Contains(lower, "forgejo") {
		return "gitea"
	}
	return ""
}
