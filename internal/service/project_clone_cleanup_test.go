package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/port/gitprovider"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// partialClone is a git provider whose clone writes part of a repository and
// then ends with err (nil: the clone succeeds).
type partialClone struct {
	gitprovider.Provider
	err error
}

func (partialClone) Name() string { return "partial" }

func (c partialClone) Clone(_ context.Context, _, destPath string, _ ...gitprovider.CloneOption) error {
	objects := filepath.Join(destPath, ".git", "objects")
	if err := os.MkdirAll(objects, 0o750); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(objects, "pack.tmp"), []byte("half"), 0o600); err != nil {
		return err
	}
	return c.err
}

// A clone that fails or is cancelled half-way (the request ended, the git
// operation deadline passed) must not leave a partial directory, which the
// next clone would take for an existing repository (KI-213). A directory that
// existed before the clone is not touched.
func TestClone_FailedCloneLeavesNoPartialDirectory(t *testing.T) {
	const tenant = "aaaaaaaa-0000-0000-0000-0000000000c1"
	tests := []struct {
		name     string
		cloneErr error
		existing bool
		wantDir  bool
	}{
		{"cancelled fresh clone", context.Canceled, false, false},
		{"deadline fresh clone", context.DeadlineExceeded, false, false},
		{"failed fresh clone", errors.New("git clone: exit status 128"), false, false},
		{"failed clone into an existing directory", context.Canceled, true, true},
		{"successful clone", nil, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			p := project.Project{ID: "p1", Provider: "partial", RepoURL: "https://example.com/r.git"}
			svc := NewProjectService(&mockStore{projects: []project.Project{p}}, root)
			svc.resolveProvider = func(*project.Project) (gitprovider.Provider, error) {
				return partialClone{err: tt.cloneErr}, nil
			}
			dest := filepath.Join(root, tenant, "p1")
			if tt.existing {
				if err := os.MkdirAll(dest, 0o750); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dest, "keep"), []byte("x"), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			ctx := tenantctx.WithTenant(context.Background(), tenant)
			_, err := svc.Clone(ctx, "p1", tenant, "")

			if (err == nil) != (tt.cloneErr == nil) {
				t.Fatalf("Clone error = %v, want %v", err, tt.cloneErr)
			}
			if tt.cloneErr != nil && !errors.Is(err, tt.cloneErr) {
				t.Errorf("Clone error %v does not wrap %v", err, tt.cloneErr)
			}
			_, statErr := os.Stat(dest)
			if gotDir := statErr == nil; gotDir != tt.wantDir {
				t.Fatalf("destination exists = %v, want %v", gotDir, tt.wantDir)
			}
			if tt.existing {
				if _, err := os.Stat(filepath.Join(dest, "keep")); err != nil {
					t.Errorf("a pre-existing directory lost its content: %v", err)
				}
			}
		})
	}
}
