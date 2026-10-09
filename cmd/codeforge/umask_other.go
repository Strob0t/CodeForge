//go:build !unix

package main

// setWorkspaceUmask does nothing where there is no umask.
func setWorkspaceUmask() int { return 0 }
