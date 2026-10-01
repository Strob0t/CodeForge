package git

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The walk for nested .git entries is bounded; reaching a bound refuses the
// workspace (fail closed).
func TestWalkForNestedGit_Bounds(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "a", "b", "c"), 0o755); err != nil {
		t.Fatal(err)
	}
	for i := range 5 {
		if err := os.WriteFile(filepath.Join(root, "f"+string(rune('0'+i))), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := walkForNestedGit(root, nil); err != nil {
		t.Fatalf("walk within bounds: %v", err)
	}

	defer func(entries, depth int) { maxNestedWalkEntries, maxNestedWalkDepth = entries, depth }(maxNestedWalkEntries, maxNestedWalkDepth)
	maxNestedWalkEntries = 4
	if err := walkForNestedGit(root, nil); !errors.Is(err, ErrUnsafeRepository) || !strings.Contains(err.Error(), "cannot be checked") {
		t.Fatalf("walk over the entry bound = %v, want a refusal", err)
	}
	maxNestedWalkEntries, maxNestedWalkDepth = 1000, 2
	if err := walkForNestedGit(root, nil); !errors.Is(err, ErrUnsafeRepository) {
		t.Fatalf("walk over the depth bound = %v, want a refusal", err)
	}
	// Ignored directories are not counted.
	maxNestedWalkDepth = 64
	if err := walkForNestedGit(root, map[string]bool{"a/": true}); err != nil {
		t.Fatalf("walk skipping an ignored directory: %v", err)
	}
}

// git treats every mode with the gitlink type bits as a gitlink and prints
// a hand-written index entry's mode as stored: the check parses the mode
// (security review of the nested-repository fix).
func TestUnsafeIndexEntry(t *testing.T) {
	tests := []struct {
		name, meta, path string
		unsafe           bool
	}{
		{"regular file", "100644 0123456789abcdef0123456789abcdef01234567 0", "src/a.go", false},
		{"executable", "100755 0123456789abcdef0123456789abcdef01234567 0", "run.sh", false},
		{"symlink", "120000 0123456789abcdef0123456789abcdef01234567 0", "link", false},
		{"ls-tree blob", "100644 blob 0123456789abcdef0123456789abcdef01234567", "a.txt", false},
		{"gitlink", "160000 0123456789abcdef0123456789abcdef01234567 0", "sub", true},
		{"gitlink with permission bits", "160755 0123456789abcdef0123456789abcdef01234567 0", "sub", true},
		{"ls-tree gitlink", "160000 commit 0123456789abcdef0123456789abcdef01234567", "sub", true},
		{"path in .git", "100644 0123456789abcdef0123456789abcdef01234567 0", ".git/x/f", true},
		{"nested .git component", "100644 0123456789abcdef0123456789abcdef01234567 0", "a/.GIT/config", true},
		{"unreadable mode", "16x000 0123456789abcdef0123456789abcdef01234567 0", "sub", true},
		{"similar name", "100644 0123456789abcdef0123456789abcdef01234567 0", "a/.github/ci.yml", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := unsafeIndexEntry(tt.meta, tt.path) != ""; got != tt.unsafe {
				t.Fatalf("unsafeIndexEntry(%q, %q) unsafe = %v, want %v", tt.meta, tt.path, got, tt.unsafe)
			}
		})
	}
	if err := refuseUnsafeEntries("100644 0123456789abcdef0123456789abcdef01234567 0\ta\x00160755 0123456789abcdef0123456789abcdef01234567 0\tsub\x00"); !errors.Is(err, ErrUnsafeRepository) {
		t.Fatalf("refuseUnsafeEntries = %v, want ErrUnsafeRepository", err)
	}
}
