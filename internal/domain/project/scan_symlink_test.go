//go:build unix

package project

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// Stack and gate detection read manifests through workspacefs (KI-95): a
// manifest symlinked out of the workspace is not read, a FIFO does not block
// the scan, and a workspace that is itself a symlink is refused.
func TestScanWorkspace_StaysInsideTheWorkspace(t *testing.T) {
	base := t.TempDir()
	ws := filepath.Join(base, "ws")
	out := filepath.Join(base, "out")
	for _, d := range []string{ws, out} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(out, "package.json"),
		[]byte(`{"dependencies":{"react":"18"},"scripts":{"test":"jest"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../out/package.json", filepath.Join(ws, "package.json")); err != nil {
		t.Fatal(err)
	}
	for _, fifo := range []string{"go.mod", "pyproject.toml"} {
		if err := syscall.Mkfifo(filepath.Join(ws, fifo), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	done := make(chan *StackDetectionResult, 1)
	go func() {
		result, err := ScanWorkspace(ws)
		if err != nil {
			t.Error(err)
		}
		done <- result
	}()
	var result *StackDetectionResult
	select {
	case result = <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the scan blocked on a FIFO")
	}
	for _, lang := range result.Languages {
		if len(lang.Frameworks) > 0 {
			t.Fatalf("frameworks read through a symlink out of the workspace: %+v", lang)
		}
	}

	swapped := filepath.Join(base, "swapped")
	if err := os.Symlink(out, swapped); err != nil {
		t.Fatal(err)
	}
	if _, err := ScanWorkspace(swapped); err == nil {
		t.Fatal("a workspace that is a symlink must be refused")
	}
}
