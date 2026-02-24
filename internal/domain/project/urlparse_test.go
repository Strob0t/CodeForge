package project

import (
	"testing"
)

func TestParseRepoURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		wantErr  bool
		owner    string
		repo     string
		provider string
	}{
		{
			name:     "github https",
			url:      "https://github.com/octocat/hello-world",
			owner:    "octocat",
			repo:     "hello-world",
			provider: "github",
		},
		{
			name:     "github https with .git suffix",
			url:      "https://github.com/octocat/hello-world.git",
			owner:    "octocat",
			repo:     "hello-world",
			provider: "github",
		},
		{
			name:     "github ssh",
			url:      "git@github.com:octocat/hello-world.git",
			owner:    "octocat",
			repo:     "hello-world",
			provider: "github",
		},
		{
			name:     "gitlab https",
			url:      "https://gitlab.com/group/project",
			owner:    "group",
			repo:     "project",
			provider: "gitlab",
		},
		{
			name:     "gitlab ssh",
			url:      "git@gitlab.com:group/project.git",
			owner:    "group",
			repo:     "project",
			provider: "gitlab",
		},
		{
			name:     "bitbucket https",
			url:      "https://bitbucket.org/team/repo",
			owner:    "team",
			repo:     "repo",
			provider: "bitbucket",
		},
		{
			name:     "self-hosted gitea",
			url:      "https://gitea.example.com/org/repo",
			owner:    "org",
			repo:     "repo",
			provider: "gitea",
		},
		{
			name:     "self-hosted gitlab",
			url:      "https://gitlab.company.com/team/service",
			owner:    "team",
			repo:     "service",
			provider: "gitlab",
		},
		{
			name:     "unknown host",
			url:      "https://custom-git.example.com/org/repo",
			owner:    "org",
			repo:     "repo",
			provider: "",
		},
		{
			name:     "trailing slash",
			url:      "https://github.com/octocat/hello-world/",
			owner:    "octocat",
			repo:     "hello-world",
			provider: "github",
		},
		{
			name:    "empty url",
			url:     "",
			wantErr: true,
		},
		{
			name:    "no path",
			url:     "https://github.com/",
			wantErr: true,
		},
		{
			name:    "only owner",
			url:     "https://github.com/octocat",
			wantErr: true,
		},
		{
			name:    "invalid ssh no colon",
			url:     "git@github.com/octocat/repo",
			wantErr: true,
		},
		{
			name:    "unsupported scheme",
			url:     "ftp://github.com/octocat/repo",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseRepoURL(tt.url)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Owner != tt.owner {
				t.Errorf("owner: got %q, want %q", result.Owner, tt.owner)
			}
			if result.Repo != tt.repo {
				t.Errorf("repo: got %q, want %q", result.Repo, tt.repo)
			}
			if result.Provider != tt.provider {
				t.Errorf("provider: got %q, want %q", result.Provider, tt.provider)
			}
		})
	}
}
