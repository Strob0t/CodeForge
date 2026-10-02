//go:build unix

package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// KI-106: detect-stack by path stays in the caller's tenant area (the rule
// Adopt uses), resolved with no symlink leading out of it.
func TestDetectStackByPath_TenantArea(t *testing.T) {
	root := t.TempDir()
	mine := filepath.Join(root, "tenant-a", "p1")
	theirs := filepath.Join(root, "tenant-b", "p2")
	for _, d := range []string{mine, theirs} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, filepath.Join(d, "package.json"), `{"dependencies":{"react":"18"}}`)
	}
	if err := os.Symlink("../../tenant-b/p2", filepath.Join(mine, "peek")); err != nil {
		t.Fatal(err)
	}
	svc := NewProjectService(&mockStore{}, root)
	ctx := tenantctx.WithTenant(context.Background(), "tenant-a")

	if _, err := svc.DetectStackByPath(ctx, mine, false); err != nil {
		t.Fatalf("own workspace: %v", err)
	}
	for _, p := range []string{theirs, filepath.Join(root, "tenant-b"), root, filepath.Join(mine, "peek")} {
		if _, err := svc.DetectStackByPath(ctx, p, true); !errors.Is(err, domain.ErrValidation) {
			t.Errorf("DetectStackByPath(%s) = %v, want a validation error", p, err)
		}
	}
}
