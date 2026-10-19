//go:build unix

package main

import "syscall"

// workspaceUmask keeps what the Go Core creates in the workspaces (clones,
// checkouts, rewinds, files) writable for the workspace group, which the
// worker's tool user is in (KI-71).
const workspaceUmask = 0o002

// setWorkspaceUmask sets the process umask and returns the previous one.
func setWorkspaceUmask() int {
	return syscall.Umask(workspaceUmask)
}
