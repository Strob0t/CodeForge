//go:build !linux

package workspaceacl

// Supported reports whether this build can set POSIX ACLs.
const Supported = false

// EnsureTenantDir needs Linux: workspace.tool_acls: required fails at startup elsewhere.
func EnsureTenantDir(string, string, int) error { return ErrUnsupported }

// SetProjectACLs needs Linux.
func SetProjectACLs(string, int) error { return ErrUnsupported }

// DirACLs needs Linux.
func DirACLs(string) (access, deflt []Entry, err error) { return nil, nil, ErrUnsupported }
