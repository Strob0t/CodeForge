//go:build unix

package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain"
)

// S2 (S7-B review): a name that cleans to the workspace root ("x/..",
// "./", "a/b/../..") must never be deleted or renamed.
func TestFileService_DeleteAndRenameRefuseTheRoot(t *testing.T) {
	ws := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws, "x", "y"), 0o750); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(ws, "keep.txt"), "keep")
	svc := newTestFileService(ws)
	ctx := context.Background()

	for _, name := range []string{".", "", "/", "x/..", "./", "x/y/../..", "x/../.", "/x/.."} {
		if err := svc.DeleteFile(ctx, "p1", name, ""); !errors.Is(err, domain.ErrValidation) {
			t.Errorf("DeleteFile(%q) = %v, want a validation error", name, err)
		}
		if err := svc.RenameFile(ctx, "p1", name, "moved", ""); !errors.Is(err, domain.ErrValidation) {
			t.Errorf("RenameFile(%q, moved) = %v, want a validation error", name, err)
		}
		if err := svc.RenameFile(ctx, "p1", "keep.txt", name, ""); !errors.Is(err, domain.ErrValidation) {
			t.Errorf("RenameFile(keep.txt, %q) = %v, want a validation error", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(ws, "keep.txt")); err != nil {
		t.Fatal("the workspace contents were changed")
	}
	if err := svc.DeleteFile(ctx, "p1", "x/y/..", ""); err != nil {
		t.Fatalf("DeleteFile(x/y/..) deletes x: %v", err)
	}
	if _, err := os.Stat(filepath.Join(ws, "x")); !os.IsNotExist(err) {
		t.Fatal("x/y/.. names x")
	}
}

// S3 (S7-B review): errors that reach clients name the path inside the
// workspace, never the workspace's absolute path.
func TestFileService_ErrorsDoNotLeakTheWorkspacePath(t *testing.T) {
	ws, out := symlinkWorkspace(t)
	swapped := filepath.Join(filepath.Dir(ws), "swapped")
	if err := os.Symlink(out, swapped); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, svc := range []*FileService{newTestFileService(ws), newTestFileService(swapped)} {
		for _, name := range []string{"leak.txt", "../out/secret.txt", "pipe", "outdir/x"} {
			_, err := svc.ReadFile(ctx, "p1", name)
			if err == nil {
				t.Fatalf("ReadFile(%s) succeeded", name)
			}
			if strings.Contains(err.Error(), filepath.Dir(ws)) {
				t.Errorf("error %q names the workspace's absolute path", err)
			}
		}
	}
}
