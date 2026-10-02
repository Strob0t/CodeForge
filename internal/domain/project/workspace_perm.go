package project

import "io/fs"

// Workspace permissions (KI-71, KI-96). The Go Core and the worker share the
// workspaces through the workspace group (gid 10010 in the images; the
// workspace root is setgid, so new entries join the group): what either
// creates stays writable for the other. The Go Core's umask (002) applies on
// top. With workspace.tool_acls: required every tenant directory carries a
// default ACL for the tenant's tool UID and the workspace group; the kernel
// then ignores the umask and these modes' group bits become the ACL mask
// (0770 -> rwx, 0664 -> rw-): the tool UID gets what the mask allows.
const (
	WorkspaceDirPerm  fs.FileMode = 0o770
	WorkspaceFilePerm fs.FileMode = 0o664
)
