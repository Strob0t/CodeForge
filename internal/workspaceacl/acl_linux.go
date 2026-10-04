//go:build linux

package workspaceacl

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

// Supported reports whether this build can set POSIX ACLs.
const Supported = true

// tenantDirMode is the mode of a tenant directory: setgid (its entries join
// the workspace group), rwx for the owner and the group (the mask), nothing
// for others.
const tenantDirMode = 0o2770

func setACL(fd int, name string, entries []Entry) error {
	if err := unix.Fsetxattr(fd, name, Encode(entries), 0); err != nil {
		return fmt.Errorf("set %s: %w", name, err)
	}
	return nil
}

// getACL reads an ACL; nil when the entry has none (an access ACL that
// says no more than the mode bits is not stored).
func getACL(fd int, name string) ([]Entry, error) {
	buf := make([]byte, 4+8*32)
	for {
		n, err := unix.Fgetxattr(fd, name, buf)
		switch {
		case errors.Is(err, unix.ENODATA):
			return nil, nil
		case errors.Is(err, unix.ERANGE) && len(buf) < 1<<16:
			buf = make([]byte, 2*len(buf))
			continue
		case err != nil:
			return nil, fmt.Errorf("get %s: %w", name, err)
		}
		return Decode(buf[:n])
	}
}

// validName reports whether name is one path component (a tenant ID).
func validName(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, "/\x00")
}

// EnsureTenantDir creates the tenant directory <root>/<tenantID> if needed
// and gives it the tenant's exact ACLs (KI-96): mode 2770, access ACL
// TenantAccess(toolUID), default ACL TenantDefault(toolUID, WorkspaceGID).
// The directory is changed only through a descriptor opened without
// following a symlink, and only when it is a directory of the Go Core's
// own: a tool cannot redirect the change, and a directory of another user
// (a legacy tool's) is refused. Call it before git or MkdirAll could create
// the tenant directory with plain modes.
func EnsureTenantDir(root, tenantID string, toolUID int) error {
	if !validName(tenantID) {
		return fmt.Errorf("tenant directory: %q is not a tenant ID", tenantID)
	}
	rootFd, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("open workspace root %s: %w", root, err)
	}
	defer unix.Close(rootFd) //nolint:errcheck // read-only descriptor
	if err := unix.Mkdirat(rootFd, tenantID, 0o770); err != nil && !errors.Is(err, unix.EEXIST) {
		return fmt.Errorf("create tenant directory %s/%s: %w", root, tenantID, err)
	}
	fd, err := unix.Openat(rootFd, tenantID, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("open tenant directory %s/%s (a symlink or no directory?): %w", root, tenantID, err)
	}
	defer unix.Close(fd) //nolint:errcheck // read-only descriptor
	return setDirACLs(fd, fmt.Sprintf("%s/%s", root, tenantID), tenantDirMode, TenantAccess(toolUID), TenantDefault(toolUID, WorkspaceGID))
}

// SetProjectACLs gives an adopted workspace directory (outside the root,
// owned by the Go Core) the ACLs a project inside a tenant directory
// inherits; its content is the worker's to migrate.
func SetProjectACLs(dir string, toolUID int) error {
	fd, err := unix.Open(dir, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("open workspace %s: %w", dir, err)
	}
	defer unix.Close(fd) //nolint:errcheck // read-only descriptor
	acl := ProjectAccess(toolUID, WorkspaceGID)
	return setDirACLs(fd, dir, -1, acl, acl)
}

// setDirACLs sets mode (unless -1) and the ACLs on the directory fd, which
// must be a directory the caller owns.
func setDirACLs(fd int, path string, mode int, access, deflt []Entry) error {
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	if st.Mode&unix.S_IFMT != unix.S_IFDIR {
		return fmt.Errorf("%s is not a directory", path)
	}
	if uid := os.Getuid(); int(st.Uid) != uid {
		return fmt.Errorf("%s belongs to uid %d, not this process (uid %d): refused", path, st.Uid, uid)
	}
	if mode >= 0 {
		// Before the ACL: setting the access ACL then sets the group bits from its mask.
		if err := unix.Fchmod(fd, uint32(mode)); err != nil { //nolint:gosec // G115: a file mode
			return fmt.Errorf("chmod %s: %w", path, err)
		}
	}
	if err := setACL(fd, XattrAccess, access); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if err := setACL(fd, XattrDefault, deflt); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// DirACLs returns a directory's access and default ACLs (for checks).
func DirACLs(dir string) (access, deflt []Entry, err error) {
	fd, err := unix.Open(dir, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, nil, err
	}
	defer unix.Close(fd) //nolint:errcheck // read-only descriptor
	if access, err = getACL(fd, XattrAccess); err != nil {
		return nil, nil, err
	}
	if deflt, err = getACL(fd, XattrDefault); err != nil {
		return nil, nil, err
	}
	return access, deflt, nil
}
