package gitprovider_test

import (
	"context"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/port/gitprovider"
)

type testProvider struct {
	name string
}

func (p *testProvider) Name() string { return p.name }
func (p *testProvider) Capabilities() gitprovider.Capabilities {
	return gitprovider.Capabilities{Clone: true}
}
func (p *testProvider) CloneURL(_ context.Context, _ string) (string, error) { return "", nil }
func (p *testProvider) ListRepos(_ context.Context) ([]string, error)        { return nil, nil }
func (p *testProvider) Clone(_ context.Context, _, _ string, _ ...gitprovider.CloneOption) error {
	return nil
}
func (p *testProvider) Status(_ context.Context, _ string) (*project.GitStatus, error) {
	return nil, nil
}
func (p *testProvider) Pull(_ context.Context, _ string) error { return nil }
func (p *testProvider) ListBranches(_ context.Context, _ string) ([]project.Branch, error) {
	return nil, nil
}
func (p *testProvider) Checkout(_ context.Context, _, _ string) error { return nil }

func TestRegisterAndNew(t *testing.T) {
	gitprovider.Register("test-git", func(_ map[string]string) (gitprovider.Provider, error) {
		return &testProvider{name: "test-git"}, nil
	})

	p, err := gitprovider.New("test-git", nil)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name() != "test-git" {
		t.Fatalf("expected test-git, got %s", p.Name())
	}
}

func TestNewUnknownProvider(t *testing.T) {
	_, err := gitprovider.New("nonexistent", nil)
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

func TestAvailable(t *testing.T) {
	names := gitprovider.Available()
	found := false
	for _, n := range names {
		if n == "test-git" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected test-git in available providers")
	}
}
