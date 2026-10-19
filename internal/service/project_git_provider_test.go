package service

import (
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/project"
)

// S3-F security review S4: the git provider learns the project's configured
// repository URL (config key repo_url) from the project, never from the
// project-editable config, so the SVN provider can refuse URLs outside it.
// (No provider is registered here: the registry is global, and a registered
// test provider would switch on provider validation in other tests.)
func TestGitProviderConfig_PassesTheProjectsRepositoryURL(t *testing.T) {
	p := &project.Project{
		Provider: "svn",
		RepoURL:  "https://svn.example.com/repos/proj/trunk",
		Config:   map[string]string{"repo_url": "https://evil.example/x", "username": "alice"},
	}
	cfg := gitProviderConfig(p)
	if cfg["repo_url"] != p.RepoURL || cfg["username"] != "alice" {
		t.Fatalf("provider config = %v, want repo_url from the project and the other keys", cfg)
	}
	if p.Config["repo_url"] != "https://evil.example/x" {
		t.Fatal("the project's own config was modified")
	}
	if cfg := gitProviderConfig(&project.Project{RepoURL: "https://x.example/r"}); cfg["repo_url"] != "https://x.example/r" {
		t.Fatalf("provider config without project config = %v", cfg)
	}
}
