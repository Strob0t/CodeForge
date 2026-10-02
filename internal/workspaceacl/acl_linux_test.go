//go:build linux

package workspaceacl

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
)

// aclSupported skips when the test directory's file system has no POSIX ACLs.
func aclSupported(t *testing.T, dir string) {
	t.Helper()
	if err := unix.Setxattr(dir, XattrDefault, Encode(TenantDefault(20000, WorkspaceGID)), 0); err != nil {
		if errors.Is(err, unix.EOPNOTSUPP) {
			t.Skip("the test directory's file system has no POSIX ACLs")
		}
		t.Fatalf("probe: %v", err)
	}
	if err := unix.Removexattr(dir, XattrDefault); err != nil {
		t.Fatal(err)
	}
}

func TestEnsureTenantDirSetsModeAndExactACLs(t *testing.T) {
	root := t.TempDir()
	aclSupported(t, root)
	const tenantID = "aaaaaaaa-0000-0000-0000-000000000001"

	for range 2 { // idempotent: an existing directory is fixed again
		if err := EnsureTenantDir(root, tenantID, 20007); err != nil {
			t.Fatalf("EnsureTenantDir: %v", err)
		}
	}
	dir := filepath.Join(root, tenantID)
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode(); !mode.IsDir() || mode&os.ModeSetgid == 0 || mode.Perm() != 0o770 {
		t.Fatalf("mode = %v, want drwxrws---", mode)
	}
	access, deflt, err := DirACLs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !Equal(access, TenantAccess(20007)) {
		t.Errorf("access ACL = %v, want %v", access, TenantAccess(20007))
	}
	if !Equal(deflt, TenantDefault(20007, WorkspaceGID)) {
		t.Errorf("default ACL = %v, want %v", deflt, TenantDefault(20007, WorkspaceGID))
	}

	// A project created inside inherits the default ACL: the tool UID can write it.
	project := filepath.Join(dir, "proj")
	if err := os.Mkdir(project, 0o770); err != nil {
		t.Fatal(err)
	}
	projAccess, _, err := DirACLs(project)
	if err != nil {
		t.Fatal(err)
	}
	if !Equal(projAccess, ProjectAccess(20007, WorkspaceGID)) {
		t.Errorf("project access ACL = %v, want %v", projAccess, ProjectAccess(20007, WorkspaceGID))
	}
}

func TestEnsureTenantDirRefusesSymlinksAndOtherOwners(t *testing.T) {
	root := t.TempDir()
	aclSupported(t, root)
	victim := t.TempDir()
	if err := os.Symlink(victim, filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	if err := EnsureTenantDir(root, "linked", 20000); err == nil {
		t.Fatal("a symlinked tenant directory was accepted")
	}
	if access, _, _ := DirACLs(victim); access != nil {
		t.Fatalf("the symlink's target got an ACL: %v", access)
	}
	if err := os.WriteFile(filepath.Join(root, "file"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := EnsureTenantDir(root, "file", 20000); err == nil {
		t.Fatal("a file was accepted as tenant directory")
	}
	for _, bad := range []string{"", ".", "..", "a/b", "x\x00"} {
		if err := EnsureTenantDir(root, bad, 20000); err == nil {
			t.Errorf("tenant ID %q accepted", bad)
		}
	}

	if os.Getuid() != 0 {
		return
	}
	foreign := filepath.Join(root, "foreign")
	if err := os.Mkdir(foreign, 0o770); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(foreign, 0o770); err != nil { //nolint:gosec // G302: a tenant directory mode
		t.Fatal(err)
	}
	if err := os.Chown(foreign, 10002, 10010); err != nil {
		t.Fatal(err)
	}
	err := EnsureTenantDir(root, "foreign", 20000)
	if err == nil || !strings.Contains(err.Error(), "uid 10002") {
		t.Fatalf("a tenant directory of the legacy tool user: err = %v, want refused", err)
	}
	var st syscall.Stat_t
	if err := syscall.Stat(foreign, &st); err != nil || st.Mode&0o7777 != 0o770 {
		t.Fatalf("the foreign directory changed: mode %o, %v", st.Mode&0o7777, err)
	}
}
