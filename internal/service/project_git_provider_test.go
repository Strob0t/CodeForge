package service

import (
	"sync"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/port/gitprovider"
)

// S3-F security review S4: the git provider learns the project's configured
// repository URL (config key repo_url) from the project, never from the
// project-editable config, so the SVN provider can refuse URLs outside it.

var (
	registerCaptureOnce sync.Once
	capturedConfig      map[string]string
)

func TestResolveGitProvider_PassesTheProjectsRepositoryURL(t *testing.T) {
	registerCaptureOnce.Do(func() {
		gitprovider.Register("capture-config-test", func(cfg map[string]string) (gitprovider.Provider, error) {
			capturedConfig = cfg
			return nil, nil
		})
	})
	p := &project.Project{
		Provider: "capture-config-test",
		RepoURL:  "https://svn.example.com/repos/proj/trunk",
		Config:   map[string]string{"repo_url": "https://evil.example/x", "username": "alice"},
	}
	if _, err := resolveGitProvider(p); err != nil {
		t.Fatal(err)
	}
	if capturedConfig["repo_url"] != p.RepoURL || capturedConfig["username"] != "alice" {
		t.Fatalf("provider config = %v, want repo_url from the project and the other keys", capturedConfig)
	}
	if p.Config["repo_url"] != "https://evil.example/x" {
		t.Fatal("the project's own config was modified")
	}
}
