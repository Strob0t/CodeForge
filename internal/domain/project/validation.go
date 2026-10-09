package project

import (
	"fmt"
	neturl "net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/Strob0t/CodeForge/internal/domain"
)

// sshURLPattern matches git SSH URLs like git@host:user/repo.git
var sshURLPattern = regexp.MustCompile(`^git@[^:]+:.+`)

// ValidateCreateRequest validates the fields of a project creation request.
// availableProviders should be the list from gitprovider.Available() to avoid import cycles.
func ValidateCreateRequest(req *CreateRequest, availableProviders []string) error {
	// Name: non-empty, max 255 chars, no control characters.
	if req.Name == "" && req.RepoURL == "" {
		return fmt.Errorf("name is required (or provide repo_url): %w", domain.ErrValidation)
	}
	if req.Name != "" {
		if len(req.Name) > 255 {
			return fmt.Errorf("name exceeds 255 characters: %w", domain.ErrValidation)
		}
		for _, r := range req.Name {
			if unicode.IsControl(r) {
				return fmt.Errorf("name contains control characters: %w", domain.ErrValidation)
			}
		}
	}

	// Provider: if non-empty, must be in available list.
	if req.Provider != "" && len(availableProviders) > 0 {
		found := false
		for _, p := range availableProviders {
			if p == req.Provider {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("unknown provider %q: %w", req.Provider, domain.ErrValidation)
		}
	}

	// RepoURL: if non-empty, a URL the provider can use.
	if req.RepoURL != "" {
		if err := ValidateRepoURL(req.Provider, req.RepoURL); err != nil {
			return err
		}
	}

	// LocalPath: if non-empty, must be absolute, exist, and be a directory.
	// Mutually exclusive with RepoURL.
	if req.LocalPath != "" {
		if req.RepoURL != "" {
			return fmt.Errorf("local_path and repo_url are mutually exclusive: %w", domain.ErrValidation)
		}
		clean := filepath.Clean(req.LocalPath)
		if !filepath.IsAbs(clean) {
			return fmt.Errorf("local_path must be absolute: %w", domain.ErrValidation)
		}
		info, err := os.Stat(clean)
		if err != nil {
			return fmt.Errorf("local_path does not exist: %w", domain.ErrValidation)
		}
		if !info.IsDir() {
			return fmt.Errorf("local_path is not a directory: %w", domain.ErrValidation)
		}
	}

	// Description: max 2000 chars.
	if len(req.Description) > 2000 {
		return fmt.Errorf("description exceeds 2000 characters: %w", domain.ErrValidation)
	}

	config := make(map[string]*string, len(req.Config))
	for key, value := range req.Config {
		config[key] = &value
	}
	return validateGateCommands(config)
}

// ValidateUpdateRequest validates the fields of a project update request;
// a repo_url is checked against the provider the request sets, or else
// storedProvider, the project's.
func ValidateUpdateRequest(req UpdateRequest, storedProvider string) error {
	if req.Name != nil {
		if *req.Name == "" {
			return fmt.Errorf("name cannot be empty: %w", domain.ErrValidation)
		}
		if len(*req.Name) > 255 {
			return fmt.Errorf("name exceeds 255 characters: %w", domain.ErrValidation)
		}
		for _, r := range *req.Name {
			if unicode.IsControl(r) {
				return fmt.Errorf("name contains control characters: %w", domain.ErrValidation)
			}
		}
	}
	if req.Description != nil && len(*req.Description) > 2000 {
		return fmt.Errorf("description exceeds 2000 characters: %w", domain.ErrValidation)
	}
	if req.RepoURL != nil && *req.RepoURL != "" {
		provider := storedProvider
		if req.Provider != nil {
			provider = *req.Provider
		}
		if err := ValidateRepoURL(provider, *req.RepoURL); err != nil {
			return err
		}
	}
	return validateGateCommands(req.Config)
}

// svnSchemes are the remote repository URL schemes of the SVN provider
// (its own check, svn.allowedSchemes, also allows them). Local file://
// repositories are no project URL: checkCloneSource and the operator key
// svn.allow_file_urls decide about those.
var svnSchemes = map[string]bool{"http": true, "https": true, "svn": true, "svn+ssh": true}

// ValidateRepoURL checks that url is a remote repository URL of provider
// (KI-189): an SVN project takes http://, https://, svn:// and svn+ssh://
// URLs with a host, every other (git) provider https:// or git@host:path.
func ValidateRepoURL(provider, url string) error {
	if provider != "svn" {
		if !IsValidRepoURL(url) {
			return fmt.Errorf("repo_url must start with https:// or match git@host:path format: %w", domain.ErrValidation)
		}
		return nil
	}
	u, err := neturl.Parse(url)
	if err != nil || !svnSchemes[strings.ToLower(u.Scheme)] || u.Host == "" || u.Hostname() == "" {
		return fmt.Errorf("repo_url of an SVN project must be an http://, https://, svn:// or svn+ssh:// URL with a host: %w", domain.ErrValidation)
	}
	return nil
}

// IsValidRepoURL checks that the URL is either HTTPS or a git SSH URL.
func IsValidRepoURL(url string) bool {
	if len(url) > 7 && url[:8] == "https://" {
		return true
	}
	return sshURLPattern.MatchString(url)
}
