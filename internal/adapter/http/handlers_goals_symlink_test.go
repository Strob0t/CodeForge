package http

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The goal documents injected into an AI goal discovery run are read
// through workspacefs (KI-95): a symlink out of the workspace is skipped and a
// FIFO does not block the request.
func TestGoalDocEntries_StayInsideTheWorkspace(t *testing.T) {
	base := t.TempDir()
	ws := filepath.Join(base, "ws")
	if err := os.MkdirAll(filepath.Join(ws, "docs"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "secret"), []byte("outside secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../../secret", filepath.Join(ws, "docs", "PROJECT.md")); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(ws, "docs", "STATE.md"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "docs", "REQUIREMENTS.md"), []byte("# Requirements"), 0o600); err != nil {
		t.Fatal(err)
	}

	done := make(chan []string, 1)
	go func() {
		var paths []string
		for _, e := range goalDocEntries(ws) {
			if strings.Contains(e.Content, "outside secret") {
				t.Errorf("%s holds outside content", e.Path)
			}
			paths = append(paths, e.Path)
		}
		done <- paths
	}()
	select {
	case paths := <-done:
		if strings.Join(paths, ",") != "docs/REQUIREMENTS.md" {
			t.Fatalf("entries = %v, want only docs/REQUIREMENTS.md", paths)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("reading the goal documents blocked on a FIFO")
	}
}
