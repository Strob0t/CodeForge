package svn

import (
	"context"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Strob0t/CodeForge/internal/port/gitprovider"
)

// S3-F security review S4: the repository URL of a working copy comes from
// its agent-writable wc.db. svn sends the configured --username/--password
// to whatever server that URL names, so every command that contacts the
// repository runs only for URLs at or below the project's configured
// repository (same scheme, host and port; path inside the project).

const projectURL = "https://svn.example.com/repos/proj/trunk"

func newCredentialedProvider(wcURL, rootURL string) (*Provider, *fakeSVN) {
	fake := &fakeSVN{rootURL: rootURL, wcURL: wcURL, revision: "7"}
	p := NewProvider(nil)
	p.execCommand = fake.command
	p.username, p.password = "alice", "secret"
	p.repoURL = projectURL
	return p, fake
}

// contacted returns the subcommands that reach the repository.
func contacted(fake *fakeSVN) []string {
	var subs []string
	for _, sub := range fake.subcommands() {
		if slices.Contains([]string{"update", "switch", "ls", "log", "checkout"}, sub) {
			subs = append(subs, sub)
		}
	}
	return subs
}

func TestSVN_CredentialsGoOnlyToTheProjectsRepository(t *testing.T) {
	ctx := context.Background()
	for _, wcURL := range []string{
		"https://evil.example/repos/proj/trunk",         // another host
		"https://svn.example.com:8443/repos/proj/trunk", // another port
		"http://svn.example.com/repos/proj/trunk",       // another scheme
		"https://svn.example.com/repos/other/trunk",     // another project on the server
		"https://svn.example.com/repos/project/trunk",   // a sibling sharing the prefix
		"https://svn.example.com/repos/proj/../other",   // a traversal
	} {
		t.Run(wcURL, func(t *testing.T) {
			p, fake := newCredentialedProvider(wcURL, "https://svn.example.com/repos/proj")
			wc := newWorkingCopy(t)
			if err := p.Pull(ctx, wc); err == nil {
				t.Error("Pull succeeded")
			}
			if err := p.Checkout(ctx, wc, "feature"); err == nil {
				t.Error("Checkout succeeded")
			}
			if _, err := p.Status(ctx, wc); err != nil {
				t.Errorf("Status (local parts): %v", err)
			}
			if subs := contacted(fake); len(subs) != 0 {
				t.Fatalf("svn contacted the working copy's repository with the project's credentials: %v", subs)
			}
		})
	}
}

func TestSVN_URLsInsideTheProjectAreContacted(t *testing.T) {
	ctx := context.Background()
	// The agent lowered the repository root; branch URLs still come from the
	// project's configured URL.
	p, fake := newCredentialedProvider("https://SVN.example.com:443/repos/proj/branches/x", "https://svn.example.com")
	wc := newWorkingCopy(t)
	if err := p.Pull(ctx, wc); err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if _, err := p.ListBranches(ctx, wc); err != nil {
		t.Fatalf("ListBranches: %v", err)
	}
	if args := fake.callWith("ls"); len(args) == 0 || args[len(args)-1] != "https://svn.example.com/repos/proj/branches" {
		t.Fatalf("ls args = %v, want the project's branches URL", args)
	}
	if err := p.Checkout(ctx, wc, "feature"); err != nil {
		t.Fatalf("Checkout: %v", err)
	}
	if args := fake.callWith("switch"); len(args) == 0 || args[len(args)-1] != "https://svn.example.com/repos/proj/branches/feature" {
		t.Fatalf("switch args = %v, want the project's feature branch", args)
	}
	if err := p.Checkout(ctx, wc, "../../other"); err == nil {
		t.Fatal("Checkout of a branch outside the project succeeded")
	}
}

func TestSVN_CredentialsNeedTheProjectsRepositoryURL(t *testing.T) {
	ctx := context.Background()
	p, fake := newCredentialedProvider("https://svn.example.com/repos/proj/trunk", "https://svn.example.com/repos/proj")
	p.repoURL = ""
	if err := p.Pull(ctx, newWorkingCopy(t)); err == nil {
		t.Fatal("Pull with credentials but no project repository URL succeeded")
	}
	if subs := contacted(fake); len(subs) != 0 {
		t.Fatalf("svn contacted the repository: %v", subs)
	}
	// Clone goes only to the project's URL or below it.
	if err := p.Clone(ctx, "https://evil.example/x", filepath.Join(t.TempDir(), "wc")); err == nil {
		t.Fatal("Clone without the project URL succeeded")
	}
	p.repoURL = projectURL
	if err := p.Clone(ctx, "https://evil.example/x", filepath.Join(t.TempDir(), "wc")); err == nil {
		t.Fatal("Clone of another server's URL succeeded")
	}
	if err := p.Clone(ctx, projectURL, filepath.Join(t.TempDir(), "wc"), gitprovider.WithBranch("feature")); err != nil {
		t.Fatalf("Clone of the project's branch: %v", err)
	}
}

func TestSVN_RegistrationReadsTheProjectsRepositoryURL(t *testing.T) {
	gp, err := gitprovider.New("svn", map[string]string{"repo_url": projectURL})
	if err != nil {
		t.Fatal(err)
	}
	if p, ok := gp.(*Provider); !ok || p.repoURL != projectURL {
		t.Fatalf("provider = %+v", gp)
	}
}
