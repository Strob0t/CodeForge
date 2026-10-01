package project

import "io/fs"

// Workspace permissions (KI-71). The Go Core, the worker and the worker's tool
// user share the workspaces through the workspace group (gid 10010 in the
// images; the workspace root is setgid, so new entries join the group): what
// any of them creates stays writable for the others. The Go Core's umask
// (002) applies on top.
const (
	WorkspaceDirPerm  fs.FileMode = 0o770
	WorkspaceFilePerm fs.FileMode = 0o664
)
