package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/review"
	"github.com/Strob0t/CodeForge/internal/domain/user"
)

// KI-94 (owner decision 2026-10-04): editor and file-API changes made while
// a review pipeline's refactoring runs count as the refactoring's, so they
// are recorded - before the change, so a change the measurement includes is
// never missing - and listed in the approval dialog. Writes stay allowed.

type recordedEdit struct {
	projectID, userID string
	op                review.UserEditOp
	paths             []string
	// existed: the first path existed when the edit was recorded, and
	// content was its content then.
	existed bool
	content string
}

// fakeEditRecorder records the edits it is told about, looking at the
// workspace at that moment; err fails the record.
type fakeEditRecorder struct {
	dir   string
	err   error
	edits []recordedEdit
}

func (f *fakeEditRecorder) RecordReviewUserEdits(_ context.Context, projectID, userID string, op review.UserEditOp, paths []string) error {
	e := recordedEdit{projectID: projectID, userID: userID, op: op, paths: slices.Clone(paths)}
	if len(paths) > 0 {
		data, err := os.ReadFile(filepath.Join(f.dir, filepath.FromSlash(paths[0]))) //nolint:gosec // test reads from t.TempDir()
		e.existed, e.content = err == nil, string(data)
	}
	f.edits = append(f.edits, e)
	return f.err
}

func editFixture(t *testing.T) (*FileService, *fakeEditRecorder, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	svc := newTestFileService(dir)
	rec := &fakeEditRecorder{dir: dir}
	svc.SetReviewEditRecorder(rec)
	return svc, rec, dir
}

func TestFileService_RecordsReviewEditsBeforeTheChange(t *testing.T) {
	ctx := context.Background()
	for _, tt := range []struct {
		name   string
		change func(svc *FileService) error
		want   recordedEdit
	}{
		{
			name:   "write",
			change: func(svc *FileService) error { return svc.WriteFile(ctx, "p1", "./a.go", "new", "u1") },
			want:   recordedEdit{projectID: "p1", userID: "u1", op: review.UserEditWrite, paths: []string{"a.go"}, existed: true, content: "old"},
		},
		{
			name:   "write a new file in a new directory",
			change: func(svc *FileService) error { return svc.WriteFile(ctx, "p1", "/src/x/../b.go", "b", "u1") },
			want:   recordedEdit{projectID: "p1", userID: "u1", op: review.UserEditWrite, paths: []string{"src/b.go"}},
		},
		{
			name:   "delete",
			change: func(svc *FileService) error { return svc.DeleteFile(ctx, "p1", "a.go", "u2") },
			want:   recordedEdit{projectID: "p1", userID: "u2", op: review.UserEditDelete, paths: []string{"a.go"}, existed: true, content: "old"},
		},
		{
			name:   "rename records the old and the new path",
			change: func(svc *FileService) error { return svc.RenameFile(ctx, "p1", "a.go", "lib/c.go", "u3") },
			want: recordedEdit{projectID: "p1", userID: "u3", op: review.UserEditRename,
				paths: []string{"a.go", "lib/c.go"}, existed: true, content: "old"},
		},
		{
			name:   "a request without an account",
			change: func(svc *FileService) error { return svc.WriteFile(ctx, "p1", "a.go", "new", "") },
			want:   recordedEdit{projectID: "p1", op: review.UserEditWrite, paths: []string{"a.go"}, existed: true, content: "old"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			svc, rec, _ := editFixture(t)
			if err := tt.change(svc); err != nil {
				t.Fatalf("change: %v", err)
			}
			if len(rec.edits) != 1 {
				t.Fatalf("recorded %+v, want one edit", rec.edits)
			}
			got := rec.edits[0]
			if got.projectID != tt.want.projectID || got.userID != tt.want.userID || got.op != tt.want.op ||
				!slices.Equal(got.paths, tt.want.paths) || got.existed != tt.want.existed || got.content != tt.want.content {
				t.Fatalf("recorded %+v, want %+v (recorded before the change)", got, tt.want)
			}
		})
	}
}

// A change that cannot be recorded is refused and the workspace is left as
// it was: undoing the refactoring would set it back without a warning.
func TestFileService_UnrecordedReviewEditIsRefused(t *testing.T) {
	ctx := context.Background()
	for _, tt := range []struct {
		name   string
		change func(svc *FileService) error
	}{
		{"write", func(svc *FileService) error { return svc.WriteFile(ctx, "p1", "a.go", "new", "u1") }},
		{"delete", func(svc *FileService) error { return svc.DeleteFile(ctx, "p1", "a.go", "u1") }},
		{"rename", func(svc *FileService) error { return svc.RenameFile(ctx, "p1", "a.go", "b.go", "u1") }},
		// Review nit: not even the parent directories are created.
		{"write into a new directory", func(svc *FileService) error { return svc.WriteFile(ctx, "p1", "newdir/b.go", "b", "u1") }},
		{"rename into a new directory", func(svc *FileService) error {
			return svc.RenameFile(ctx, "p1", "a.go", "newdir/b.go", "u1")
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			svc, rec, dir := editFixture(t)
			rec.err = user.ErrAccountGone
			if err := tt.change(svc); !errors.Is(err, user.ErrAccountGone) {
				t.Fatalf("change = %v, want the record's error", err)
			}
			if got := readFile(t, dir, "a.go"); got != "old" {
				t.Fatalf("a.go = %q, want it unchanged", got)
			}
			for _, name := range []string{"b.go", "newdir"} {
				if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
					t.Fatalf("%s: %v, want the workspace left as it was", name, err)
				}
			}
		})
	}
}

// A change the workspace refuses before it is made records nothing.
func TestFileService_RefusedChangeRecordsNoReviewEdit(t *testing.T) {
	ctx := context.Background()
	for _, tt := range []struct {
		name   string
		change func(svc *FileService) error
	}{
		{"write outside the workspace", func(svc *FileService) error { return svc.WriteFile(ctx, "p1", "../x.go", "x", "u1") }},
		{"delete a missing file", func(svc *FileService) error { return svc.DeleteFile(ctx, "p1", "missing.go", "u1") }},
		{"delete the workspace", func(svc *FileService) error { return svc.DeleteFile(ctx, "p1", ".", "u1") }},
		{"rename a missing file", func(svc *FileService) error { return svc.RenameFile(ctx, "p1", "missing.go", "b.go", "u1") }},
		{"rename out of the workspace", func(svc *FileService) error { return svc.RenameFile(ctx, "p1", "a.go", "../b.go", "u1") }},
		{"unknown project", func(svc *FileService) error { return svc.WriteFile(ctx, "p2", "a.go", "x", "u1") }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			svc, rec, _ := editFixture(t)
			if err := tt.change(svc); err == nil {
				t.Fatal("change succeeded, want it refused")
			}
			if len(rec.edits) != 0 {
				t.Fatalf("recorded %+v for a refused change, want nothing", rec.edits)
			}
		})
	}
}

// Without a recorder (no review pipeline wired) the file API works as before.
func TestFileService_WithoutReviewEditRecorder(t *testing.T) {
	dir := t.TempDir()
	svc := newTestFileService(dir)
	if err := svc.WriteFile(context.Background(), "p1", "a.go", "x", "u1"); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "a.go")); err != nil {
		t.Fatalf("a.go not written: %v", err)
	}
}
