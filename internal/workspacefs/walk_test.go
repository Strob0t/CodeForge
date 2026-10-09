//go:build unix

package workspacefs

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
)

func openFDs(t *testing.T) int {
	t.Helper()
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Skip("no /proc/self/fd")
	}
	return len(entries)
}

// C12 (S7-B review): WalkDir reads each directory through its parent's
// descriptor, holds at most maxHeldDirs descriptors and still walks deeper
// trees completely; it never descends into a symlink.
func TestWalkDirDeepTreeBoundedDescriptors(t *testing.T) {
	ws := t.TempDir()
	const depth = 3 * maxHeldDirs
	dir := ws
	for i := range depth {
		dir = filepath.Join(dir, "d")
		mustMkdir(t, dir)
		mustWrite(t, filepath.Join(dir, "f"), "x")
		if i == depth/2 {
			mustSymlink(t, "..", filepath.Join(dir, "up"))
		}
	}
	r := mustOpen(t, ws)
	before := openFDs(t)
	most, files := 0, 0
	err := r.WalkDir(".", func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type().IsRegular() {
			files++
		}
		most = max(most, openFDs(t)-before)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files != depth {
		t.Fatalf("walked %d files, want %d", files, depth)
	}
	if most > maxHeldDirs+4 {
		t.Fatalf("held %d descriptors, want at most about %d", most, maxHeldDirs)
	}
	if fds := openFDs(t); fds > before {
		t.Fatalf("WalkDir leaked %d descriptors", fds-before)
	}
}

func TestWalkDirSemantics(t *testing.T) {
	ws, _ := tree(t)
	mustMkdir(t, filepath.Join(ws, "skip", "inner"))
	r := mustOpen(t, ws)
	var seen []string
	err := r.WalkDir(".", func(p string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		seen = append(seen, p)
		if p == "skip" {
			return fs.SkipDir
		}
		if p == "src/sub/b.go" {
			return fs.SkipAll
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{".", "skip", "src", "src/a.go", "src/sub", "src/sub/b.go"}; !slices.Equal(seen, want) {
		t.Fatalf("seen = %v, want %v", seen, want)
	}
	if err := r.WalkDir("missing", func(_ string, _ fs.DirEntry, err error) error { return err }); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing root: %v", err)
	}
}

// A directory swapped for a symlink after its parent listed it is not
// descended through the link.
func TestWalkDirOpenChildRefusesASwap(t *testing.T) {
	ws, _ := tree(t)
	r := mustOpen(t, ws)
	parent, err := r.root.OpenRoot("src/.")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = parent.Close() }()
	child := r.openChild(parent, "src", "sub", true)
	if child == nil {
		t.Fatal("a real subdirectory must open")
	}
	_ = child.Close()
	if err := os.Rename(filepath.Join(ws, "src", "sub"), filepath.Join(ws, "sub-real")); err != nil {
		t.Fatal(err)
	}
	mustSymlink(t, "../sub-real", filepath.Join(ws, "src", "sub"))
	if child := r.openChild(parent, "src", "sub", true); child != nil {
		_ = child.Close()
		t.Fatal("a directory swapped for a symlink must not be descended")
	}
}

// KI-152 review: a bounded walk reads directories in batches and stops once
// its entry budget is spent, before a huge directory is held in memory.
func TestWalkDirBounded(t *testing.T) {
	ws := t.TempDir()
	mustMkdir(t, filepath.Join(ws, "a"))
	mustMkdir(t, filepath.Join(ws, "big"))
	mustWrite(t, filepath.Join(ws, "a", "x"), "1")
	mustWrite(t, filepath.Join(ws, "b"), "2")
	for i := range 3 * readDirBatch {
		mustWrite(t, filepath.Join(ws, "big", "f"+strconv.Itoa(i)), "")
	}
	r := mustOpen(t, ws)
	total := 3 + 3*readDirBatch + 1 // a, a/x, b, big's entries, big

	tests := []struct {
		name    string
		budget  int
		wantErr bool
	}{
		{name: "budget covers the tree", budget: total},
		{name: "one entry too few", budget: total - 1, wantErr: true},
		{name: "a fraction of the big directory", budget: 10, wantErr: true},
		{name: "zero", budget: 0, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var visited []string
			err := r.WalkDirBounded(".", tc.budget, func(name string, _ fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				visited = append(visited, name)
				return nil
			})
			if tc.wantErr != errors.Is(err, ErrTooManyEntries) {
				t.Fatalf("WalkDirBounded(%d) = %v, want too many entries: %v", tc.budget, err, tc.wantErr)
			}
			if tc.wantErr {
				if len(visited) > tc.budget+1 {
					t.Fatalf("visited %d entries with a budget of %d", len(visited), tc.budget)
				}
				return
			}
			if len(visited) != total+1 { // and "." itself
				t.Fatalf("visited %d entries, want %d", len(visited), total+1)
			}
			if !slices.IsSorted(visited[:4]) || visited[1] != "a" || visited[2] != "a/x" {
				t.Fatalf("not in lexical order: %v", visited[:4])
			}
		})
	}
}
