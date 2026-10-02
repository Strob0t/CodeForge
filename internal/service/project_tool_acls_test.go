//go:build linux

package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/port/gitprovider"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
	"github.com/Strob0t/CodeForge/internal/workspaceacl"
)

// KI-96: with workspace.tool_acls: required the Go Core creates the tenant
// directory with its ACLs before git or MkdirAll could create it with plain
// modes; with off nothing of that runs.

const aclTenant = "aaaaaaaa-0000-0000-0000-0000000000a1"

func aclRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := unix.Setxattr(root, workspaceacl.XattrDefault, workspaceacl.Encode(workspaceacl.TenantDefault(20000, 10010)), 0); err != nil {
		if errors.Is(err, unix.EOPNOTSUPP) {
			t.Skip("no POSIX ACLs on the test file system")
		}
		t.Fatal(err)
	}
	if err := unix.Removexattr(root, workspaceacl.XattrDefault); err != nil {
		t.Fatal(err)
	}
	return root
}

// cloneProbe is a git provider that records what the tenant directory looked
// like when Clone ran, and creates the destination.
type cloneProbe struct {
	gitprovider.Provider
	seen *[]error
}

func (c cloneProbe) Name() string { return "acl-probe" }

func (c cloneProbe) Clone(_ context.Context, _, destPath string, _ ...gitprovider.CloneOption) error {
	tenantDir := filepath.Dir(destPath)
	access, _, err := workspaceacl.DirACLs(tenantDir)
	switch {
	case err != nil:
		*c.seen = append(*c.seen, err)
	case !workspaceacl.Equal(access, workspaceacl.TenantAccess(20000)):
		*c.seen = append(*c.seen, errors.New("the tenant directory had no tenant ACL when Clone ran"))
	}
	return os.MkdirAll(destPath, 0o770)
}

func aclProjectService(t *testing.T, root string, required bool, p *project.Project) *ProjectService {
	t.Helper()
	store := &mockStore{projects: []project.Project{*p}}
	svc := NewProjectService(store, root)
	svc.SetToolUIDs(NewToolUIDService(&fakeToolUIDStore{next: 20000}, required))
	return svc
}

func TestClone_TenantDirectoryHasItsACLsBeforeGitRuns(t *testing.T) {
	root := aclRoot(t)
	var seen []error
	svc := aclProjectService(t, root, true, &project.Project{ID: "p1", Provider: "acl-probe", RepoURL: "https://example.com/r.git"})
	svc.resolveProvider = func(*project.Project) (gitprovider.Provider, error) { return cloneProbe{seen: &seen}, nil }
	ctx := tenantctx.WithTenant(context.Background(), aclTenant)
	if _, err := svc.Clone(ctx, "p1", aclTenant, ""); err != nil {
		t.Fatalf("Clone: %v", err)
	}
	if len(seen) != 0 {
		t.Fatalf("at Clone: %v", seen)
	}
}

func TestInitWorkspace_ToolACLs(t *testing.T) {
	for _, required := range []bool{true, false} {
		root := aclRoot(t)
		svc := aclProjectService(t, root, required, &project.Project{ID: "p1"})
		ctx := tenantctx.WithTenant(context.Background(), aclTenant)
		p, err := svc.InitWorkspace(ctx, "p1", aclTenant)
		if err != nil {
			t.Fatalf("required=%v: InitWorkspace: %v", required, err)
		}
		access, _, err := workspaceacl.DirACLs(filepath.Join(root, aclTenant))
		if err != nil {
			t.Fatal(err)
		}
		if required != (access != nil) {
			t.Fatalf("required=%v: tenant directory ACL = %v", required, access)
		}
		if required {
			projAccess, _, _ := workspaceacl.DirACLs(p.WorkspacePath)
			if !workspaceacl.Equal(projAccess, workspaceacl.ProjectAccess(20000, workspaceacl.WorkspaceGID)) {
				t.Fatalf("project ACL = %v", projAccess)
			}
		}
	}
}

func TestAdopt_OutsideTheRootGetsProjectACLs(t *testing.T) {
	root := aclRoot(t)
	adoptRoot := aclRoot(t)
	dir := filepath.Join(adoptRoot, "ws")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	svc := aclProjectService(t, root, true, &project.Project{ID: "p1"})
	svc.SetAdoptRoots([]string{adoptRoot})
	ctx := tenantctx.WithTenant(context.Background(), aclTenant)
	if _, err := svc.Adopt(ctx, "p1", dir, true); err != nil {
		t.Fatalf("Adopt: %v", err)
	}
	access, deflt, err := workspaceacl.DirACLs(dir)
	if err != nil || !workspaceacl.Equal(access, workspaceacl.ProjectAccess(20000, 10010)) || deflt == nil {
		t.Fatalf("adopted dir ACLs = %v / %v, %v", access, deflt, err)
	}

	if os.Getuid() != 0 {
		return
	}
	foreign := filepath.Join(adoptRoot, "foreign")
	if err := os.Mkdir(foreign, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(foreign, 10002, 10002); err != nil {
		t.Fatal(err)
	}
	_, err = svc.Adopt(ctx, "p1", foreign, true)
	if err == nil || !strings.Contains(err.Error(), "setfacl -R -m u:20000:rwX") {
		t.Fatalf("adopting a directory of another user: %v, want the operator's setfacl command", err)
	}
}

type adoptedLister struct{ projects []project.Project }

func (a adoptedLister) ListAdoptedWorkspaces(context.Context, string) ([]project.Project, error) {
	return a.projects, nil
}

type advanceRecorder struct {
	fakeToolUIDStore
	advanced []int
}

func (a *advanceRecorder) AdvanceToolUIDSequence(_ context.Context, atLeast int) (bool, error) {
	a.advanced = append(a.advanced, atLeast)
	return true, nil
}

func TestPrepareAtStartup_AdvancesPastTheBindingsAndOpensAdoptedWorkspaces(t *testing.T) {
	root := aclRoot(t)
	bindings := filepath.Join(root, toolUIDBindingsDir)
	if err := os.MkdirAll(bindings, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"20003", "20010", "junk", "30001", "19999"} {
		if err := os.WriteFile(filepath.Join(bindings, name), []byte(aclTenant), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	adopted := aclRoot(t)
	store := &advanceRecorder{fakeToolUIDStore: fakeToolUIDStore{next: 20011}}
	svc := NewToolUIDService(store, true)
	lister := adoptedLister{projects: []project.Project{{ID: "p9", TenantID: aclTenant, WorkspacePath: adopted}}}
	if err := svc.PrepareAtStartup(context.Background(), root, lister); err != nil {
		t.Fatalf("PrepareAtStartup: %v", err)
	}
	if len(store.advanced) != 1 || store.advanced[0] != 20010 {
		t.Fatalf("advanced to %v, want the highest binding 20010", store.advanced)
	}
	access, _, _ := workspaceacl.DirACLs(adopted)
	if !workspaceacl.Equal(access, workspaceacl.ProjectAccess(20011, workspaceacl.WorkspaceGID)) {
		t.Fatalf("adopted workspace ACL = %v", access)
	}

	// Off: nothing runs.
	off := &advanceRecorder{}
	if err := NewToolUIDService(off, false).PrepareAtStartup(context.Background(), root, lister); err != nil || len(off.advanced) != 0 {
		t.Fatalf("off: %v, advanced %v", err, off.advanced)
	}
}
