package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

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

// gatedClone is a git provider whose clones wait for release and then, like
// git clone, refuse a destination that already exists.
type gatedClone struct {
	gitprovider.Provider
	entered chan<- struct{}
	release <-chan struct{}
}

func (gatedClone) Name() string { return "gated" }

func (c gatedClone) Clone(_ context.Context, _, destPath string, _ ...gitprovider.CloneOption) error {
	c.entered <- struct{}{}
	<-c.release
	if err := os.MkdirAll(filepath.Dir(destPath), 0o750); err != nil {
		return err
	}
	if err := os.Mkdir(destPath, 0o750); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(destPath, "HEAD"), []byte("ref"), 0o600)
}

// syncedStore serializes the project reads and writes of concurrent clones
// and hands each one its own copy, as a database does.
type syncedStore struct {
	*mockStore
	mu sync.Mutex
}

func (s *syncedStore) GetProject(ctx context.Context, id string) (*project.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.mockStore.GetProject(ctx, id)
	if err != nil {
		return nil, err
	}
	cp := *p
	return &cp, nil
}

func (s *syncedStore) UpdateProject(ctx context.Context, p *project.Project) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mockStore.UpdateProject(ctx, p)
}

// Two concurrent clones of one project: the one that loses must not remove
// the winner's workspace while cleaning up after itself.
func TestClone_ConcurrentClonesKeepTheWinnersWorkspace(t *testing.T) {
	const tenant = "aaaaaaaa-0000-0000-0000-0000000000c2"
	root := t.TempDir()
	p := project.Project{ID: "p1", Provider: "gated", RepoURL: "https://example.com/r.git"}
	svc := NewProjectService(&syncedStore{mockStore: &mockStore{projects: []project.Project{p}}}, root)
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	svc.resolveProvider = func(*project.Project) (gitprovider.Provider, error) {
		return gatedClone{entered: entered, release: release}, nil
	}
	ctx := tenantctx.WithTenant(context.Background(), tenant)

	errs := make(chan error, 2)
	clone := func() {
		_, err := svc.Clone(ctx, "p1", tenant, "")
		errs <- err
	}
	go clone()
	<-entered
	go clone()
	// Give the second clone the chance to reach the provider while the first
	// one is still running.
	select {
	case <-entered:
	case <-time.After(100 * time.Millisecond):
	}
	close(release)

	succeeded := 0
	for range 2 {
		if err := <-errs; err == nil {
			succeeded++
		}
	}
	if succeeded == 0 {
		t.Fatal("no clone succeeded")
	}
	if _, err := os.Stat(filepath.Join(root, tenant, "p1", "HEAD")); err != nil {
		t.Fatalf("the successful clone's workspace is gone: %v", err)
	}
}
