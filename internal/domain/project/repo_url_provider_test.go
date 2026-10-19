package project

import (
	"errors"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain"
)

// KI-189 (R6-12): repository URLs are validated per provider. The SVN
// provider contacts http, https, svn and svn+ssh repositories, which the
// git-only check refused; git providers keep https:// and git@host:path.
func TestValidateRepoURL_PerProvider(t *testing.T) {
	tests := []struct {
		provider, url string
		wantErr       bool
	}{
		{"", "https://github.com/user/repo.git", false},
		{"", "git@github.com:user/repo.git", false},
		{"github-api", "https://github.com/user/repo", false},
		{"", "http://example.com/repo.git", true},
		{"", "svn://svn.example.com/repo", true},
		{"local", "svn+ssh://svn.example.com/repo", true},
		{"", "not-a-url", true},

		{"svn", "https://svn.example.com/repo/trunk", false},
		{"svn", "http://svn.example.com/repo", false},
		{"svn", "svn://svn.example.com/repo", false},
		{"svn", "svn+ssh://user@svn.example.com/repo", false},
		{"svn", "SVN://svn.example.com/repo", false},
		{"svn", "file:///srv/svn/repo", true}, // local repositories: checkCloneSource and svn.allow_file_urls
		{"svn", "git@github.com:user/repo.git", true},
		{"svn", "svn:///repo", true}, // no host
		{"svn", "svn://", true},
		{"svn", "ftp://svn.example.com/repo", true},
		{"svn", "not-a-url", true},
		{"svn", "svn://svn.example.com/repo\n", true},
	}
	for _, tt := range tests {
		t.Run(tt.provider+" "+tt.url, func(t *testing.T) {
			err := ValidateRepoURL(tt.provider, tt.url)
			if tt.wantErr != (err != nil) {
				t.Fatalf("ValidateRepoURL(%q, %q) = %v, want error %t", tt.provider, tt.url, err, tt.wantErr)
			}
			if err != nil && !errors.Is(err, domain.ErrValidation) {
				t.Fatalf("error %v is not a validation error", err)
			}
		})
	}
}

func TestValidateCreateRequest_SVNURL(t *testing.T) {
	providers := []string{"local", "github-api", "svn"}
	if err := ValidateCreateRequest(&CreateRequest{RepoURL: "svn://svn.example.com/repo", Provider: "svn"}, providers); err != nil {
		t.Fatalf("an svn:// URL of an SVN project: %v", err)
	}
	if err := ValidateCreateRequest(&CreateRequest{RepoURL: "svn://svn.example.com/repo", Provider: "local"}, providers); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("an svn:// URL of a git project = %v, want a validation error", err)
	}
}

// An update checks the URL against the provider it sets, or the stored one.
func TestValidateUpdateRequest_URLPerProvider(t *testing.T) {
	svnURL, svn, local := "svn://svn.example.com/repo", "svn", "local"
	tests := []struct {
		name           string
		req            UpdateRequest
		storedProvider string
		wantErr        bool
	}{
		{"stored svn", UpdateRequest{RepoURL: &svnURL}, "svn", false},
		{"stored git", UpdateRequest{RepoURL: &svnURL}, "", true},
		{"set to svn", UpdateRequest{RepoURL: &svnURL, Provider: &svn}, "", false},
		{"set to git", UpdateRequest{RepoURL: &svnURL, Provider: &local}, "svn", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateUpdateRequest(tt.req, tt.storedProvider)
			if tt.wantErr != (err != nil) {
				t.Fatalf("ValidateUpdateRequest = %v, want error %t", err, tt.wantErr)
			}
		})
	}
}
