package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// S3 follow-up 1f: a project may adopt (local_path, POST /adopt) or clone
// from a local directory only inside its own tenant's workspace directory,
// or - for admins - inside an operator-configured adopt root. Another
// tenant's workspace, the rest of the workspace root and symlinks leading
// there are refused.

type adoptEnv struct {
	svc     *ProjectService
	root    string // workspace root
	own     string // a directory of tenant-a
	other   string // a directory of tenant-b
	extra   string // a directory in the adopt root
	outside string
	link    string // in tenant-a's area, pointing at tenant-b's directory
}

func newAdoptEnv(t *testing.T) *adoptEnv {
	t.Helper()
	root := t.TempDir()
	adoptRoot := t.TempDir()
	e := &adoptEnv{
		root:    root,
		own:     filepath.Join(root, "tenant-a", "work"),
		other:   filepath.Join(root, "tenant-b", "secret"),
		extra:   filepath.Join(adoptRoot, "e2e-workspace"),
		outside: t.TempDir(),
		link:    filepath.Join(root, "tenant-a", "link"),
	}
	for _, d := range []string{e.own, e.other, e.extra} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(e.other, e.link); err != nil {
		t.Fatal(err)
	}
	store := &mockStore{projects: []project.Project{{ID: "p1", Name: "Alpha"}}}
	e.svc = NewProjectService(store, root)
	e.svc.SetAdoptRoots([]string{adoptRoot})
	return e
}

func TestAdopt_OnlyTheTenantsAreaOrAdminAdoptRoots(t *testing.T) {
	tests := []struct {
		name    string
		path    func(e *adoptEnv) string
		admin   bool
		wantErr bool
	}{
		{name: "own tenant's directory", path: func(e *adoptEnv) string { return e.own }},
		{name: "own tenant's directory as admin", path: func(e *adoptEnv) string { return e.own }, admin: true},
		{name: "another tenant's directory", path: func(e *adoptEnv) string { return e.other }, wantErr: true},
		{name: "another tenant's directory as admin", path: func(e *adoptEnv) string { return e.other }, admin: true, wantErr: true},
		{name: "the tenant directory itself", path: func(e *adoptEnv) string { return filepath.Join(e.root, "tenant-a") }, wantErr: true},
		{name: "the workspace root", path: func(e *adoptEnv) string { return e.root }, admin: true, wantErr: true},
		{name: "symlink to another tenant", path: func(e *adoptEnv) string { return e.link }, admin: true, wantErr: true},
		{name: "traversal to another tenant", path: func(e *adoptEnv) string { return e.own + "/../../tenant-b/secret" }, wantErr: true},
		{name: "adopt root as editor", path: func(e *adoptEnv) string { return e.extra }, wantErr: true},
		{name: "adopt root as admin", path: func(e *adoptEnv) string { return e.extra }, admin: true},
		{name: "outside everything as admin", path: func(e *adoptEnv) string { return e.outside }, admin: true, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newAdoptEnv(t)
			ctx := tenantctx.WithTenant(context.Background(), "tenant-a")
			p, err := e.svc.Adopt(ctx, "p1", tc.path(e), tc.admin)
			if tc.wantErr {
				if !errors.Is(err, domain.ErrValidation) {
					t.Fatalf("Adopt = %+v, %v; want a validation error", p, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Adopt: %v", err)
			}
			want, _ := filepath.EvalSymlinks(tc.path(e))
			if p.WorkspacePath != want {
				t.Fatalf("workspace = %s, want %s", p.WorkspacePath, want)
			}
		})
	}
}

// S3-F review C4: adopt roots (platform admins only) never open the
// workspace root: a path under it is allowed only in the caller's own
// tenant area, whatever the adopt roots say - also when an adopt root
// contains the workspace root.
func TestAdopt_AdoptRootsNeverOpenTheWorkspaceRoot(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "workspaces")
	own := filepath.Join(root, tenantctx.DefaultTenantID, "mine")
	otherTenant := filepath.Join(root, "tenant-a", "work")
	beside := filepath.Join(base, "e2e", "repo")
	for _, d := range []string{own, otherTenant, beside} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	svc := NewProjectService(&mockStore{projects: []project.Project{{ID: "p1", Name: "Alpha"}}}, root)
	svc.SetAdoptRoots([]string{base}) // contains the workspace root
	ctx := tenantctx.WithTenant(context.Background(), tenantctx.DefaultTenantID)

	if _, err := svc.Adopt(ctx, "p1", otherTenant, true); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("platform admin adopting another tenant's directory through an adopt root = %v, want a validation error", err)
	}
	if err := svc.checkCloneSource(ctx, otherTenant); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("clone from another tenant's directory through an adopt root = %v, want a validation error", err)
	}
	if _, err := svc.Adopt(ctx, "p1", own, true); err != nil {
		t.Fatalf("adopting the own tenant area: %v", err)
	}
	if _, err := svc.Adopt(ctx, "p1", beside, true); err != nil {
		t.Fatalf("platform admin adopting an adopt-root directory beside the workspace root: %v", err)
	}
	if _, err := svc.Adopt(ctx, "p1", beside, false); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("a non-platform admin adopting an adopt-root directory = %v, want a validation error", err)
	}
}

func TestCheckCloneSource(t *testing.T) {
	e := newAdoptEnv(t)
	tests := []struct {
		url     string
		wantErr bool
	}{
		{url: "https://github.com/example/repo.git"},
		{url: "git@github.com:example/repo.git"},
		{url: e.other, wantErr: true},
		{url: "file://" + e.other, wantErr: true},
		{url: e.outside, wantErr: true},
		{url: "relative/path", wantErr: true},
		{url: e.extra},
		{url: "file://" + e.extra},
	}
	for _, tc := range tests {
		t.Run(tc.url, func(t *testing.T) {
			err := e.svc.checkCloneSource(tenantctx.WithTenant(context.Background(), "tenant-a"), tc.url)
			if tc.wantErr != (err != nil) {
				t.Fatalf("checkCloneSource(%q) = %v, want error %t", tc.url, err, tc.wantErr)
			}
			if err != nil && !errors.Is(err, domain.ErrValidation) {
				t.Fatalf("error %v is not a validation error", err)
			}
		})
	}
}
