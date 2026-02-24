package project

import (
	"fmt"
	"regexp"
	"unicode"

	"github.com/Strob0t/CodeForge/internal/domain"
)

// sshURLPattern matches git SSH URLs like git@host:user/repo.git
var sshURLPattern = regexp.MustCompile(`^git@[^:]+:.+`)

// ValidateCreateRequest validates the fields of a project creation request.
// availableProviders should be the list from gitprovider.Available() to avoid import cycles.
func ValidateCreateRequest(req CreateRequest, availableProviders []string) error {
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

	// RepoURL: if non-empty, must be HTTPS or SSH.
	if req.RepoURL != "" {
		if !IsValidRepoURL(req.RepoURL) {
			return fmt.Errorf("repo_url must start with https:// or match git@host:path format: %w", domain.ErrValidation)
		}
	}

	// Description: max 2000 chars.
	if len(req.Description) > 2000 {
		return fmt.Errorf("description exceeds 2000 characters: %w", domain.ErrValidation)
	}

	return nil
}

// ValidateUpdateRequest validates the fields of a project update request.
func ValidateUpdateRequest(req UpdateRequest) error {
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
		if !IsValidRepoURL(*req.RepoURL) {
			return fmt.Errorf("repo_url must start with https:// or match git@host:path format: %w", domain.ErrValidation)
		}
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
