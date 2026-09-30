package svn

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
)

// S3 follow-up 1c: the SVN provider runs svn in agent-writable working
// copies. It must not use operator or agent controlled client config, must
// not contact a repository the agent pointed the working copy at through its
// metadata (another tenant's local repository), and must not fetch externals.

// fakeSVN records svn invocations and answers `info --show-item` queries.
type fakeSVN struct {
	mu       sync.Mutex
	calls    [][]string
	rootURL  string
	wcURL    string
	revision string
}

func (f *fakeSVN) command(ctx context.Context, _ string, args ...string) *exec.Cmd {
	out := ""
	if i := slices.Index(args, "--show-item"); i >= 0 && i+1 < len(args) {
		switch args[i+1] {
		case "repos-root-url":
			out = f.rootURL
		case "url":
			out = f.wcURL
		case "revision":
			out = f.revision
		}
	}
	cmd := exec.CommandContext(ctx, "printf", "%s", out) //nolint:gosec // test fake
	f.mu.Lock()
	f.calls = append(f.calls, args)
	f.mu.Unlock()
	return cmd
}

func (f *fakeSVN) subcommands() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var subs []string
	for _, args := range f.calls {
		subs = append(subs, subcommand(args))
	}
	return subs
}

// subcommand returns the first argument that is not a global option.
func subcommand(args []string) string {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--config-dir", "--username", "--password":
			i++
		case "--non-interactive", "--no-auth-cache":
		default:
			return args[i]
		}
	}
	return ""
}

func (f *fakeSVN) callWith(sub string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, args := range f.calls {
		if subcommand(args) == sub {
			return args
		}
	}
	return nil
}

// newWorkingCopy creates a directory that looks like an SVN working copy.
func newWorkingCopy(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".svn", "pristine"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".svn", "wc.db"), []byte("sqlite"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func newFakeProvider(rootURL string) (*Provider, *fakeSVN) {
	fake := &fakeSVN{rootURL: rootURL, wcURL: rootURL + "/trunk", revision: "7"}
	p := NewProvider(nil)
	p.execCommand = fake.command
	return p, fake
}

func TestSVN_EveryInvocationIsHardened(t *testing.T) {
	t.Setenv("SVN_EDITOR", "/tmp/evil")
	t.Setenv("SVN_SSH", "/tmp/evil")
	t.Setenv("EDITOR", "/tmp/evil")
	t.Setenv("VISUAL", "/tmp/evil")
	t.Setenv("SVN_MERGE", "/tmp/evil")
	p, fake := newFakeProvider("https://svn.example.com/repo")
	if _, err := p.runSVN(context.Background(), "", "info"); err != nil {
		t.Fatalf("runSVN: %v", err)
	}
	args := fake.callWith("info")
	for _, want := range []string{"--non-interactive", "--no-auth-cache"} {
		if !slices.Contains(args, want) {
			t.Errorf("args %v miss %s", args, want)
		}
	}
	i := slices.Index(args, "--config-dir")
	if i < 0 || i+1 >= len(args) {
		t.Fatalf("args %v miss --config-dir", args)
	}
	info, err := os.Lstat(args[i+1])
	if err != nil || !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("config dir %s: %v, mode %v; want a private directory", args[i+1], err, info)
	}
	if home, _ := os.UserHomeDir(); home != "" && strings.HasPrefix(args[i+1], home) {
		t.Fatalf("config dir %s is in the home directory", args[i+1])
	}

	for _, kv := range p.environment() {
		for _, dropped := range []string{"SVN_EDITOR=", "SVN_SSH=", "EDITOR=", "VISUAL=", "SVN_MERGE="} {
			if strings.HasPrefix(kv, dropped) {
				t.Errorf("environment keeps %s", kv)
			}
		}
	}
}

func TestSVN_UpdateSwitchAndCheckoutIgnoreExternals(t *testing.T) {
	ctx := context.Background()
	p, fake := newFakeProvider("https://svn.example.com/repo")
	wc := newWorkingCopy(t)
	if err := p.Pull(ctx, wc); err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if err := p.Checkout(ctx, wc, "feature"); err != nil {
		t.Fatalf("Checkout: %v", err)
	}
	if err := p.Clone(ctx, "https://svn.example.com/repo/trunk", filepath.Join(t.TempDir(), "new")); err != nil {
		t.Fatalf("Clone: %v", err)
	}
	for _, sub := range []string{"update", "switch", "checkout"} {
		args := fake.callWith(sub)
		if args == nil {
			t.Fatalf("svn %s not run (calls %v)", sub, fake.subcommands())
		}
		if !slices.Contains(args, "--ignore-externals") {
			t.Errorf("svn %s args %v miss --ignore-externals", sub, args)
		}
	}
}

func TestSVN_RefusesWorkingCopyMetadataTricks(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name  string
		setup func(t *testing.T) string
	}{
		{name: "no .svn (svn would use a parent working copy)", setup: func(t *testing.T) string { return t.TempDir() }},
		{name: ".svn is a symlink to another working copy", setup: func(t *testing.T) string {
			other := newWorkingCopy(t)
			dir := t.TempDir()
			if err := os.Symlink(filepath.Join(other, ".svn"), filepath.Join(dir, ".svn")); err != nil {
				t.Fatal(err)
			}
			return dir
		}},
		{name: "wc.db is a symlink", setup: func(t *testing.T) string {
			other := newWorkingCopy(t)
			dir := newWorkingCopy(t)
			db := filepath.Join(dir, ".svn", "wc.db")
			if err := os.Remove(db); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(other, ".svn", "wc.db"), db); err != nil {
				t.Fatal(err)
			}
			return dir
		}},
		{name: "pristine store is a symlink", setup: func(t *testing.T) string {
			other := newWorkingCopy(t)
			dir := newWorkingCopy(t)
			pristine := filepath.Join(dir, ".svn", "pristine")
			if err := os.Remove(pristine); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(other, ".svn", "pristine"), pristine); err != nil {
				t.Fatal(err)
			}
			return dir
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, fake := newFakeProvider("https://svn.example.com/repo")
			dir := tc.setup(t)
			if _, err := p.Status(ctx, dir); err == nil {
				t.Error("Status succeeded")
			}
			if err := p.Pull(ctx, dir); err == nil {
				t.Error("Pull succeeded")
			}
			if _, err := p.ListBranches(ctx, dir); err == nil {
				t.Error("ListBranches succeeded")
			}
			if err := p.Checkout(ctx, dir, "trunk"); err == nil {
				t.Error("Checkout succeeded")
			}
			if subs := fake.subcommands(); len(subs) != 0 {
				t.Fatalf("svn ran on a refused working copy: %v", subs)
			}
		})
	}
}

func TestSVN_NetworkOperationsRefuseLocalRepositories(t *testing.T) {
	ctx := context.Background()
	for _, root := range []string{"file:///srv/other-tenant/repo", "svn+evil://host/repo", "FILE:///srv/x"} {
		t.Run(root, func(t *testing.T) {
			p, fake := newFakeProvider(root)
			wc := newWorkingCopy(t)
			if err := p.Pull(ctx, wc); err == nil {
				t.Error("Pull from a local repository set in the working copy succeeded")
			}
			if err := p.Checkout(ctx, wc, "trunk"); err == nil {
				t.Error("Checkout succeeded")
			}
			if _, err := p.ListBranches(ctx, wc); err == nil {
				t.Error("ListBranches succeeded")
			}
			status, err := p.Status(ctx, wc)
			if err != nil {
				t.Fatalf("Status (local only): %v", err)
			}
			if status.CommitHash != "7" {
				t.Fatalf("status revision = %q", status.CommitHash)
			}
			for _, sub := range fake.subcommands() {
				if slices.Contains([]string{"update", "switch", "ls", "log", "checkout"}, sub) {
					t.Fatalf("svn %s contacted the repository the working copy points at (calls %v)", sub, fake.subcommands())
				}
			}
		})
	}
	t.Run("remote repository allowed", func(t *testing.T) {
		for _, root := range []string{"https://svn.example.com/repo", "http://svn.example.com/repo", "svn://svn.example.com/repo", "svn+ssh://svn.example.com/repo"} {
			p, fake := newFakeProvider(root)
			if err := p.Pull(ctx, newWorkingCopy(t)); err != nil {
				t.Fatalf("Pull from %s: %v", root, err)
			}
			if fake.callWith("update") == nil {
				t.Fatalf("no update for %s", root)
			}
		}
	})
	t.Run("file repositories allowed by the operator", func(t *testing.T) {
		p, fake := newFakeProvider("file:///srv/svn/repo")
		p.allowFileURLs = true
		if err := p.Pull(ctx, newWorkingCopy(t)); err != nil {
			t.Fatalf("Pull: %v", err)
		}
		if fake.callWith("update") == nil {
			t.Fatal("no update")
		}
	})
}
