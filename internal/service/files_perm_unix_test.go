//go:build unix

package service

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// KI-71: files the Go Core writes into a workspace stay writable for the
// workspace group, which the worker's tool user is in (the Go Core runs with
// umask 002).
func TestWriteFile_WritableForTheWorkspaceGroup(t *testing.T) {
	previous := syscall.Umask(0o002)
	t.Cleanup(func() { syscall.Umask(previous) })

	wsDir := t.TempDir()
	svc := newTestFileService(wsDir)
	if err := svc.WriteFile(context.Background(), "p1", "new.txt", "x", ""); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(wsDir, "new.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o664 {
		t.Errorf("mode %o, want 664", got)
	}
}
